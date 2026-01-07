package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	"one-api/relay"
	"one-api/relay/channel"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strconv"
	"strings"
	"time"
)

func UpdateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, taskChannelM map[int][]string, taskM map[string]*model.Task) error {
	for channelId, taskIds := range taskChannelM {
		if err := updateVideoTaskAll(ctx, platform, channelId, taskIds, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Channel #%d failed to update video async tasks: %s", channelId, err.Error()))
		}
	}
	return nil
}

func updateVideoTaskAll(ctx context.Context, platform constant.TaskPlatform, channelId int, taskIds []string, taskM map[string]*model.Task) error {
	logger.LogInfo(ctx, fmt.Sprintf("Channel #%d pending video tasks: %d", channelId, len(taskIds)))
	if len(taskIds) == 0 {
		return nil
	}
	cacheGetChannel, err := model.CacheGetChannel(channelId)
	if err != nil {
		errUpdate := model.TaskBulkUpdate(taskIds, map[string]any{
			"fail_reason": fmt.Sprintf("Failed to get channel info, channel ID: %d", channelId),
			"status":      "FAILURE",
			"progress":    "100%",
		})
		if errUpdate != nil {
			common.SysLog(fmt.Sprintf("UpdateVideoTask error: %v", errUpdate))
		}
		return fmt.Errorf("CacheGetChannel failed: %w", err)
	}
	adaptor := relay.GetTaskAdaptor(platform)
	if adaptor == nil {
		return fmt.Errorf("video adaptor not found")
	}
	info := &relaycommon.RelayInfo{}
	info.ChannelMeta = &relaycommon.ChannelMeta{
		ChannelType:    cacheGetChannel.Type,
		ApiKey:         cacheGetChannel.Key,
		ChannelBaseUrl: cacheGetChannel.GetBaseURL(),
	}
	adaptor.Init(info)
	for _, taskId := range taskIds {
		if err := updateVideoSingleTask(ctx, adaptor, cacheGetChannel, taskId, taskM); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to update video task %s: %s", taskId, err.Error()))
		}
	}
	return nil
}

func updateVideoSingleTask(ctx context.Context, adaptor channel.TaskAdaptor, channel *model.Channel, taskId string, taskM map[string]*model.Task) error {
	baseURL := constant.ChannelBaseURLs[channel.Type]
	if channel.GetBaseURL() != "" {
		baseURL = channel.GetBaseURL()
	}

	task := taskM[taskId]
	if task == nil {
		logger.LogError(ctx, fmt.Sprintf("Task %s not found in taskM", taskId))
		return fmt.Errorf("task %s not found", taskId)
	}

	// 打印轮询请求信息
	logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 开始轮询任务 - TaskID: %s, Platform: %s, Status: %s",
		taskId, task.Platform, task.Status))

	resp, err := adaptor.FetchTask(baseURL, channel.Key, map[string]any{
		"task_id": taskId,
		"action":  task.Action,
	})
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] 调用厂商接口失败 - TaskID: %s, Error: %v", taskId, err))
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	//if resp.StatusCode != http.StatusOK {
	//return fmt.Errorf("get Video Task status code: %d", resp.StatusCode)
	//}
	defer resp.Body.Close()

	logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 厂商接口响应 - TaskID: %s, StatusCode: %d", taskId, resp.StatusCode))

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] 读取响应失败 - TaskID: %s, Error: %v", taskId, err))
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}

	// 打印厂商返回的响应数据（截断过长内容）
	truncatedResp := common.TruncateBase64Content(string(responseBody))
	logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 厂商返回数据 - TaskID: %s, Response: %s", taskId, truncatedResp))

	taskResult := &relaycommon.TaskInfo{}
	// try parse as New API response format
	var responseItems dto.TaskResponse[model.Task]
	if err = json.Unmarshal(responseBody, &responseItems); err == nil && responseItems.IsSuccess() {
		t := responseItems.Data
		taskResult.TaskID = t.TaskID
		taskResult.Status = string(t.Status)
		taskResult.Url = t.FailReason
		taskResult.Progress = t.Progress
		taskResult.Reason = t.FailReason
		task.Data = t.Data
		logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 解析为NewAPI格式 - TaskID: %s, Status: %s", taskId, taskResult.Status))
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] 解析任务结果失败 - TaskID: %s, Error: %v", taskId, err))
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	} else {
		logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 解析任务结果成功 - TaskID: %s, Status: %s, TotalTokens: %d",
			taskId, taskResult.Status, taskResult.TotalTokens))

		// 保留原有的关键信息（requested_seconds, token信息等），合并新的响应数据
		var existingData map[string]interface{}
		var newData map[string]interface{}

		// 解析现有数据
		if task.Data != nil {
			if err := json.Unmarshal(task.Data, &existingData); err != nil {
				logger.LogWarn(ctx, fmt.Sprintf("[VideoTaskPoll] 无法解析现有 task.Data: %v", err))
				existingData = make(map[string]interface{})
			}
		} else {
			existingData = make(map[string]interface{})
		}

		// 解析新响应数据
		if err := json.Unmarshal(redactVideoResponseBody(responseBody), &newData); err != nil {
			// 如果解析失败，直接使用原始响应体
			logger.LogWarn(ctx, fmt.Sprintf("[VideoTaskPoll] 无法解析响应数据，直接使用原始: %v", err))
			task.Data = redactVideoResponseBody(responseBody)
		} else {
			// 保留原有的关键字段（这些字段不在 API 响应中，但扣费需要）
			preservedFields := []string{
				"requested_seconds", // ⚠️ 关键：扣费需要
				"token_name",
				"token_id",
				"model",
				"model_price",
				"group_ratio",
				"user_group_ratio",
				"billing_pending",
				"billing_processed",
				"generate_audio",
				"generateAudio",
				"sampleCount",
				"sample_count",
				"oem_user_discount", // ⚠️ 关键：OEM用户折扣，扣费需要
				"oem_code",          // ⚠️ 关键：OEM代码，价格链计算需要
			}

			for _, field := range preservedFields {
				if value, exists := existingData[field]; exists {
					newData[field] = value
					logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 保留字段 %s: %v", field, value))
				}
			}
			if _, exists := newData["generateAudio"]; exists {
				delete(newData, "generate_audio")
			}

			// 合并数据并更新
			if mergedData, err := json.Marshal(newData); err == nil {
				task.Data = mergedData
				logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 成功合并 task.Data，保留了 %d 个字段", len(preservedFields)))
			} else {
				logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] 合并数据失败: %v", err))
				task.Data = redactVideoResponseBody(responseBody)
			}
		}
	}

	now := time.Now().Unix()
	if taskResult.Status == "" {
		logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] 任务状态为空 - TaskID: %s", taskId))
		return fmt.Errorf("task %s status is empty", taskId)
	}

	// 记录状态变化
	oldStatus := task.Status
	task.Status = model.TaskStatus(taskResult.Status)
	if oldStatus != task.Status {
		logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 任务状态变化 - TaskID: %s, %s → %s",
			taskId, oldStatus, task.Status))
	}

	switch taskResult.Status {
	case string(model.TaskStatusSubmitted):
		task.Progress = "10%"
	case string(model.TaskStatusQueued):
		task.Progress = "20%"
	case string(model.TaskStatusInProgress):
		task.Progress = "30%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case string(model.TaskStatusSuccess):
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if err := handleVeoTaskMedia(ctx, task, taskResult); err != nil {
			logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] Veo 媒体处理失败，回退为原始 fail_reason，TaskID: %s, Error: %v", task.TaskID, err))
			task.FailReason = taskResult.Url
		}

		// 添加详细调试日志
		logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] ✅ 任务成功 - TaskID: %s, ModelName: %s, TotalTokens: %d",
			task.TaskID, task.ModelName, taskResult.TotalTokens))

		// 处理任务成功后的扣费逻辑（按模型类型判断）
		if isWan25Model(task.ModelName) {
			logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 🎯 检测到 wan2.5 模型，准备扣费 - ModelName: %s", task.ModelName))
			// wan2.5 模型：按分辨率和秒数扣费（使用Usage信息）
			if err := handleWan25TaskBilling(ctx, task, taskResult, channel); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] wan2.5 billing failed for task %s: %v", task.TaskID, err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] wan2.5 billing completed for task %s", task.TaskID))
			}
		} else if isVeoModel(task.ModelName) {
			logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 🎯 检测到 Veo 模型，准备扣费 - ModelName: %s", task.ModelName))
			// Veo 模型：按秒扣费
			if err := handleVeoTaskBilling(ctx, task, channel); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] Veo billing failed for task %s: %v", task.TaskID, err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] Veo billing completed for task %s", task.TaskID))
			}
		} else if isSora2Model(task.ModelName) {
			logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 🎯 检测到 Sora-2 模型，准备扣费 - ModelName: %s", task.ModelName))
			// Sora-2 模型：按秒扣费
			if err := handleSora2TaskBilling(ctx, task, channel); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] Sora-2 billing failed for task %s: %v", task.TaskID, err))
			} else {
				logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] Sora-2 billing completed for task %s", task.TaskID))
			}
		} else if taskResult.TotalTokens > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 🎯 检测到 Doubao 模型（TotalTokens > 0），准备扣费 - ModelName: %s, Tokens: %d", task.ModelName, taskResult.TotalTokens))
			// Doubao 模型：按 token 扣费
			if err := handleVideoTaskBilling(ctx, task, taskResult, channel); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] Failed to handle billing for task %s: %v", task.TaskID, err))
			}
		} else {
			logger.LogWarn(ctx, fmt.Sprintf("[VideoTaskPoll] ⚠️ 未匹配到任何扣费模型 - TaskID: %s, ModelName: %s, TotalTokens: %d",
				task.TaskID, task.ModelName, taskResult.TotalTokens))
		}
	case string(model.TaskStatusFailure):
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogError(ctx, fmt.Sprintf("[VideoTaskPoll] ❌ 任务失败 - TaskID: %s, Reason: %s", task.TaskID, task.FailReason))
		quota := task.Quota
		if quota != 0 {
			logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 任务失败退费 - TaskID: %s, Quota: %d", task.TaskID, quota))
			if err := model.IncreaseUserQuota(task.UserId, quota, false); err != nil {
				logger.LogError(ctx, "Failed to increase user quota: "+err.Error())
			}
			logContent := fmt.Sprintf("Video async task failed %s, refund %s", task.TaskID, logger.LogQuota(quota))
			model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("[VideoTaskPoll] 任务失败无需退费 - TaskID: %s, Quota: 0 (未预扣费)", task.TaskID))
		}
	default:
		return fmt.Errorf("unknown task status %s for task %s", taskResult.Status, taskId)
	}
	if taskResult.Progress != "" {
		task.Progress = taskResult.Progress
	}
	if err := task.Update(); err != nil {
		common.SysLog("UpdateVideoTask task error: " + err.Error())
	}

	return nil
}

func handleVeoTaskMedia(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo) error {
	if !isVeoModel(task.ModelName) {
		task.FailReason = taskResult.Url
		return nil
	}
	if taskResult == nil {
		return errors.New("taskResult 为空")
	}

	dataURIs := make([]string, 0)
	seen := make(map[string]struct{})
	for _, u := range taskResult.Urls {
		if u == "" {
			continue
		}
		if _, exists := seen[u]; exists {
			continue
		}
		dataURIs = append(dataURIs, u)
		seen[u] = struct{}{}
	}
	if taskResult.Url != "" {
		if _, exists := seen[taskResult.Url]; !exists {
			dataURIs = append(dataURIs, taskResult.Url)
			seen[taskResult.Url] = struct{}{}
		}
	}

	if len(dataURIs) == 0 {
		if taskResult.Url == "" {
			return errors.New("taskResult.Url 为空")
		}
		if !strings.HasPrefix(taskResult.Url, "data:") {
			task.FailReason = taskResult.Url
			return nil
		}
		dataURIs = append(dataURIs, taskResult.Url)
	}

	endpoint := service.GetOSSBase64Endpoint()
	if endpoint == "" {
		return fmt.Errorf("OSS_BASE64_ENDPOINT 未配置")
	}

	mediaList := make([]model.TaskMediaInfo, 0, len(dataURIs))
	for idx, dataURI := range dataURIs {
		if !strings.HasPrefix(dataURI, "data:") {
			logger.LogWarn(ctx, fmt.Sprintf("[VideoTaskPoll] Veo 返回非 data URI，第 %d 个，直接保留原链接", idx))
			mediaList = append(mediaList, model.TaskMediaInfo{OssURL: dataURI})
			continue
		}

		mediaInfo, err := uploadVeoMediaToOSS(ctx, endpoint, task, dataURI, idx)
		if err != nil {
			return fmt.Errorf("上传第 %d 个 Veo 视频失败: %w", idx, err)
		}
		mediaList = append(mediaList, *mediaInfo)
	}

	if len(mediaList) == 0 {
		task.FailReason = taskResult.Url
		return nil
	}

	if task.Data != nil {
		var taskData map[string]interface{}
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			taskData["sampleCount"] = len(mediaList)
			if updated, err := json.Marshal(taskData); err == nil {
				task.Data = updated
			}
		}
	}

	serialized, err := model.SerializeTaskMediaInfo(mediaList)
	if err != nil {
		return fmt.Errorf("序列化 Veo 媒体信息失败: %w", err)
	}
	task.FailReason = serialized
	return nil
}

func uploadVeoMediaToOSS(ctx context.Context, endpoint string, task *model.Task, dataURI string, index int) (*model.TaskMediaInfo, error) {
	fileExt := inferExtensionFromDataURI(dataURI)
	if fileExt == "" {
		fileExt = "mp4"
	}
	baseName := sanitizeFileName(task.TaskID)
	fileName := baseName + "." + fileExt
	if index > 0 {
		fileName = fmt.Sprintf("%s-%d.%s", baseName, index+1, fileExt)
	}

	payload := &service.OSSUploadRequest{
		Base64Content: dataURI,
		FileName:      fileName,
		ExtensionType: fileExt,
	}

	resp, err := service.UploadBase64ToOSS(ctx, endpoint, payload)
	if err != nil {
		return nil, err
	}

	info := &model.TaskMediaInfo{
		OssID:    resp.OssID,
		OssURL:   resp.OssURL,
		FileName: resp.FileName,
	}
	if info.FileName == "" {
		info.FileName = fileName
	}

	return info, nil
}

func inferExtensionFromDataURI(dataURI string) string {
	const prefix = "data:"
	if !strings.HasPrefix(dataURI, prefix) {
		return ""
	}
	semicolonIdx := strings.Index(dataURI, ";")
	if semicolonIdx <= len(prefix) {
		return ""
	}
	mime := dataURI[len(prefix):semicolonIdx]
	slashIdx := strings.Index(mime, "/")
	if slashIdx < 0 || slashIdx == len(mime)-1 {
		return ""
	}
	subType := mime[slashIdx+1:]
	if subType == "" {
		return ""
	}
	if plusIdx := strings.Index(subType, "+"); plusIdx > 0 {
		subType = subType[:plusIdx]
	}
	return strings.ToLower(subType)
}

func sanitizeFileName(taskID string) string {
	if taskID == "" {
		return fmt.Sprintf("veo-%d", time.Now().Unix())
	}
	builder := strings.Builder{}
	builder.Grow(len(taskID))
	for _, r := range taskID {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	result := builder.String()
	if result == "" {
		return fmt.Sprintf("veo-%d", time.Now().Unix())
	}
	return result
}

// handleVideoTaskBilling 处理视频任务完成后的实际token消耗补扣费
func handleVideoTaskBilling(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo, channel *model.Channel) error {
	// 防重复扣费：检查是否已经处理过
	var taskData map[string]interface{}
	if task.Data != nil {
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			if billingProcessed, ok := taskData["billing_processed"].(bool); ok && billingProcessed {
				logger.LogInfo(ctx, fmt.Sprintf("[DoubaoTaskBilling] Task %s already billed, skip", task.TaskID))
				return nil
			}
		}
	}

	// 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %v", task.UserId, err)
	}

	// 获取原始模型名称和token信息
	var modelName string
	var tokenName string
	var tokenId int
	if taskData != nil {
		// 尝试从任务数据中提取原始模型名称和token信息
		if model, ok := taskData["model"].(string); ok && model != "" {
			modelName = model
		}
		if token, ok := taskData["token_name"].(string); ok && token != "" {
			tokenName = token
		}
		if token, ok := taskData["token_id"].(float64); ok {
			tokenId = int(token)
		}
	}

	// 如果没有找到模型名称，使用平台-动作组合作为备选
	if modelName == "" {
		modelName = fmt.Sprintf("%s-%s", task.Platform, task.Action)
	}

	// 如果没有找到 token_name，使用 task.ApiKey 作为备选（新增的表字段）
	if tokenName == "" && task.ApiKey != "" {
		tokenName = task.ApiKey
		logger.LogInfo(ctx, fmt.Sprintf("[DoubaoTaskBilling] 使用 task.ApiKey 作为 token_name: %s", tokenName))
	}

	logger.LogInfo(ctx, fmt.Sprintf("Task %s using model name: %s", task.TaskID, modelName))

	// 从任务数据中获取OEM用户折扣（在任务创建时保存的）
	// 视频任务的轮询是在后台进行的，没有HTTP请求上下文，所以需要从任务数据中读取
	oemUserDiscount := 1.0
	if taskData != nil {
		if discount, ok := taskData["oem_user_discount"].(float64); ok && discount > 0 {
			oemUserDiscount = discount
		}
	}

	// 使用 helper.ModelPriceHelper 获取模型价格和倍率信息
	// 注意：传入 nil 作为 context，因为视频任务轮询没有 HTTP 请求上下文
	// OEM 用户折扣已经从 taskData 中获取，不需要 ModelPriceHelper 再获取
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserId:          task.UserId,
		UserGroup:       user.Group,
		UsingGroup:      user.Group,
		UserSetting:     dto.UserSetting{},
	}

	meta := &types.TokenCountMeta{
		MaxTokens: 0,
	}

	// 使用 helper.ModelPriceHelper 获取价格信息
	// 注意：这里传入 nil，ModelPriceHelper 不会应用 OEM 折扣，我们后面手动应用
	priceData, err := helper.ModelPriceHelper(nil, relayInfo, 1, meta)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Failed to get model price for %s: %v", modelName, err))
		priceData = types.PriceData{
			ModelPrice: 0.1,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1.0,
			},
		}
	}

	modelPrice := priceData.ModelPrice
	groupRatio := priceData.GroupRatioInfo.GroupRatio
	modelRatio := priceData.ModelRatio
	completionRatio := priceData.CompletionRatio

	// 记录计费参数（用于调试）
	logger.LogInfo(ctx, fmt.Sprintf("[DoubaoTaskBilling] 计费参数: modelPrice=%.6f, modelRatio=%.6f, completionRatio=%.2f, groupRatio=%.2f, oemUserDiscount=%.4f",
		modelPrice, modelRatio, completionRatio, groupRatio, oemUserDiscount))

	// 根据实际token消耗重新计算quota
	// 注意：由于 ModelPriceHelper 传入了 nil context，返回的 modelRatio 是原始倍率（未应用 OEM 折扣）
	// 所以这里需要手动应用 oemUserDiscount
	var actualQuota int
	if modelPrice == -1 {
		// 按量计费：根据实际token消耗计算，应用OEM用户折扣
		actualQuota = int(float64(taskResult.TotalTokens) * modelRatio * completionRatio * groupRatio * oemUserDiscount)
		logger.LogInfo(ctx, fmt.Sprintf("[DoubaoTaskBilling] 按量计费: %d tokens × %.6f modelRatio × %.2f completionRatio × %.2f groupRatio × %.4f oemUserDiscount = %d quota",
			taskResult.TotalTokens, modelRatio, completionRatio, groupRatio, oemUserDiscount, actualQuota))
	} else {
		// 固定价格：按固定价格计费，应用OEM用户折扣
		actualQuota = int(modelPrice * common.QuotaPerUnit * groupRatio * oemUserDiscount)
		logger.LogInfo(ctx, fmt.Sprintf("[DoubaoTaskBilling] 固定价格计费: %.6f modelPrice × %.2f QuotaPerUnit × %.2f groupRatio × %.4f oemUserDiscount = %d quota",
			modelPrice, common.QuotaPerUnit, groupRatio, oemUserDiscount, actualQuota))
	}

	// 计算quota差值（参考对话的补扣费逻辑）
	quotaDelta := actualQuota - task.Quota

	logger.LogInfo(ctx, fmt.Sprintf("Task %s billing: actual_quota=%d, pre_quota=%d, delta=%d, tokens=%d",
		task.TaskID, actualQuota, task.Quota, quotaDelta, taskResult.TotalTokens))

	// 如果有quota差值，进行补扣费或退费（参考对话的补扣费逻辑）
	if quotaDelta != 0 {
		// 构建RelayInfo用于补扣费
		relayInfo := &relaycommon.RelayInfo{
			UserId:   task.UserId,
			TokenId:  0,  // 视频任务可能没有具体的token ID
			TokenKey: "", // 视频任务可能没有具体的token key
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelId: task.ChannelId,
			},
		}

		if quotaDelta > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("Task %s 需要补扣费：%d", task.TaskID, quotaDelta))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("Task %s 需要退费：%d", task.TaskID, -quotaDelta))
		}

		// 执行补扣费或退费
		err := service.PostConsumeQuota(relayInfo, quotaDelta, task.Quota, true)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("Failed to post consume quota for task %s: %v", task.TaskID, err))
			return err
		}
	}

	// 记录详细的消费日志（参考对话的补扣费日志）
	var logContent string
	if quotaDelta > 0 {
		logContent = fmt.Sprintf("视频任务完成，预扣费后补扣费：%d（实际消耗：%d，预扣费：%d）",
			quotaDelta, actualQuota, task.Quota)
	} else if quotaDelta < 0 {
		logContent = fmt.Sprintf("视频任务完成，预扣费后返还扣费：%d（实际消耗：%d，预扣费：%d）",
			-quotaDelta, actualQuota, task.Quota)
	} else {
		logContent = fmt.Sprintf("视频任务完成，实际消耗token: %d，预扣费与实际消耗一致",
			taskResult.TotalTokens)
	}

	// 构建其他信息
	other := make(map[string]interface{})
	other["task_id"] = task.TaskID
	other["platform"] = task.Platform
	other["action"] = task.Action
	other["actual_tokens"] = taskResult.TotalTokens
	other["model_price"] = modelPrice
	other["model_ratio"] = modelRatio
	other["completion_ratio"] = completionRatio
	other["group_ratio"] = groupRatio
	other["oem_user_discount"] = oemUserDiscount
	other["pre_consumed_quota"] = task.Quota
	other["actual_quota"] = actualQuota
	other["quota_delta"] = quotaDelta
	// 不存储 video_url，避免将 base64 视频数据存储到日志中
	other["video_task"] = true              // 标记为视频任务
	other["billing_type"] = "final_billing" // 标记为最终计费

	// 获取OEM信息用于价格链计算
	var oemCode string
	if taskData != nil {
		if code, ok := taskData["oem_code"].(string); ok && code != "" {
			oemCode = code
		}
	}
	// 如果任务数据中没有保存 oem_code，使用默认值
	if oemCode == "" {
		oemCode = "nebula"
	}

	// 获取厂商名称用于价格链计算
	vendorName := service.GetVendorNameFromModel(modelName)
	other["vendor_name"] = vendorName
	other["oem_code"] = oemCode

	// 计算价格链（OEM盈利/亏损）
	priceChain := service.CalculatePriceChainForVideoTask(
		oemCode,
		modelName,
		vendorName,
		actualQuota,
		oemUserDiscount,
		groupRatio,
	)

	// 记录消费日志
	otherStr := common.MapToJsonStr(other)
	consumeLog := &model.Log{
		UserId:           task.UserId,
		Username:         user.Username,
		CreatedAt:        common.GetTimestamp(),
		Type:             model.LogTypeConsume,
		Content:          logContent,
		ChannelId:        task.ChannelId,
		PromptTokens:     0,
		CompletionTokens: taskResult.TotalTokens,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            actualQuota,
		UseTime:          int(task.FinishTime - task.StartTime),
		IsStream:         false,
		Group:            user.Group,
		TokenId:          tokenId,
		Ip:               "",
		Other:            otherStr,
	}

	// 设置价格链信息
	if priceChain != nil {
		consumeLog.OemId = priceChain.OemId
		consumeLog.OfficialQuota = priceChain.OfficialQuota
		consumeLog.CostQuota = priceChain.CostQuota
		consumeLog.SystemQuota = priceChain.SystemQuota
		consumeLog.UserQuota = priceChain.UserQuota
		consumeLog.PlatformProfit = priceChain.PlatformProfit
		consumeLog.OemSubsidy = priceChain.OemSubsidy
	}

	// 插入消费日志
	if err := model.LOG_DB.Create(consumeLog).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("Failed to insert consume log for task %s: %v", task.TaskID, err))
	}

	// 处理OEM盈利/亏损（如果有价格链信息）
	if priceChain != nil && priceChain.OemSubsidy != 0 {
		// OemSubsidy > 0 表示OEM盈利，< 0 表示OEM亏损
		err := model.ProcessOemSubsidy(nil, oemCode, priceChain.OemSubsidy, consumeLog.Id, task.UserId)
		if err != nil {
			logger.LogError(ctx, fmt.Sprintf("[DoubaoTaskBilling] 处理OEM盈利失败: %v", err))
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("[DoubaoTaskBilling] OEM盈利处理完成: oemCode=%s, profit=%d", oemCode, priceChain.OemSubsidy))
		}
	}

	logger.LogInfo(ctx, fmt.Sprintf("Task %s billing completed successfully", task.TaskID))

	// 扣费成功后，标记已处理
	if taskData != nil {
		taskData["billing_processed"] = true
		if updatedData, err := json.Marshal(taskData); err == nil {
			task.Data = updatedData
			if err := task.Update(); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[DoubaoTaskBilling] Failed to update task billing flag: %v", err))
			}
		}
	}

	return nil
}

func redactVideoResponseBody(body []byte) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	resp, _ := m["response"].(map[string]any)
	if resp != nil {
		delete(resp, "bytesBase64Encoded")
		if v, ok := resp["video"].(string); ok {
			resp["video"] = truncateBase64(v)
		}
		if vs, ok := resp["videos"].([]any); ok {
			for i := range vs {
				if vm, ok := vs[i].(map[string]any); ok {
					delete(vm, "bytesBase64Encoded")
				}
			}
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return b
}

func truncateBase64(s string) string {
	const maxKeep = 256
	if len(s) <= maxKeep {
		return s
	}
	return s[:maxKeep] + "..."
}

// isVeoModel 判断是否为 Veo 模型
func isVeoModel(modelName string) bool {
	if modelName == "" {
		common.SysLog("[isVeoModel] ❌ modelName is empty")
		return false
	}
	result := strings.Contains(strings.ToLower(modelName), "veo")
	common.SysLog(fmt.Sprintf("[isVeoModel] 🔍 判断模型: %s → 结果: %v", modelName, result))
	return result
}

// isSora2Model 判断是否为 Sora-2 模型
func isSora2Model(modelName string) bool {
	if modelName == "" {
		common.SysLog("[isSora2Model] ❌ modelName is empty")
		return false
	}
	result := strings.Contains(strings.ToLower(modelName), "sora-2") ||
		strings.Contains(strings.ToLower(modelName), "sora2") ||
		modelName == "sora-2"
	common.SysLog(fmt.Sprintf("[isSora2Model] 🔍 判断模型: %s → 结果: %v", modelName, result))
	return result
}

// isWan25Model 判断是否为阿里云万相 wan2.5 模型
func isWan25Model(modelName string) bool {
	if modelName == "" {
		common.SysLog("[isWan25Model] ❌ modelName is empty")
		return false
	}
	result := strings.Contains(strings.ToLower(modelName), "wan2.5") ||
		strings.Contains(modelName, "wan2.5-i2v")
	common.SysLog(fmt.Sprintf("[isWan25Model] 🔍 判断模型: %s → 结果: %v", modelName, result))
	return result
}

// handleVeoTaskBilling 处理 Veo 任务的按秒扣费
func handleVeoTaskBilling(ctx context.Context, task *model.Task, channel *model.Channel) error {
	logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] ✅ 开始处理 Veo 扣费 - TaskID: %s, ModelName: %s", task.TaskID, task.ModelName))

	// 防重复扣费：检查是否已经处理过
	var taskData map[string]interface{}
	if task.Data != nil {
		logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] task.Data 长度: %d bytes", len(task.Data)))
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] 成功解析 task.Data，字段数: %d", len(taskData)))
			if billingProcessed, ok := taskData["billing_processed"].(bool); ok && billingProcessed {
				logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] Task %s already billed, skip", task.TaskID))
				return nil
			}
		} else {
			logger.LogError(ctx, fmt.Sprintf("[VeoTaskBilling] 解析 task.Data 失败: %v", err))
			return fmt.Errorf("unmarshal task data failed: %w", err)
		}
	} else {
		logger.LogWarn(ctx, fmt.Sprintf("[VeoTaskBilling] ⚠️ task.Data is nil for task %s", task.TaskID))
	}

	// 1. 从 task.Data 提取 requested_seconds
	var requestedSeconds int

	if taskData != nil {
		logger.LogInfo(ctx, "[VeoTaskBilling] 🔍 查找 requested_seconds 字段...")
		// 提取 requested_seconds
		if rs, ok := taskData["requested_seconds"].(float64); ok {
			requestedSeconds = int(rs)
			logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] ✅ 找到 requested_seconds (float64): %d", requestedSeconds))
		} else if rs, ok := taskData["requested_seconds"].(int); ok {
			requestedSeconds = rs
			logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] ✅ 找到 requested_seconds (int): %d", requestedSeconds))
		} else {
			logger.LogWarn(ctx, fmt.Sprintf("[VeoTaskBilling] ⚠️ 未找到 requested_seconds，taskData 内容: %+v", taskData))
		}
	}
	requestedSeconds = sanitizeVideoSeconds(requestedSeconds)
	if taskData != nil {
		taskData["requested_seconds"] = requestedSeconds
	}

	if requestedSeconds <= 0 {
		logger.LogError(ctx, fmt.Sprintf("[VeoTaskBilling] ❌ Invalid requested_seconds: %d for task %s", requestedSeconds, task.TaskID))
		return fmt.Errorf("invalid requested_seconds: %d", requestedSeconds)
	}

	generateAudio := false
	if taskData != nil {
		if val, ok := taskData["generate_audio"]; ok {
			if parsed, ok2 := boolFromInterface(val); ok2 {
				generateAudio = parsed
			}
		} else if val, ok := taskData["generateAudio"]; ok {
			if parsed, ok2 := boolFromInterface(val); ok2 {
				generateAudio = parsed
			}
		}
	}
	sampleCount := 1
	if taskData != nil {
		if sc, ok := taskData["sampleCount"]; ok {
			if parsed, ok2 := toInt(sc); ok2 && parsed > 0 {
				sampleCount = parsed
			}
		} else if sc, ok := taskData["sample_count"]; ok {
			if parsed, ok2 := toInt(sc); ok2 && parsed > 0 {
				sampleCount = parsed
			}
		}
	}
	if sampleCount <= 0 {
		sampleCount = 1
	}
	if taskData != nil {
		taskData["sampleCount"] = sampleCount
	}

	logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] ✅ Task %s 扣费时长: %d 秒", task.TaskID, requestedSeconds))

	// 2. 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %w", task.UserId, err)
	}

	// 3. 提取模型名称和token信息
	var modelName string
	var tokenName string
	var tokenId int

	if taskData != nil {
		if model, ok := taskData["model"].(string); ok && model != "" {
			modelName = model
		}
		if token, ok := taskData["token_name"].(string); ok && token != "" {
			tokenName = token
		}
		if token, ok := taskData["token_id"].(float64); ok {
			tokenId = int(token)
		}
	}

	if modelName == "" {
		modelName = task.ModelName
	}
	if modelName == "" {
		modelName = fmt.Sprintf("%s-%s", task.Platform, task.Action)
	}
	if isFastVeoModel(modelName) {
		generateAudio = true
	}

	// 如果没有找到 token_name，使用 task.ApiKey 作为备选（新增的表字段）
	if tokenName == "" && task.ApiKey != "" {
		tokenName = task.ApiKey
		logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] 使用 task.ApiKey 作为 token_name: %s", tokenName))
	}

	logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] Model: %s, Token: %s", modelName, tokenName))

	// 4. 获取模型价格（VideoModelPricePerSecond）
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserId:          task.UserId,
		UserGroup:       user.Group,
		UsingGroup:      user.Group,
	}

	meta := &types.TokenCountMeta{
		MaxTokens: 0,
	}

	priceData, err := helper.ModelPriceHelper(nil, relayInfo, 1, meta)
	if err != nil {
		return fmt.Errorf("get model price failed: %w", err)
	}

	// 5. 计算实际扣费（按秒价格 * 秒数 * 组倍率）
	videoPrice, hasVideoPrice := ratio_setting.GetVideoModelPricePerSecondWithAudio(modelName, generateAudio)
	if !hasVideoPrice || videoPrice <= 0 {
		return fmt.Errorf("video price per second not configured for model: %s", modelName)
	}

	groupRatio := priceData.GroupRatioInfo.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// 获取OEM用户折扣（从任务数据中获取）
	oemUserDiscount := 1.0
	if taskData != nil {
		if discount, ok := taskData["oem_user_discount"].(float64); ok && discount > 0 {
			oemUserDiscount = discount
		}
	}

	actualQuota := int(videoPrice * float64(requestedSeconds) * float64(sampleCount) * common.QuotaPerUnit * groupRatio * oemUserDiscount)

	logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] Billing: %d seconds × $%.4f/sec × sampleCount %d × group_ratio %.2f × oem_user_discount %.4f = quota %d",
		requestedSeconds, videoPrice, sampleCount, groupRatio, oemUserDiscount, actualQuota))

	// 6. 扣除用户余额（带重试机制）
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = model.DecreaseUserQuota(task.UserId, actualQuota)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			logger.LogWarn(ctx, fmt.Sprintf("[VeoTaskBilling] Retry %d/%d for task %s: %v", i+1, maxRetries, task.TaskID, err))
			time.Sleep(time.Millisecond * 100 * time.Duration(i+1))
		}
	}
	if err != nil {
		return fmt.Errorf("decrease user quota failed after %d retries: %w", maxRetries, err)
	}

	// 7. 记录日志
	logContent := fmt.Sprintf("Veo视频任务完成，实际生成 %d 秒，扣费 quota: %d", requestedSeconds, actualQuota)

	other := make(map[string]interface{})
	other["task_id"] = task.TaskID
	other["model_name"] = modelName
	other["token_name"] = tokenName
	other["token_id"] = tokenId
	other["video_seconds"] = requestedSeconds
	other["video_price_per_second"] = videoPrice
	other["group_ratio"] = groupRatio
	other["oem_user_discount"] = oemUserDiscount
	other["sample_count"] = sampleCount
	other["actual_quota"] = actualQuota
	// 不存储 video_url，避免将 base64 视频数据存储到日志中（Veo 使用 FailReason 存储视频数据）
	other["billing_type"] = "per_second"
	other["generate_audio"] = generateAudio
	other["platform"] = task.Platform
	other["action"] = task.Action
	other["video_task"] = true

	otherStr := common.MapToJsonStr(other)
	consumeLog := &model.Log{
		UserId:           task.UserId,
		Username:         user.Username,
		CreatedAt:        common.GetTimestamp(),
		Type:             model.LogTypeConsume,
		Content:          logContent,
		ChannelId:        task.ChannelId,
		PromptTokens:     0,
		CompletionTokens: requestedSeconds, // 用秒数作为 completion tokens
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            actualQuota,
		UseTime:          int(task.FinishTime - task.StartTime),
		IsStream:         false,
		Group:            user.Group,
		TokenId:          tokenId,
		Ip:               "",
		Other:            otherStr,
	}

	// 插入消费日志
	if err := model.LOG_DB.Create(consumeLog).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("[VeoTaskBilling] Failed to insert consume log for task %s: %v", task.TaskID, err))
	}

	logger.LogInfo(ctx, fmt.Sprintf("[VeoTaskBilling] Successfully billed task %s: %d quota", task.TaskID, actualQuota))

	// 扣费成功后，标记已处理
	if taskData != nil {
		taskData["billing_processed"] = true
		if updatedData, err := json.Marshal(taskData); err == nil {
			task.Data = updatedData
			if err := task.Update(); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[VeoTaskBilling] Failed to update task billing flag: %v", err))
			}
		}
	}

	return nil
}

func boolFromInterface(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		normalized := strings.TrimSpace(strings.ToLower(v))
		if normalized == "true" || normalized == "1" || normalized == "yes" {
			return true, true
		}
		if normalized == "false" || normalized == "0" || normalized == "no" {
			return false, true
		}
	case float64:
		return v != 0, true
	case float32:
		return v != 0, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint:
		return v != 0, true
	}
	return false, false
}

func toInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i, true
		}
	}
	return 0, false
}

func isFastVeoModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "-fast-")
}

func sanitizeVideoSeconds(seconds int) int {
	switch seconds {
	case 4, 6, 8, 12:
		return seconds
	default:
		return 4
	}
}

// handleSora2TaskBilling 处理 Sora-2 任务的按秒扣费
func handleSora2TaskBilling(ctx context.Context, task *model.Task, channel *model.Channel) error {
	logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] Start billing for task %s", task.TaskID))

	// 防重复扣费：检查是否已经处理过
	var taskData map[string]interface{}
	if task.Data != nil {
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			if billingProcessed, ok := taskData["billing_processed"].(bool); ok && billingProcessed {
				logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] Task %s already billed, skip", task.TaskID))
				return nil
			}
		} else {
			return fmt.Errorf("unmarshal task data failed: %w", err)
		}
	}

	// 1. 从 task.Data 提取时长（优先使用 requested_seconds，其次使用 API 返回的 seconds）
	var requestedSeconds int

	if taskData != nil {
		// 优先：从提交时保存的 requested_seconds 获取
		if rs, ok := taskData["requested_seconds"].(float64); ok {
			requestedSeconds = int(rs)
		} else if rs, ok := taskData["requested_seconds"].(int); ok {
			requestedSeconds = rs
		}

		// 备用：如果 requested_seconds 为空，从 API 返回的 seconds 字段获取
		if requestedSeconds <= 0 {
			// Sora-2 API 返回的数据中有 seconds 字段（字符串格式）
			if secondsStr, ok := taskData["seconds"].(string); ok && secondsStr != "" {
				if sec, err := strconv.Atoi(secondsStr); err == nil && sec > 0 {
					requestedSeconds = sec
					logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] 从 API 返回的 seconds 字段获取时长: %d秒", requestedSeconds))
				}
			}
		}
	}

	if requestedSeconds <= 0 {
		logger.LogError(ctx, fmt.Sprintf("[Sora2TaskBilling] Invalid requested_seconds: %d for task %s", requestedSeconds, task.TaskID))
		return fmt.Errorf("invalid requested_seconds: %d", requestedSeconds)
	}

	logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] Task %s requested duration: %d seconds", task.TaskID, requestedSeconds))

	// 2. 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %w", task.UserId, err)
	}

	// 3. 提取模型名称和token信息
	var modelName string
	var tokenName string
	var tokenId int

	if taskData != nil {
		if model, ok := taskData["model"].(string); ok && model != "" {
			modelName = model
		}
		if token, ok := taskData["token_name"].(string); ok && token != "" {
			tokenName = token
		}
		if token, ok := taskData["token_id"].(float64); ok {
			tokenId = int(token)
		}
	}

	if modelName == "" {
		modelName = task.ModelName
	}
	if modelName == "" {
		modelName = "sora-2"
	}

	// 如果没有找到 token_name，使用 task.ApiKey 作为备选（新增的表字段）
	if tokenName == "" && task.ApiKey != "" {
		tokenName = task.ApiKey
		logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] 使用 task.ApiKey 作为 token_name: %s", tokenName))
	}

	logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] Model: %s, Token: %s", modelName, tokenName))

	// 4. 获取模型价格
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserId:          task.UserId,
		UserGroup:       user.Group,
		UsingGroup:      user.Group,
	}

	meta := &types.TokenCountMeta{
		MaxTokens: 0,
	}

	priceData, err := helper.ModelPriceHelper(nil, relayInfo, 1, meta)
	if err != nil {
		return fmt.Errorf("get model price failed: %w", err)
	}

	// 5. 计算实际扣费
	videoPrice, hasVideoPrice := ratio_setting.GetVideoModelPricePerSecond(modelName)
	if !hasVideoPrice || videoPrice <= 0 {
		return fmt.Errorf("video price per second not configured for model: %s", modelName)
	}

	groupRatio := priceData.GroupRatioInfo.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// 获取OEM用户折扣（从任务数据中获取）
	oemUserDiscount := 1.0
	if taskData != nil {
		if discount, ok := taskData["oem_user_discount"].(float64); ok && discount > 0 {
			oemUserDiscount = discount
		}
	}

	actualQuota := int(videoPrice * float64(requestedSeconds) * common.QuotaPerUnit * groupRatio * oemUserDiscount)

	logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] Billing: %d seconds × $%.4f/sec × group_ratio %.2f × oem_user_discount %.4f = quota %d",
		requestedSeconds, videoPrice, groupRatio, oemUserDiscount, actualQuota))

	// 6. 扣除用户余额
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = model.DecreaseUserQuota(task.UserId, actualQuota)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			logger.LogWarn(ctx, fmt.Sprintf("[Sora2TaskBilling] Retry %d/%d for task %s: %v", i+1, maxRetries, task.TaskID, err))
			time.Sleep(time.Millisecond * 100 * time.Duration(i+1))
		}
	}
	if err != nil {
		return fmt.Errorf("decrease user quota failed after %d retries: %w", maxRetries, err)
	}

	// 7. 记录日志
	logContent := fmt.Sprintf("Sora-2视频任务完成，实际生成 %d 秒，扣费 quota: %d", requestedSeconds, actualQuota)

	other := make(map[string]interface{})
	other["task_id"] = task.TaskID
	other["model_name"] = modelName
	other["token_name"] = tokenName
	other["token_id"] = tokenId
	other["video_seconds"] = requestedSeconds
	other["video_price_per_second"] = videoPrice
	other["group_ratio"] = groupRatio
	other["oem_user_discount"] = oemUserDiscount
	other["actual_quota"] = actualQuota
	// 不存储 video_url，避免将 base64 视频数据存储到日志中（使用 FailReason 存储视频数据）
	other["billing_type"] = "per_second"
	other["platform"] = task.Platform
	other["action"] = task.Action
	other["video_task"] = true

	otherStr := common.MapToJsonStr(other)
	consumeLog := &model.Log{
		UserId:           task.UserId,
		Username:         user.Username,
		CreatedAt:        common.GetTimestamp(),
		Type:             model.LogTypeConsume,
		Content:          logContent,
		ChannelId:        task.ChannelId,
		PromptTokens:     0,
		CompletionTokens: requestedSeconds,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            actualQuota,
		UseTime:          int(task.FinishTime - task.StartTime),
		IsStream:         false,
		Group:            user.Group,
		TokenId:          tokenId,
		Ip:               "",
		Other:            otherStr,
	}

	// 插入消费日志
	if err := model.LOG_DB.Create(consumeLog).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("[Sora2TaskBilling] Failed to insert consume log for task %s: %v", task.TaskID, err))
	}

	logger.LogInfo(ctx, fmt.Sprintf("[Sora2TaskBilling] Successfully billed task %s: %d quota", task.TaskID, actualQuota))

	// 扣费成功后，标记已处理
	if taskData != nil {
		taskData["billing_processed"] = true
		if updatedData, err := json.Marshal(taskData); err == nil {
			task.Data = updatedData
			if err := task.Update(); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[Sora2TaskBilling] Failed to update task billing flag: %v", err))
			}
		}
	}

	return nil
}

// handleWan25TaskBilling 处理阿里云万相 wan2.5 任务的按分辨率和秒数扣费
func handleWan25TaskBilling(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo, channel *model.Channel) error {
	logger.LogInfo(ctx, fmt.Sprintf("[Wan25TaskBilling] ✅ 开始处理 wan2.5 扣费 - TaskID: %s, ModelName: %s", task.TaskID, task.ModelName))

	// 防重复扣费：检查是否已经处理过
	var taskData map[string]interface{}
	if task.Data != nil {
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			if billingProcessed, ok := taskData["billing_processed"].(bool); ok && billingProcessed {
				logger.LogInfo(ctx, fmt.Sprintf("[Wan25TaskBilling] Task %s already billed, skip", task.TaskID))
				return nil
			}
		} else {
			logger.LogError(ctx, fmt.Sprintf("[Wan25TaskBilling] 解析 task.Data 失败: %v", err))
			return fmt.Errorf("unmarshal task data failed: %w", err)
		}
	}

	// wan2.5 使用 Usage 信息（包含 duration 和 resolution）
	if taskResult.Usage == nil {
		logger.LogError(ctx, fmt.Sprintf("[Wan25TaskBilling] ❌ taskResult.Usage is nil for task %s", task.TaskID))
		return fmt.Errorf("taskResult.Usage is nil")
	}

	actualDuration := taskResult.Usage.Duration
	resolution := taskResult.Usage.Resolution

	logger.LogInfo(ctx, fmt.Sprintf("[Wan25TaskBilling] Usage信息 - Duration: %d秒, Resolution: %s, VideoCount: %d",
		actualDuration, resolution, taskResult.Usage.VideoCount))

	// 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %w", task.UserId, err)
	}

	// 提取模型名称和token信息
	var modelName string
	var tokenName string
	var tokenId int

	if taskData != nil {
		if model, ok := taskData["model"].(string); ok && model != "" {
			modelName = model
		}
		if token, ok := taskData["token_name"].(string); ok && token != "" {
			tokenName = token
		}
		if token, ok := taskData["token_id"].(float64); ok {
			tokenId = int(token)
		}
	}

	if modelName == "" {
		modelName = task.ModelName
	}
	if modelName == "" {
		// 如果还是空，根据任务类型推断（但应该不会到这里）
		// 默认使用 i2v，但实际应该从 task.ModelName 获取
		modelName = "wan2.5-i2v-preview"
		logger.LogWarn(ctx, "[Wan25TaskBilling] ⚠️ modelName 为空，使用默认值，实际应该从 task.ModelName 获取")
	}
	if tokenName == "" && task.ApiKey != "" {
		tokenName = task.ApiKey
	}

	// 获取按分辨率的价格
	videoPrice, hasVideoPrice := ratio_setting.GetVideoModelPriceByResolution(modelName, resolution)
	if !hasVideoPrice || videoPrice <= 0 {
		logger.LogError(ctx, fmt.Sprintf("[Wan25TaskBilling] ❌ 未找到模型价格配置: model=%s, resolution=%s", modelName, resolution))
		return fmt.Errorf("video price not configured for model %s resolution %s", modelName, resolution)
	}

	// 获取组倍率
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserId:          task.UserId,
		UserGroup:       user.Group,
		UsingGroup:      user.Group,
	}
	meta := &types.TokenCountMeta{MaxTokens: 0}
	priceData, err := helper.ModelPriceHelper(nil, relayInfo, 1, meta)
	if err != nil {
		return fmt.Errorf("get model price failed: %w", err)
	}

	groupRatio := priceData.GroupRatioInfo.GroupRatio
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// 获取OEM用户折扣（从任务数据中获取）
	oemUserDiscount := 1.0
	if taskData != nil {
		if discount, ok := taskData["oem_user_discount"].(float64); ok && discount > 0 {
			oemUserDiscount = discount
		}
	}

	// 计算实际扣费：价格/秒 * 秒数 * 组倍率 * OEM用户折扣
	actualQuota := int(videoPrice * float64(actualDuration) * common.QuotaPerUnit * groupRatio * oemUserDiscount)

	logger.LogInfo(ctx, fmt.Sprintf("[Wan25TaskBilling] Billing: %d seconds × $%.4f/sec (resolution=%s) × group_ratio %.2f × oem_user_discount %.4f = quota %d",
		actualDuration, videoPrice, resolution, groupRatio, oemUserDiscount, actualQuota))

	// 扣除用户余额
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err = model.DecreaseUserQuota(task.UserId, actualQuota)
		if err == nil {
			break
		}
		if i < maxRetries-1 {
			logger.LogWarn(ctx, fmt.Sprintf("[Wan25TaskBilling] Retry %d/%d for task %s: %v", i+1, maxRetries, task.TaskID, err))
			time.Sleep(time.Millisecond * 100 * time.Duration(i+1))
		}
	}
	if err != nil {
		return fmt.Errorf("decrease user quota failed after %d retries: %w", maxRetries, err)
	}

	// 记录日志
	logContent := fmt.Sprintf("wan2.5视频任务完成，实际生成 %d 秒（分辨率%s），扣费 quota: %d", actualDuration, resolution, actualQuota)

	other := make(map[string]interface{})
	other["task_id"] = task.TaskID
	other["model_name"] = modelName
	other["token_name"] = tokenName
	other["token_id"] = tokenId
	other["video_seconds"] = actualDuration
	other["video_resolution"] = resolution
	other["video_price_per_second"] = videoPrice
	other["group_ratio"] = groupRatio
	other["oem_user_discount"] = oemUserDiscount
	other["actual_quota"] = actualQuota
	other["billing_type"] = "per_second_per_resolution"
	other["platform"] = task.Platform
	other["action"] = task.Action
	other["video_task"] = true

	otherStr := common.MapToJsonStr(other)
	consumeLog := &model.Log{
		UserId:           task.UserId,
		Username:         user.Username,
		CreatedAt:        common.GetTimestamp(),
		Type:             model.LogTypeConsume,
		Content:          logContent,
		ChannelId:        task.ChannelId,
		PromptTokens:     0,
		CompletionTokens: actualDuration, // 用秒数作为 completion tokens
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            actualQuota,
		UseTime:          int(task.FinishTime - task.StartTime),
		IsStream:         false,
		Group:            user.Group,
		TokenId:          tokenId,
		Ip:               "",
		Other:            otherStr,
	}

	// 插入消费日志
	if err := model.LOG_DB.Create(consumeLog).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("[Wan25TaskBilling] Failed to insert consume log for task %s: %v", task.TaskID, err))
	}

	logger.LogInfo(ctx, fmt.Sprintf("[Wan25TaskBilling] ✅ Successfully billed task %s: %d quota", task.TaskID, actualQuota))

	// 扣费成功后，标记已处理
	if taskData != nil {
		taskData["billing_processed"] = true
		if updatedData, err := json.Marshal(taskData); err == nil {
			task.Data = updatedData
			if err := task.Update(); err != nil {
				logger.LogError(ctx, fmt.Sprintf("[Wan25TaskBilling] Failed to update task billing flag: %v", err))
			}
		}
	}

	return nil
}
