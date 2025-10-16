package controller

import (
	"context"
	"encoding/json"
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
	"one-api/setting/ratio_setting"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
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
	resp, err := adaptor.FetchTask(baseURL, channel.Key, map[string]any{
		"task_id": taskId,
		"action":  task.Action,
	})
	if err != nil {
		return fmt.Errorf("fetchTask failed for task %s: %w", taskId, err)
	}
	//if resp.StatusCode != http.StatusOK {
	//return fmt.Errorf("get Video Task status code: %d", resp.StatusCode)
	//}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("readAll failed for task %s: %w", taskId, err)
	}

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
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	} else {
		task.Data = redactVideoResponseBody(responseBody)
		//// 保留原有的token信息，合并新的响应数据
		//var existingData map[string]interface{}
		//var newData map[string]interface{}
		//
		//// 解析现有数据
		//if task.Data != nil {
		//	if err := json.Unmarshal(task.Data, &existingData); err != nil {
		//		existingData = make(map[string]interface{})
		//	}
		//} else {
		//	existingData = make(map[string]interface{})
		//}
		//
		//// 解析新响应数据
		//if err := json.Unmarshal(responseBody, &newData); err != nil {
		//	// 如果解析失败，直接使用原始响应体
		//	task.Data = responseBody
		//} else {
		//	// 保留原有的token信息
		//	if tokenName, exists := existingData["token_name"]; exists {
		//		newData["token_name"] = tokenName
		//	}
		//	if tokenId, exists := existingData["token_id"]; exists {
		//		newData["token_id"] = tokenId
		//	}
		//
		//	// 合并数据并更新
		//	if mergedData, err := json.Marshal(newData); err == nil {
		//		task.Data = mergedData
		//	} else {
		//		task.Data = responseBody
		//	}
		//}
	}

	now := time.Now().Unix()
	if taskResult.Status == "" {
		return fmt.Errorf("task %s status is empty", taskId)
	}
	task.Status = model.TaskStatus(taskResult.Status)
	switch taskResult.Status {
	case model.TaskStatusSubmitted:
		task.Progress = "10%"
	case model.TaskStatusQueued:
		task.Progress = "20%"
	case model.TaskStatusInProgress:
		task.Progress = "30%"
		if task.StartTime == 0 {
			task.StartTime = now
		}
	case model.TaskStatusSuccess:
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		if !(len(taskResult.Url) > 5 && taskResult.Url[:5] == "data:") {
			task.FailReason = taskResult.Url
		}

		// 如果返回了 total_tokens 并且配置了模型倍率(非固定价格),则重新计费
		if taskResult.TotalTokens > 0 {
			// 获取模型名称
			var taskData map[string]interface{}
			if err := json.Unmarshal(task.Data, &taskData); err == nil {
				if modelName, ok := taskData["model"].(string); ok && modelName != "" {
					// 获取模型价格和倍率
					modelRatio, hasRatioSetting, _ := ratio_setting.GetModelRatio(modelName)

					// 只有配置了倍率(非固定价格)时才按 token 重新计费
					if hasRatioSetting && modelRatio > 0 {
						// 获取用户和组的倍率信息
						user, err := model.GetUserById(task.UserId, false)
						if err == nil {
							groupRatio := ratio_setting.GetGroupRatio(user.Group)
							userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(user.Group, user.Group)

							var finalGroupRatio float64
							if hasUserGroupRatio {
								finalGroupRatio = userGroupRatio
							} else {
								finalGroupRatio = groupRatio
							}

							// 计算实际应扣费额度: totalTokens * modelRatio * groupRatio
							actualQuota := int(float64(taskResult.TotalTokens) * modelRatio * finalGroupRatio)

							// 计算差额
							preConsumedQuota := task.Quota
							quotaDelta := actualQuota - preConsumedQuota

							if quotaDelta > 0 {
								// 需要补扣费
								logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费后补扣费：%s（实际消耗：%s，预扣费：%s，tokens：%d）",
									task.TaskID,
									logger.LogQuota(quotaDelta),
									logger.LogQuota(actualQuota),
									logger.LogQuota(preConsumedQuota),
									taskResult.TotalTokens,
								))
								if err := model.DecreaseUserQuota(task.UserId, quotaDelta); err != nil {
									logger.LogError(ctx, fmt.Sprintf("补扣费失败: %s", err.Error()))
								} else {
									model.UpdateUserUsedQuotaAndRequestCount(task.UserId, quotaDelta)
									model.UpdateChannelUsedQuota(task.ChannelId, quotaDelta)
									task.Quota = actualQuota // 更新任务记录的实际扣费额度

									// 记录消费日志
									logContent := fmt.Sprintf("视频任务成功补扣费，模型倍率 %.2f，分组倍率 %.2f，tokens %d，预扣费 %s，实际扣费 %s，补扣费 %s",
										modelRatio, finalGroupRatio, taskResult.TotalTokens,
										logger.LogQuota(preConsumedQuota), logger.LogQuota(actualQuota), logger.LogQuota(quotaDelta))
									model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
								}
							} else if quotaDelta < 0 {
								// 需要退还多扣的费用
								refundQuota := -quotaDelta
								logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费后返还：%s（实际消耗：%s，预扣费：%s，tokens：%d）",
									task.TaskID,
									logger.LogQuota(refundQuota),
									logger.LogQuota(actualQuota),
									logger.LogQuota(preConsumedQuota),
									taskResult.TotalTokens,
								))
								if err := model.IncreaseUserQuota(task.UserId, refundQuota, false); err != nil {
									logger.LogError(ctx, fmt.Sprintf("退还预扣费失败: %s", err.Error()))
								} else {
									task.Quota = actualQuota // 更新任务记录的实际扣费额度

									// 记录退款日志
									logContent := fmt.Sprintf("视频任务成功退还多扣费用，模型倍率 %.2f，分组倍率 %.2f，tokens %d，预扣费 %s，实际扣费 %s，退还 %s",
										modelRatio, finalGroupRatio, taskResult.TotalTokens,
										logger.LogQuota(preConsumedQuota), logger.LogQuota(actualQuota), logger.LogQuota(refundQuota))
									model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
								}
							} else {
								// quotaDelta == 0, 预扣费刚好准确
								logger.LogInfo(ctx, fmt.Sprintf("视频任务 %s 预扣费准确（%s，tokens：%d）",
									task.TaskID, logger.LogQuota(actualQuota), taskResult.TotalTokens))
							}
						}
					}
				}
			}
		}
	case model.TaskStatusFailure:
		task.Status = model.TaskStatusFailure
		task.Progress = "100%"
		if task.FinishTime == 0 {
			task.FinishTime = now
		}
		task.FailReason = taskResult.Reason
		logger.LogInfo(ctx, fmt.Sprintf("Task %s failed: %s", task.TaskID, task.FailReason))
		quota := task.Quota
		if quota != 0 {
			if err := model.IncreaseUserQuota(task.UserId, quota, false); err != nil {
				logger.LogError(ctx, "Failed to increase user quota: "+err.Error())
			}
			logContent := fmt.Sprintf("Video async task failed %s, refund %s", task.TaskID, logger.LogQuota(quota))
			model.RecordLog(task.UserId, model.LogTypeSystem, logContent)
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

// handleVideoTaskBilling 处理视频任务完成后的实际token消耗补扣费
func handleVideoTaskBilling(ctx context.Context, task *model.Task, taskResult *relaycommon.TaskInfo, channel *model.Channel) error {
	// 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %v", task.UserId, err)
	}

	// 获取原始模型名称和token信息
	var modelName string
	var tokenName string
	var tokenId int
	if task.Data != nil {
		// 尝试从任务数据中提取原始模型名称和token信息
		var taskData map[string]interface{}
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
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
	}

	// 如果没有找到模型名称，使用平台-动作组合作为备选
	if modelName == "" {
		modelName = fmt.Sprintf("%s-%s", task.Platform, task.Action)
	}

	logger.LogInfo(ctx, fmt.Sprintf("Task %s using model name: %s", task.TaskID, modelName))

	// 使用 helper.ModelPriceHelper 获取模型价格和倍率信息
	// 构建 RelayInfo 用于价格查询
	relayInfo := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserId:          task.UserId,
		UserGroup:       user.Group,
		UsingGroup:      user.Group,
		UserSetting:     dto.UserSetting{},
	}

	// 构建 TokenCountMeta
	meta := &types.TokenCountMeta{
		MaxTokens: 0, // 视频任务不需要max_tokens
	}

	// 使用 helper.ModelPriceHelper 获取价格信息
	priceData, err := helper.ModelPriceHelper(nil, relayInfo, 1, meta)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Failed to get model price for %s: %v", modelName, err))
		// 使用默认价格作为备选
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

	// 根据实际token消耗重新计算quota
	var actualQuota int
	if modelPrice == -1 {
		// 按量计费：根据实际token消耗计算
		actualQuota = int(float64(taskResult.TotalTokens) * modelRatio * completionRatio * groupRatio)
	} else {
		// 固定价格：按固定价格计费
		actualQuota = int(modelPrice * common.QuotaPerUnit * groupRatio)
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
	other["pre_consumed_quota"] = task.Quota
	other["actual_quota"] = actualQuota
	other["quota_delta"] = quotaDelta
	other["video_url"] = taskResult.Url
	other["video_task"] = true              // 标记为视频任务
	other["billing_type"] = "final_billing" // 标记为最终计费

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

	// 插入消费日志
	if err := model.LOG_DB.Create(consumeLog).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("Failed to insert consume log for task %s: %v", task.TaskID, err))
	}

	logger.LogInfo(ctx, fmt.Sprintf("Task %s billing completed successfully", task.TaskID))
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
