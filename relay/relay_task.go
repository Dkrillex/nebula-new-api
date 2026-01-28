package relay

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

/*
Task 任务通过平台、Action 区分任务
*/
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	info.InitChannelMeta(c)
	// ensure TaskRelayInfo is initialized to avoid nil dereference when accessing embedded fields
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}

	info.InitChannelMeta(c)
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	// get & validate taskRequest 获取并验证文本请求
	taskErr = adaptor.ValidateRequestAndSetAction(c, info)
	if taskErr != nil {
		return
	}

	// 处理模型映射（在验证请求之后，构建请求体之前）
	// 这样可以确保发送到上游的模型名是映射后的名称
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		common.SysError(fmt.Sprintf("[RelayTaskSubmit] 模型映射失败: %v", err))
		// 映射失败不阻塞请求，继续使用原始模型名
	} else if info.IsModelMapped {
		common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 模型映射成功: %s -> %s", info.OriginModelName, info.UpstreamModelName))
	}

	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}
	// 视频任务使用模型名称用于后续计费
	_ = modelName

	// 使用 helper.ModelPriceHelper 获取模型价格和预扣费信息
	meta := &types.TokenCountMeta{
		MaxTokens: 0, // 视频任务不需要max_tokens
	}
	priceData, err := helper.ModelPriceHelper(c, info, 1, meta) // 使用1作为基础token数
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "get_model_price_failed", http.StatusInternalServerError)
		return
	}

	quota := priceData.ShouldPreConsumedQuota

	// 尝试从请求参数中获取视频秒数（sora-2按秒计费）
	videoSeconds := 4 // 默认4秒
	if secondsVal, exists := c.Get("video_seconds"); exists {
		if seconds, ok := secondsVal.(int); ok && seconds > 0 {
			videoSeconds = seconds
		}
	}
	videoSeconds = sanitizeVideoSeconds(videoSeconds)
	c.Set("video_seconds", videoSeconds)

	generateAudio := false
	if reqVal, exists := c.Get("task_request"); exists {
		if req, ok := reqVal.(relaycommon.TaskSubmitReq); ok {
			generateAudio = req.GenerateAudio
			if req.Metadata != nil {
				if val, ok := req.Metadata["generateAudio"]; ok {
					if parsed, ok2 := parseBool(val); ok2 {
						generateAudio = parsed
					}
				} else if val, ok := req.Metadata["generate_audio"]; ok {
					if parsed, ok2 := parseBool(val); ok2 {
						generateAudio = parsed
					}
				}
			}
		}
	}
	if isFastVeoModel(modelName) {
		generateAudio = true
	}

	// 打印请求参数用于调试
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 接收到的参数: model=%s, videoSeconds=%d, platform=%s, action=%s",
		modelName, videoSeconds, platform, info.Action))

	// 视频模型计费优先级：VideoModelPricePerSecond > ModelPrice > ModelRatio
	// 1. 优先检查视频每秒价格
	videoPrice, hasVideoPrice := ratio_setting.GetVideoModelPricePerSecondWithAudio(modelName, generateAudio)
	if hasVideoPrice && videoPrice > 0 {
		// 应用OEM用户折扣到 videoPrice（用于用户实际支付价）
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		if oemUserDiscount != 1.0 {
			videoPrice = videoPrice * oemUserDiscount
			common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 应用OEM用户折扣到videoPrice: oemUserDiscount=%.4f, 原价=%.4f, 折后价=%.4f",
				oemUserDiscount, videoPrice/oemUserDiscount, videoPrice))
		}
		// 按秒计费：价格 * 秒数
		quota = int(videoPrice * float64(videoSeconds) * common.QuotaPerUnit * priceData.GroupRatioInfo.GroupRatio)
		common.SysLog(fmt.Sprintf("[RelayTaskSubmit] Video task per-second billing: %d seconds × $%.4f/sec × group_ratio %.2f = quota %d (generateAudio=%v)",
			videoSeconds, videoPrice, priceData.GroupRatioInfo.GroupRatio, quota, generateAudio))
	} else if priceData.ModelPrice > 0 {
		// 2. 固定价格（按次计费），应用OEM用户折扣
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		quota = int(priceData.ModelPrice * common.QuotaPerUnit * priceData.GroupRatioInfo.GroupRatio * oemUserDiscount)
		common.SysLog(fmt.Sprintf("[RelayTaskSubmit] Video task per-call billing: $%.4f × group_ratio %.2f × oem_user_discount %.4f = quota %d",
			priceData.ModelPrice, priceData.GroupRatioInfo.GroupRatio, oemUserDiscount, quota))
	} else if quota == 0 || quota < 1000 {
		// 3. 如果预扣费为0或过小，设置默认值
		quota = int(0.1 * common.QuotaPerUnit) // 默认0.1美元的预扣费
		common.SysLog(fmt.Sprintf("[RelayTaskSubmit] Video task pre-consume quota is too small (%d), using default value: %d",
			priceData.ShouldPreConsumedQuota, quota))
	}
	modelPrice := priceData.ModelPrice
	groupRatio := priceData.GroupRatioInfo.GroupRatio
	hasUserGroupRatio := priceData.GroupRatioInfo.GroupSpecialRatio != 0
	userGroupRatio := priceData.GroupRatioInfo.GroupSpecialRatio

	// 检查用户余额是否充足（不进行预扣费，等任务成功后再扣费）
	userQuota, err := model.GetUserQuota(info.UserId, false)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
		return
	}
	if userQuota-quota < 0 {
		taskErr = service.TaskErrorWrapperLocal(errors.New("user quota is not enough"), "quota_not_enough", http.StatusForbidden)
		return
	}

	// 视频任务不进行预扣费，只做余额检查
	// 等任务成功后，根据实际生成的时长（metadata.n_seconds）进行扣费
	common.SysLog(fmt.Sprintf("视频任务提交成功，不进行预扣费。预估费用: quota=%d (仅用于余额检查)", quota))

	if info.OriginTaskID != "" {
		originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
		if err != nil {
			taskErr = service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
			return
		}
		if !exist {
			taskErr = service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
			return
		}
		if originTask.ChannelId != info.ChannelId {
			channel, err := model.GetChannelById(originTask.ChannelId, true)
			if err != nil {
				taskErr = service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
				return
			}
			if channel.Status != common.ChannelStatusEnabled {
				return service.TaskErrorWrapperLocal(errors.New("该任务所属渠道已被禁用"), "task_channel_disable", http.StatusBadRequest)
			}
			c.Set("base_url", channel.GetBaseURL())
			c.Set("channel_id", originTask.ChannelId)
			c.Request.Header.Set("Authorization", fmt.Sprintf("Bearer %s", channel.Key))

			info.ChannelBaseUrl = channel.GetBaseURL()
			info.ChannelId = originTask.ChannelId
		}
	}

	// build body
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		// 请求构建失败（无需返还预扣费，因为没有预扣费）
		taskErr = service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
		return
	}

	// 打印构建的请求体用于调试（截断过长的base64内容）
	if bodyBytes, err := io.ReadAll(requestBody); err == nil {
		truncatedBody := common.TruncateBase64Content(string(bodyBytes))
		common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 构建的请求体: %s", truncatedBody))
		// 重新创建reader，因为已经读取过了
		requestBody = bytes.NewReader(bodyBytes)
	}

	// do request
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		// 请求发送失败（无需返还预扣费）
		taskErr = service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
		return
	}

	// 打印响应状态码
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 收到响应: StatusCode=%d", resp.StatusCode))
	// handle response - 接受 200 和 201 状态码（201 Created 用于资源创建）
	if resp != nil && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		responseBody, _ := io.ReadAll(resp.Body)
		truncatedResponseBody := common.TruncateBase64Content(string(responseBody))
		// HTTP请求失败（无需返还预扣费）
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", truncatedResponseBody), "fail_to_fetch_task", resp.StatusCode)
		return
	}

	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		// 任务提交失败（无需返还预扣费）
		return
	}

	// 打印解析后的任务信息
	truncatedTaskData := common.TruncateBase64Content(string(taskData))
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 解析响应成功: taskID=%s, taskData=%s", taskID, truncatedTaskData))

	info.ConsumeQuota = true

	// 提前获取token信息（后面会用到）
	tokenName := c.GetString("token_name")
	tokenId := c.GetInt("token_id")

	// insert task
	task := model.InitTask(platform, info)
	task.TaskID = taskID
	task.Quota = 0 // 不记录预扣费，等任务成功后根据实际情况扣费
	task.Action = info.Action
	task.ModelName = modelName // 保存模型名称到表字段
	task.ApiKey = tokenName    // 保存token名称到api_key字段（用于日志记录）

	// 保存token信息和计费相关信息到任务数据中，用于后续扣费日志记录
	var taskDataMap map[string]interface{}
	if taskData != nil {
		// 如果已有数据，先解析
		if err := json.Unmarshal(taskData, &taskDataMap); err != nil {
			common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 解析taskData失败: %v", err))
			taskDataMap = make(map[string]interface{})
		}
	} else {
		taskDataMap = make(map[string]interface{})
	}

	// 添加token信息到taskDataMap
	taskDataMap["token_name"] = tokenName
	taskDataMap["token_id"] = tokenId

	// 添加计费相关信息（用于后续扣费）
	taskDataMap["model_name"] = modelName
	// 如果按秒计费，使用折扣后的 videoPrice；否则使用 modelPrice（已应用折扣）
	if hasVideoPrice && videoPrice > 0 {
		taskDataMap["model_price"] = videoPrice // 使用折扣后的 videoPrice
		taskDataMap["video_price_per_second"] = videoPrice
	} else {
		taskDataMap["model_price"] = modelPrice // modelPrice 已在 ModelPriceHelperPerCall 中应用折扣
	}
	taskDataMap["group_ratio"] = groupRatio
	if hasUserGroupRatio {
		taskDataMap["user_group_ratio"] = userGroupRatio
	}
	// 保存OEM用户折扣（用于后台轮询时扣费）
	oemUserDiscountForSave := service.GetOemUserDiscountForQuota(c, modelName)
	taskDataMap["oem_user_discount"] = oemUserDiscountForSave
	// 保存OEM代码（用于后台轮询时计算价格链）
	oemCodeForSave := "nebula" // 默认值
	if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
		if codeStr, ok := code.(string); ok && codeStr != "" {
			oemCodeForSave = codeStr
		}
	}
	taskDataMap["oem_code"] = oemCodeForSave
	// 记录保存的 OEM 用户折扣
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 保存OEM信息: modelName=%s, oemCode=%s, oemUserDiscount=%.4f", modelName, oemCodeForSave, oemUserDiscountForSave))
	taskDataMap["requested_seconds"] = videoSeconds // 保存请求的秒数
	taskDataMap["durationSeconds"] = videoSeconds
	taskDataMap["generate_audio"] = generateAudio
	taskDataMap["generateAudio"] = generateAudio
	taskDataMap["billing_pending"] = true // 标记待扣费

	// 记录调试信息
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 保存任务信息: token_name=%s, token_id=%d, model=%s, requested_seconds=%d, model_price=%.2f, group_ratio=%.2f",
		tokenName, tokenId, modelName, videoSeconds, modelPrice, groupRatio))

	// 重新序列化为JSON
	if updatedData, err := json.Marshal(taskDataMap); err == nil {
		task.Data = updatedData
		// 打印最终保存的完整数据（方便调试）
		truncatedData := common.TruncateBase64Content(string(updatedData))
		common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 任务数据已更新，完整数据: %s", truncatedData))
	} else {
		common.SysError(fmt.Sprintf("[RelayTaskSubmit] 序列化任务数据失败: %v", err))
		// 如果序列化失败，至少保存原始数据
		task.Data = taskData
	}

	err = task.Insert()
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "insert_task_failed", http.StatusInternalServerError)
		return
	}

	// 不记录任务提交日志，等任务成功后再扣费并记录日志
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 任务提交完成: TaskID=%s, 不记录日志，等待轮询成功后扣费", taskID))

	return nil
}

func isFastVeoModel(modelName string) bool {
	return strings.Contains(strings.ToLower(modelName), "-fast-")
}

func sanitizeVideoSeconds(seconds int) int {
	switch seconds {
	case 6, 8:
		return seconds
	default:
		return 4
	}
}

func parseBool(value interface{}) (bool, bool) {
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

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:  sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:      sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID: videoFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	common.SysLog(fmt.Sprintf("[TaskFetch] RelayTaskFetch - RelayMode: %d", relayMode))
	common.SysLog(fmt.Sprintf("[TaskFetch] 请求路径: %s", c.Request.URL.Path))

	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		common.SysError(fmt.Sprintf("[TaskFetch] 不支持的RelayMode: %d", relayMode))
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
		return taskResp
	}

	common.SysLog(fmt.Sprintf("[TaskFetch] 找到对应的响应构建器，RelayMode: %d", relayMode))
	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		common.SysError(fmt.Sprintf("[TaskFetch] 响应构建失败: %+v", taskErr))
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		common.SysError(fmt.Sprintf("[TaskFetch] 复制响应体失败: %v", err))
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = json.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "json_marshal_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = json.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "json_marshal_failed", http.StatusInternalServerError)
		return
	}
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	common.SysLog("[VideoTask] videoFetchByIDRespBodyBuilder - 开始查询视频任务")

	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
		common.SysLog(fmt.Sprintf("[VideoTask] 从上下文获取task_id: %s", taskId))
	}

	userId := c.GetInt("id")

	if taskId == "" {
		common.SysError("[VideoTask] 任务ID为空")
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_id is empty"), "task_id_empty", http.StatusBadRequest)
		return
	}

	if userId <= 0 {
		common.SysError("[VideoTask] 用户ID无效")
		taskResp = service.TaskErrorWrapperLocal(errors.New("user_id is invalid"), "user_id_invalid", http.StatusBadRequest)
		return
	}

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		common.SysError(fmt.Sprintf("[VideoTask] 数据库查询失败: %v", err))
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		common.SysError(fmt.Sprintf("[VideoTask] 任务不存在: userId=%d, taskId=%s", userId, taskId))
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	common.SysLog(fmt.Sprintf("[VideoTask] 找到任务记录: ID=%d, TaskID=%s, Status=%s, Platform=%s",
		originTask.ID, originTask.TaskID, originTask.Status, originTask.Platform))
	common.SysLog(fmt.Sprintf("[VideoTask] 任务表字段: ModelName=%s, ApiKey(TokenName)=%s, Quota=%d",
		originTask.ModelName, originTask.ApiKey, originTask.Quota))

	// 检查任务是否成功
	if originTask.Status == model.TaskStatusSuccess {
		common.SysLog("[VideoTask] 任务已成功，返回结果")
		// 不再进行扣费操作，扣费由轮询线程统一处理
	}

	// 转换为统一视频生成接口文档格式
	response := convertToUnifiedVideoResponse(originTask)
	common.SysLog(fmt.Sprintf("[VideoTask] 转换后的响应格式: TaskId=%s, Status=%s, Url=%s",
		response.TaskId, response.Status, common.TruncateBase64Content(fmt.Sprintf("%v", response.Url))))

	respBody, err = json.Marshal(response)
	if err != nil {
		common.SysError(fmt.Sprintf("[VideoTask] JSON序列化失败: %v", err))
		taskResp = service.TaskErrorWrapper(err, "json_marshal_failed", http.StatusInternalServerError)
		return
	}
	return
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		TaskID:     task.TaskID,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Data:       task.Data,
	}
}

// convertToUnifiedVideoResponse 将任务数据转换为统一视频生成接口文档格式
func convertToUnifiedVideoResponse(task *model.Task) *dto.VideoTaskResponse {
	response := &dto.VideoTaskResponse{
		TaskId: task.TaskID,
		Status: convertTaskStatus(string(task.Status)),
		Url:    "",    // 默认为空，成功时填入视频URL
		Format: "mp4", // 默认格式
	}

	ossFiles, hasOSSFiles := model.ParseTaskMediaInfo(task.FailReason)

	// 解析并返回原厂响应数据作为metadata（所有状态都返回）
	if task.Data != nil {
		var rawData map[string]interface{}
		if err := json.Unmarshal(task.Data, &rawData); err == nil {
			// 判断是否为 veo 模型
			modelName := strings.ToLower(task.ModelName)
			isVeo := strings.Contains(modelName, "veo")

			// 如果任务成功，尝试提取视频URL
			if task.Status == model.TaskStatusSuccess {
				if isVeo {
					urls := extractOSSUrls(ossFiles)
					if len(urls) > 0 {
						setVideoTaskResponseURLs(response, urls)
						rawData = injectVideoUrls(rawData, urls)
						common.SysLog(fmt.Sprintf("[VideoTask] Veo media stored in OSS, first url: %s", common.TruncateBase64Content(urls[0])))
					} else if task.FailReason != "" {
						// 兼容历史数据：fail_reason 中保存 data URI
						common.SysLog(fmt.Sprintf("[VideoTask] Veo model detected, fallback to data URI from fail_reason for task %s", task.TaskID))
						setVideoTaskResponseURLs(response, []string{task.FailReason})
						if responseObj, ok := rawData["response"].(map[string]interface{}); ok {
							if videos, ok := responseObj["videos"].([]interface{}); ok && len(videos) > 0 {
								if video, ok := videos[0].(map[string]interface{}); ok {
									if strings.Contains(task.FailReason, ";base64,") {
										base64Part := strings.Split(task.FailReason, ";base64,")[1]
										video["bytesBase64Encoded"] = common.TruncateBase64Content(base64Part)
									}
									video["url"] = task.FailReason
								}
							}
						}
					} else {
						common.SysError("[VideoTask] Veo task success but fail_reason is empty")
					}
				} else {
					// 其他模型：从数据库提取视频URL
					if videoURL := extractVideoURL(rawData); videoURL != "" {
						setVideoTaskResponseURLs(response, []string{videoURL})
					}
				}
			}

			sanitizeVideoMetadata(rawData)
			// 将处理后的数据作为metadata返回
			// 对于Veo，此时metadata中的base64已经被截断
			response.Metadata = rawData
		}
	}

	if response.Url == nil && hasOSSFiles && len(ossFiles) > 0 {
		urls := extractOSSUrls(ossFiles)
		setVideoTaskResponseURLs(response, urls)
	}

	// 如果任务失败，添加错误信息
	if task.Status == model.TaskStatusFailure {
		response.Error = &dto.VideoTaskError{
			Code:    400, // 默认错误码
			Message: getFailureReason(task.FailReason),
		}
	}

	return response
}

func sanitizeVideoMetadata(data map[string]interface{}) {
	if data == nil {
		return
	}
	blockedKeys := []string{
		"billing_pending",
		"billing_processed",
		"group_ratio",
		"model_price",
		"ossFiles",
		"user_group_ratio",
		"token_id",
		"name",
		"oem_code",          // 敏感信息：OEM代码
		"oem_user_discount", // 敏感信息：OEM用户折扣
	}
	for _, key := range blockedKeys {
		delete(data, key)
	}
}

func extractOSSUrls(media []model.TaskMediaInfo) []string {
	urls := make([]string, 0, len(media))
	for _, item := range media {
		if item.OssURL != "" {
			urls = append(urls, item.OssURL)
		}
	}
	return urls
}

func setVideoTaskResponseURLs(response *dto.VideoTaskResponse, urls []string) {
	if len(urls) == 0 {
		return
	}
	if len(urls) == 1 {
		response.Url = urls[0]
	} else {
		response.Url = urls
	}
}

func injectVideoUrls(rawData map[string]interface{}, urls []string) map[string]interface{} {
	if rawData == nil {
		rawData = make(map[string]interface{})
	}
	rawData["sampleCount"] = len(urls)
	responseObj, _ := rawData["response"].(map[string]interface{})
	if responseObj == nil {
		responseObj = make(map[string]interface{})
		rawData["response"] = responseObj
	}
	videos, _ := responseObj["videos"].([]interface{})
	if len(videos) < len(urls) {
		expanded := make([]interface{}, len(urls))
		copy(expanded, videos)
		for i := len(videos); i < len(urls); i++ {
			expanded[i] = map[string]interface{}{}
		}
		videos = expanded
	}
	for idx, url := range urls {
		var video map[string]interface{}
		if idx < len(videos) {
			if existing, ok := videos[idx].(map[string]interface{}); ok {
				video = existing
			}
		}
		if video == nil {
			video = make(map[string]interface{})
			if idx < len(videos) {
				videos[idx] = video
			} else {
				videos = append(videos, video)
			}
		}
		video["url"] = url
		if _, ok := video["mimeType"]; !ok {
			video["mimeType"] = "video/mp4"
		}
		if _, ok := video["encoding"]; !ok {
			video["encoding"] = "mp4"
		}
	}
	responseObj["videos"] = videos
	return rawData
}

// convertTaskStatus 将系统任务状态转换为接口文档规范的状态
func convertTaskStatus(status string) string {
	switch status {
	case "SUBMITTED":
		return "submitted"
	case "QUEUED":
		return "queued"
	case "IN_PROGRESS":
		return "in_progress"
	case "SUCCESS":
		return "succeeded"
	case "FAILURE":
		return "failed"
	default:
		return "unknown"
	}
}

// extractVideoURL 从不同厂商的响应格式中提取视频URL
func extractVideoURL(rawData map[string]interface{}) string {
	// 阿里云万相格式: output.video_url
	if output, ok := rawData["output"].(map[string]interface{}); ok {
		if videoURL, ok := output["video_url"].(string); ok && videoURL != "" {
			common.SysLog(fmt.Sprintf("[VideoTask] 从output.video_url提取URL（阿里云万相格式）: %s", common.TruncateBase64Content(videoURL)))
			return videoURL
		}
	}

	// 豆包格式: content.video_url
	if content, ok := rawData["content"].(map[string]interface{}); ok {
		if videoURL, ok := content["video_url"].(string); ok && videoURL != "" {
			return videoURL
		}
	}

	// 可灵格式: data.task_result.videos[0].url
	if data, ok := rawData["data"].(map[string]interface{}); ok {
		if taskResult, ok := data["task_result"].(map[string]interface{}); ok {
			if videos, ok := taskResult["videos"].([]interface{}); ok && len(videos) > 0 {
				if firstVideo, ok := videos[0].(map[string]interface{}); ok {
					if videoURL, ok := firstVideo["url"].(string); ok && videoURL != "" {
						return videoURL
					}
				}
			}
		}
	}

	// 即梦格式: result.video_url
	if result, ok := rawData["result"].(map[string]interface{}); ok {
		if videoURL, ok := result["video_url"].(string); ok && videoURL != "" {
			return videoURL
		}
	}

	// 直接的url字段
	if videoURL, ok := rawData["url"].(string); ok && videoURL != "" {
		return videoURL
	}

	// 直接的video_url字段
	if videoURL, ok := rawData["video_url"].(string); ok && videoURL != "" {
		return videoURL
	}

	return "" // 未找到视频URL
}

// getFailureReason 获取失败原因，如果为空返回默认消息
func getFailureReason(reason string) string {
	if reason == "" {
		return "任务执行失败"
	}
	return reason
}

// handleVideoTaskBillingInQuery 处理视频任务查询时的补扣费逻辑
func handleVideoTaskBillingInQuery(c *gin.Context, task *model.Task, taskResult *relaycommon.TaskInfo, channel *model.Channel) error {
	// 检查是否已经处理过补扣费（避免重复处理）
	if task.FinishTime > 0 && task.Progress == "100%" {
		// 检查是否已经有补扣费标记
		var taskData map[string]interface{}
		if task.Data != nil {
			if err := json.Unmarshal(task.Data, &taskData); err == nil {
				if _, exists := taskData["billing_processed"]; exists {
					common.SysLog("[VideoTask] 任务已处理过补扣费，跳过")
					return nil
				}
			}
		}
	}

	// 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %v", task.UserId, err)
	}

	// 优先从表字段获取模型名称
	modelName := task.ModelName

	// 如果表字段为空，尝试从任务数据中提取（兼容旧数据）
	if modelName == "" && task.Data != nil {
		var taskData map[string]interface{}
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			if model, ok := taskData["model"].(string); ok && model != "" {
				modelName = model
			}
		}
	}

	// 如果仍然没有找到模型名称，使用平台-动作组合作为备选
	if modelName == "" {
		modelName = fmt.Sprintf("%s-%s", task.Platform, task.Action)
	}

	common.SysLog(fmt.Sprintf("[VideoTask] Task %s using model name: %s (from table: %s)", task.TaskID, modelName, task.ModelName))

	// 豆包火山视频模型计费逻辑：输入免费，按输出计费
	// 根据实际token消耗重新计算quota
	var actualQuota int
	var quotaDelta int

	if taskResult.TotalTokens > 0 {
		// 使用 helper.ModelPriceHelper 获取实时的模型价格和倍率信息
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
		priceData, err := helper.ModelPriceHelper(c, relayInfo, 1, meta)
		if err != nil {
			common.SysLog(fmt.Sprintf("[VideoTask] Failed to get model price for %s: %v", modelName, err))
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

		// 获取OEM用户折扣
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)

		// 豆包火山视频模型：按输出token计费，输入免费
		outputTokens := taskResult.TotalTokens // 豆包返回的total_tokens就是输出token

		// 根据模型价格类型计算实际quota，应用OEM用户折扣
		if modelPrice == -1 {
			// 按量计费：根据实际token消耗计算
			actualQuota = int(float64(outputTokens) * modelRatio * completionRatio * groupRatio * oemUserDiscount)
		} else {
			// 固定价格：按固定价格计费
			actualQuota = int(float64(outputTokens) * modelPrice * common.QuotaPerUnit * groupRatio * oemUserDiscount)
		}

		// 计算quota差值（参考对话的补扣费逻辑）
		quotaDelta = actualQuota - task.Quota

		common.SysLog(fmt.Sprintf("[VideoTask] Task %s billing: output_tokens=%d, model_price=%.2f, group_ratio=%.2f, actual_quota=%d, pre_quota=%d, delta=%d",
			task.TaskID, outputTokens, modelPrice, groupRatio, actualQuota, task.Quota, quotaDelta))

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
				common.SysLog(fmt.Sprintf("[VideoTask] Task %s 需要补扣费：%d", task.TaskID, quotaDelta))
			} else {
				common.SysLog(fmt.Sprintf("[VideoTask] Task %s 需要退费：%d", task.TaskID, -quotaDelta))
			}

			// 执行补扣费或退费
			err := service.PostConsumeQuota(relayInfo, quotaDelta, task.Quota, true)
			if err != nil {
				common.SysError(fmt.Sprintf("[VideoTask] Failed to post consume quota for task %s: %v", task.TaskID, err))
				return err
			}
		}

		// 查找并更新现有的消费日志
		if err := updateExistingConsumeLog(task, taskResult, actualQuota, quotaDelta, outputTokens); err != nil {
			common.SysError(fmt.Sprintf("[VideoTask] Failed to update consume log for task %s: %v", task.TaskID, err))
		}

		// 更新任务数据，标记已处理补扣费
		if task.Data != nil {
			var taskData map[string]interface{}
			if err := json.Unmarshal(task.Data, &taskData); err == nil {
				// 保留原有的token信息
				originalTokenName := taskData["token_name"]
				originalTokenId := taskData["token_id"]

				// 添加计费相关字段
				taskData["billing_processed"] = true
				taskData["billing_processed_at"] = common.GetTimestamp()
				taskData["actual_tokens"] = taskResult.TotalTokens
				taskData["quota_delta"] = quotaDelta

				// 确保token信息不被覆盖
				if originalTokenName != nil {
					taskData["token_name"] = originalTokenName
				}
				if originalTokenId != nil {
					taskData["token_id"] = originalTokenId
				}

				// 更新任务数据
				if updatedData, err := json.Marshal(taskData); err == nil {
					task.Data = updatedData
					if err := task.Update(); err != nil {
						common.SysError(fmt.Sprintf("[VideoTask] Failed to update task data: %v", err))
					}
				}
			}
		}
	} else {
		common.SysLog(fmt.Sprintf("[VideoTask] Task %s succeeded but no token usage reported", task.TaskID))
	}

	common.SysLog(fmt.Sprintf("[VideoTask] Task %s billing completed successfully", task.TaskID))
	return nil
}

// updateExistingConsumeLog 更新现有的消费日志
func updateExistingConsumeLog(task *model.Task, taskResult *relaycommon.TaskInfo, actualQuota, quotaDelta, outputTokens int) error {
	// 查找现有的消费日志（通过task_id在other字段中查找）
	var existingLog model.Log
	err := model.LOG_DB.Where("user_id = ? AND type = ? AND other LIKE ?",
		task.UserId, model.LogTypeConsume, "%\"task_id\":\""+task.TaskID+"\"%").First(&existingLog).Error

	if err != nil {
		common.SysError(fmt.Sprintf("[VideoTask] Failed to find existing log for task %s: %v", task.TaskID, err))
		return err
	}

	// 解析现有的other字段
	var otherMap map[string]interface{}
	if existingLog.Other != "" {
		if err := json.Unmarshal([]byte(existingLog.Other), &otherMap); err != nil {
			common.SysError(fmt.Sprintf("[VideoTask] Failed to parse existing log other field: %v", err))
			otherMap = make(map[string]interface{})
		}
	} else {
		otherMap = make(map[string]interface{})
	}

	// 更新other字段中的信息
	otherMap["billing_processed"] = true
	otherMap["billing_processed_at"] = common.GetTimestamp()
	otherMap["actual_output_tokens"] = outputTokens
	otherMap["actual_quota"] = actualQuota
	otherMap["quota_delta"] = quotaDelta
	// 不存储 video_url，避免将 base64 视频数据存储到日志中
	otherMap["billing_type"] = "final_billing" // 标记为最终计费

	// 添加视频任务特有的字段
	otherMap["video_task"] = true
	otherMap["task_completed"] = true
	otherMap["completion_tokens"] = outputTokens // 用于前端显示

	// 更新日志记录 - 只更新必要的字段，不修改tokenname和模型名称
	existingLog.CompletionTokens = outputTokens // 将输出token放在CompletionTokens字段
	existingLog.Quota = actualQuota
	existingLog.Other = common.MapToJsonStr(otherMap)

	// 更新内容 - 包含补扣费信息
	existingLog.Content = fmt.Sprintf("视频任务 %s 补扣费完成，实际消耗token: %d，quota差值: %d",
		task.TaskID, outputTokens, quotaDelta)

	// 保存更新
	if err := model.LOG_DB.Save(&existingLog).Error; err != nil {
		common.SysError(fmt.Sprintf("[VideoTask] Failed to update existing log: %v", err))
		return err
	}

	common.SysLog(fmt.Sprintf("[VideoTask] Successfully updated existing log for task %s", task.TaskID))
	return nil
}

// handleVideoTaskBillingBySeconds 处理按秒计费的视频任务（sora-2等模型）
func handleVideoTaskBillingBySeconds(c *gin.Context, task *model.Task, taskResult *relaycommon.TaskInfo, channel *model.Channel, actualSeconds int) error {
	common.SysLog(fmt.Sprintf("[VideoTask] 开始处理按秒计费，任务ID: %s，实际秒数: %d", task.TaskID, actualSeconds))

	// 获取用户信息
	user, err := model.GetUserById(task.UserId, false)
	if err != nil {
		return fmt.Errorf("failed to get user %d: %v", task.UserId, err)
	}

	// 优先从表字段获取模型名称和token名称
	modelName := task.ModelName
	tokenName := task.ApiKey // ApiKey字段存储的是token名称

	// 从任务数据中获取其他计费信息
	var taskData map[string]interface{}
	var modelPrice float64
	var groupRatio float64
	var tokenId int

	if task.Data != nil {
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			if price, ok := taskData["model_price"].(float64); ok {
				modelPrice = price
			}
			if ratio, ok := taskData["group_ratio"].(float64); ok {
				groupRatio = ratio
			}
			if tid, ok := taskData["token_id"].(float64); ok {
				tokenId = int(tid)
			}
		}
	}

	// 如果表字段为空，尝试从JSON获取（兼容旧数据）
	if modelName == "" && taskData != nil {
		if name, ok := taskData["model_name"].(string); ok {
			modelName = name
		}
	}
	if tokenName == "" && taskData != nil {
		if tn, ok := taskData["token_name"].(string); ok {
			tokenName = tn
		}
	}

	// 如果仍然没有，使用默认值
	if modelName == "" {
		modelName = fmt.Sprintf("%s-%s", task.Platform, task.Action)
	}
	if groupRatio == 0 {
		groupRatio = 1.0
	}

	common.SysLog(fmt.Sprintf("[VideoTask] 从表字段读取: model_name=%s, api_key(token_name)=%s", task.ModelName, task.ApiKey))

	// 获取模型倍率信息（用于按token计费的视频模型，如doubao）
	modelRatio, hasModelRatio, _ := ratio_setting.GetModelRatio(modelName)
	completionRatio := ratio_setting.GetCompletionRatio(modelName)

	common.SysLog(fmt.Sprintf("[VideoTask] 计费信息: model=%s, price=%.2f, ratio=%.2f, completion_ratio=%.2f, seconds=%d, tokens=%d",
		modelName, modelPrice, groupRatio, completionRatio, actualSeconds, taskResult.TotalTokens))

	// 计算实际quota - 四级优先级判断
	var actualQuota int
	var billingType string
	var videoPricePerSecond float64

	// 1. 优先检查视频每秒价格（sora-2等按秒计费的模型）
	var videoPrice float64
	var hasVideoPrice bool

	// 特殊处理 wan2.5-i2v-preview：根据分辨率获取价格
	if modelName == "wan2.5-i2v-preview" && taskResult.Usage != nil && taskResult.Usage.Resolution != "" {
		videoPrice, hasVideoPrice = ratio_setting.GetVideoModelPriceByResolution(modelName, taskResult.Usage.Resolution)
		common.SysLog(fmt.Sprintf("[VideoTask] wan2.5-i2v-preview resolution-based pricing: %s = $%.4f/sec",
			taskResult.Usage.Resolution, videoPrice))
	} else {
		videoPrice, hasVideoPrice = ratio_setting.GetVideoModelPricePerSecond(modelName)
	}

	// 三层视频每秒价格（仅在 per_second 计费时写入 other）
	var officialVideoPricePerSecond float64
	var oemVideoPricePerSecond float64
	var userVideoPricePerSecond float64

	if hasVideoPrice && videoPrice > 0 && actualSeconds > 0 {
		// 记录原始视频价格（应用OEM用户折扣前）
		officialVideoPrice := videoPrice
		// 应用OEM用户折扣到 videoPrice（用于用户实际支付价）
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		if oemUserDiscount != 1.0 {
			videoPrice = videoPrice * oemUserDiscount
			common.SysLog(fmt.Sprintf("[VideoTask] 应用OEM用户折扣到videoPrice: oemUserDiscount=%.4f, 原价=%.4f, 折后价=%.4f",
				oemUserDiscount, videoPrice/oemUserDiscount, videoPrice))
		}
		// 计算OEM平台视频价格（原厂价格 * OEM折扣）
		var oemVideoPrice float64
		if c != nil {
			oemCode := "nebula"
			if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
			vendorName := service.GetVendorNameFromModel(modelName)
			oemDiscount := model.GetOemDiscountByCode(oemCode, modelName, vendorName)
			if oemDiscount <= 0 {
				oemDiscount = 1.0
			}
			oemVideoPrice = officialVideoPrice * oemDiscount
		} else {
			oemVideoPrice = officialVideoPrice
		}
		// 按秒计费：价格 * 秒数
		actualQuota = int(videoPrice * float64(actualSeconds) * common.QuotaPerUnit * groupRatio)
		billingType = "per_second"
		videoPricePerSecond = videoPrice
		// 暂存三层视频价格，等 other 初始化后再写入
		officialVideoPricePerSecond = officialVideoPrice
		oemVideoPricePerSecond = oemVideoPrice
		userVideoPricePerSecond = videoPricePerSecond
		common.SysLog(fmt.Sprintf("[VideoTask] Per-second billing: %d seconds × $%.4f/sec × group_ratio %.2f × oem_user_discount %.4f = quota %d",
			actualSeconds, videoPrice, groupRatio, oemUserDiscount, actualQuota))
	} else if modelPrice > 0 {
		// 2. 固定价格（按次计费），应用OEM用户折扣
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		actualQuota = int(modelPrice * common.QuotaPerUnit * groupRatio * oemUserDiscount)
		billingType = "per_call"
		common.SysLog(fmt.Sprintf("[VideoTask] Per-call billing: $%.4f × group_ratio %.2f × oem_user_discount %.4f = quota %d",
			modelPrice, groupRatio, oemUserDiscount, actualQuota))
	} else if hasModelRatio && modelRatio > 0 && taskResult.TotalTokens > 0 {
		// 3. 按token计费（doubao等返回tokens的视频模型），应用OEM用户折扣
		// 使用和文本模型相同的计费逻辑：ratio * 2 * tokens / 1M
		inputTokens := taskResult.TotalTokens // doubao只返回total_tokens
		outputTokens := 0
		if completionRatio > 0 {
			// 如果有completion_ratio，按比例分配
			outputTokens = int(float64(inputTokens) * completionRatio)
		}

		inputRatioPrice := modelRatio * 2.0 // 1倍率=0.002刀/1K tokens
		outputRatioPrice := inputRatioPrice
		if completionRatio > 0 {
			outputRatioPrice = inputRatioPrice * completionRatio
		}

		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		actualQuota = int((float64(inputTokens)/1000000)*inputRatioPrice*groupRatio*oemUserDiscount +
			(float64(outputTokens)/1000000)*outputRatioPrice*groupRatio*oemUserDiscount)
		billingType = "per_token"
		common.SysLog(fmt.Sprintf("[VideoTask] Per-token billing: %d tokens × ratio %.2f × group_ratio %.2f × oem_user_discount %.4f = quota %d",
			taskResult.TotalTokens, modelRatio, groupRatio, oemUserDiscount, actualQuota))
	} else if actualSeconds > 0 {
		// 4. 如果没有价格信息且有秒数，使用默认价格（$0.1/秒），应用OEM用户折扣
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		actualQuota = int(0.1 * float64(actualSeconds) * common.QuotaPerUnit * groupRatio * oemUserDiscount)
		billingType = "per_second"
		videoPricePerSecond = 0.1 * oemUserDiscount
		common.SysLog(fmt.Sprintf("[VideoTask] Default per-second billing: %d seconds × $0.1/sec × group_ratio %.2f × oem_user_discount %.4f = quota %d",
			actualSeconds, groupRatio, oemUserDiscount, actualQuota))
	} else {
		// 5. 兜底：使用最小扣费，应用OEM用户折扣
		oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
		actualQuota = int(0.01 * common.QuotaPerUnit * groupRatio * oemUserDiscount)
		billingType = "fallback"
		common.SysLog(fmt.Sprintf("[VideoTask] Fallback billing: $0.01 × group_ratio %.2f × oem_user_discount %.4f = quota %d",
			groupRatio, oemUserDiscount, actualQuota))
	}

	common.SysLog(fmt.Sprintf("[VideoTask] 计算实际费用: seconds=%d, quota=%d", actualSeconds, actualQuota))

	// 构建RelayInfo用于扣费
	relayInfo := &relaycommon.RelayInfo{
		UserId:   task.UserId,
		TokenId:  tokenId,
		TokenKey: "",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId: task.ChannelId,
		},
		UsingGroup: user.Group,
	}

	// 执行扣费
	if actualQuota > 0 {
		err := service.PostConsumeQuota(relayInfo, actualQuota, 0, true)
		if err != nil {
			common.SysError(fmt.Sprintf("[VideoTask] 扣费失败: %v", err))
			return err
		}
		common.SysLog(fmt.Sprintf("[VideoTask] 扣费成功: %d", actualQuota))
	}

	// 记录扣费日志
	other := make(map[string]interface{})
	other["video_task"] = true
	other["billing_type"] = billingType
	other["billing_processed"] = true
	other["billing_processed_at"] = common.GetTimestamp()
	other["task_id"] = task.TaskID
	other["actual_seconds"] = actualSeconds
	other["actual_quota"] = actualQuota
	// 不存储 video_url，避免将 base64 视频数据存储到日志中
	other["model_name"] = modelName
	other["group_ratio"] = groupRatio

	// 记录OEM用户折扣信息（用于溯源）
	oemUserDiscount := service.GetOemUserDiscountForQuota(c, modelName)
	if oemUserDiscount != 1.0 && oemUserDiscount > 0 {
		other["oem_user_discount"] = oemUserDiscount
		oemCode := "nebula"
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		other["oem_code"] = oemCode
		vendorName := service.GetVendorNameFromModel(modelName)
		if vendorName != "" {
			other["vendor_name"] = vendorName
		}
	}

	// 如果按秒计费，使用折扣后的 videoPricePerSecond；否则使用 modelPrice（已应用折扣）
	if billingType == "per_second" && videoPricePerSecond > 0 {
		other["model_price"] = videoPricePerSecond // 使用折扣后的 videoPricePerSecond
		other["video_seconds"] = actualSeconds
		other["video_price_per_second"] = videoPricePerSecond
		// 写入三层视频价格（导出用）
		if officialVideoPricePerSecond > 0 {
			other["official_video_price_per_second"] = officialVideoPricePerSecond
		}
		if oemVideoPricePerSecond > 0 {
			other["oem_video_price_per_second"] = oemVideoPricePerSecond
		}
		if userVideoPricePerSecond > 0 {
			other["video_price_per_second"] = userVideoPricePerSecond
		}
	} else if billingType == "per_token" {
		other["total_tokens"] = taskResult.TotalTokens
		other["model_ratio"] = modelRatio
		if completionRatio > 0 {
			other["completion_ratio"] = completionRatio
		}
	}

	// 根据计费类型记录不同的日志内容和tokens
	var logContent string
	var promptTokens, completionTokens int

	if billingType == "per_token" {
		logContent = fmt.Sprintf("视频任务 %s 实际扣费: %d tokens，quota: %d", task.TaskID, taskResult.TotalTokens, actualQuota)
		promptTokens = 0 // doubao视频没有prompt tokens
		completionTokens = taskResult.TotalTokens
	} else if billingType == "per_second" {
		logContent = fmt.Sprintf("视频任务完成，实际生成 %d 秒，扣费 quota: %d", actualSeconds, actualQuota)
	} else {
		logContent = fmt.Sprintf("视频任务 %s 实际扣费 quota: %d", task.TaskID, actualQuota)
	}

	// 计算价格链条（视频任务可能没有标准tokens，使用0作为默认值）
	priceChain := service.CalculatePriceChainForLog(c, modelName, promptTokens, completionTokens, actualQuota)

	model.RecordConsumeLog(c, task.UserId, model.RecordConsumeLogParams{
		ChannelId:        task.ChannelId,
		ModelName:        modelName,
		TokenName:        tokenName,
		Quota:            actualQuota,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Content:          logContent,
		TokenId:          tokenId,
		Group:            user.Group,
		Other:            other,
		PriceChain:       priceChain,
	})

	// 更新用户和渠道的配额使用情况
	model.UpdateUserUsedQuotaAndRequestCount(task.UserId, actualQuota)
	model.UpdateChannelUsedQuota(task.ChannelId, actualQuota)

	// 更新任务数据，标记已处理扣费
	if taskData != nil {
		taskData["billing_processed"] = true
		taskData["billing_processed_at"] = common.GetTimestamp()
		taskData["actual_seconds"] = actualSeconds
		taskData["actual_quota"] = actualQuota

		if updatedData, err := json.Marshal(taskData); err == nil {
			task.Data = updatedData
			task.Quota = actualQuota // 更新任务记录的quota字段
			if err := task.Update(); err != nil {
				common.SysError(fmt.Sprintf("[VideoTask] 更新任务数据失败: %v", err))
			}
		}
	}

	common.SysLog(fmt.Sprintf("[VideoTask] 任务 %s 按秒计费完成", task.TaskID))
	return nil
}
