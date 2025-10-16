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
	"one-api/relay/channel"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"one-api/setting/ratio_setting"
	"strconv"
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
	// 如果预扣费为0或过小，设置一个合理的默认值（参考Midjourney的预扣费逻辑）
	if quota == 0 || quota < 1000 { // 小于1000 quota（约0.002美元）认为过小
		// 视频任务设置一个基础的预扣费，避免余额检查失效
		quota = int(0.1 * common.QuotaPerUnit) // 默认0.1美元的预扣费
		common.SysLog(fmt.Sprintf("Video task pre-consume quota is too small (%d), using default value: %d", priceData.ShouldPreConsumedQuota, quota))
	}
	modelPrice := priceData.ModelPrice
	groupRatio := priceData.GroupRatioInfo.GroupRatio
	hasUserGroupRatio := priceData.GroupRatioInfo.GroupSpecialRatio != 0
	userGroupRatio := priceData.GroupRatioInfo.GroupSpecialRatio

	// 检查用户余额是否足够并进行预扣费（参考Midjourney的预扣费逻辑）
	userQuota, err := model.GetUserQuota(info.UserId, false)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
		return
	}
	if userQuota-quota < 0 {
		taskErr = service.TaskErrorWrapperLocal(errors.New("user quota is not enough"), "quota_not_enough", http.StatusForbidden)
		return
	}

	// 先进行预扣费
	if quota > 0 {
		err := service.PostConsumeQuota(info, quota, 0, true)
		if err != nil {
			taskErr = service.TaskErrorWrapper(err, "pre_consume_quota_failed", http.StatusInternalServerError)
			return
		}
		// 记录预扣费日志
		tokenName := c.GetString("token_name")
		var logContent string
		if modelPrice == -1 {
			logContent = fmt.Sprintf("模型按量计费，预扣费token: %d，分组倍率 %.2f，操作 %s", common.Max(1, common.PreConsumedQuota), groupRatio, info.Action)
		} else {
			logContent = fmt.Sprintf("模型固定价格 %.2f，分组倍率 %.2f，操作 %s", modelPrice, groupRatio, info.Action)
		}
		other := make(map[string]interface{})
		other["model_price"] = modelPrice
		other["group_ratio"] = groupRatio
		if hasUserGroupRatio {
			other["user_group_ratio"] = userGroupRatio
		}
		other["video_task"] = true
		other["billing_type"] = "pre_consume"
		if modelPrice == -1 {
			other["model_ratio"] = 1.0
			other["completion_ratio"] = 1.0
		} else {
			other["model_ratio"] = 0.0
			other["completion_ratio"] = 0.0
		}
		model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
			ChannelId: info.ChannelId,
			ModelName: modelName,
			TokenName: tokenName,
			Quota:     quota,
			Content:   logContent,
			TokenId:   info.TokenId,
			Group:     info.UsingGroup,
			Other:     other,
		})
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, quota)
		model.UpdateChannelUsedQuota(info.ChannelId, quota)
	}

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
		// 请求构建失败，返还预扣费
		if quota > 0 {
			common.SysLog(fmt.Sprintf("Request build failed, returning pre-consumed quota: %d", quota))
			err := service.PostConsumeQuota(info, -quota, 0, true)
			if err != nil {
				common.SysLog("error returning pre-consumed quota: " + err.Error())
			}
		}
		taskErr = service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
		return
	}
	// do request
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		// 请求发送失败，返还预扣费
		if quota > 0 {
			common.SysLog(fmt.Sprintf("Request send failed, returning pre-consumed quota: %d", quota))
			err := service.PostConsumeQuota(info, -quota, 0, true)
			if err != nil {
				common.SysLog("error returning pre-consumed quota: " + err.Error())
			}
		}
		taskErr = service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
		return
	}
	// handle response
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		truncatedResponseBody := common.TruncateBase64Content(string(responseBody))
		// HTTP请求失败，返还预扣费
		if quota > 0 {
			common.SysLog(fmt.Sprintf("HTTP request failed, returning pre-consumed quota: %d", quota))
			err := service.PostConsumeQuota(info, -quota, 0, true)
			if err != nil {
				common.SysLog("error returning pre-consumed quota: " + err.Error())
			}
			// 记录返还日志
			tokenName := c.GetString("token_name")
			logContent := fmt.Sprintf("HTTP请求失败，返还预扣费: %d", quota)
			other := make(map[string]interface{})
			other["video_task"] = true
			other["billing_type"] = "refund"
			other["task_platform"] = platform
			other["task_action"] = info.Action
			model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
				ChannelId: info.ChannelId,
				ModelName: modelName,
				TokenName: tokenName,
				Quota:     -quota,
				Content:   logContent,
				TokenId:   info.TokenId,
				Group:     info.UsingGroup,
				Other:     other,
			})
		}
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", truncatedResponseBody), "fail_to_fetch_task", resp.StatusCode)
		return
	}

	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		// 任务提交失败，返还预扣费
		if quota > 0 {
			common.SysLog(fmt.Sprintf("Task submission failed, returning pre-consumed quota: %d", quota))
			err := service.PostConsumeQuota(info, -quota, 0, true)
			if err != nil {
				common.SysLog("error returning pre-consumed quota: " + err.Error())
			}
			// 记录返还日志
			tokenName := c.GetString("token_name")
			logContent := fmt.Sprintf("任务提交失败，返还预扣费: %d", quota)
			other := make(map[string]interface{})
			other["video_task"] = true
			other["billing_type"] = "refund"
			other["task_platform"] = platform
			other["task_action"] = info.Action
			model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
				ChannelId: info.ChannelId,
				ModelName: modelName,
				TokenName: tokenName,
				Quota:     -quota,
				Content:   logContent,
				TokenId:   info.TokenId,
				Group:     info.UsingGroup,
				Other:     other,
			})
		}
		return
	}
	info.ConsumeQuota = true

	// insert task
	task := model.InitTask(platform, info)
	task.TaskID = taskID
	task.Quota = quota // 记录预扣费金额
	task.Action = info.Action

	// 保存token信息到任务数据中，用于后续补扣费日志记录
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

	// 添加token信息
	tokenName := c.GetString("token_name")
	tokenId := c.GetInt("token_id")
	taskDataMap["token_name"] = tokenName
	taskDataMap["token_id"] = tokenId

	// 记录调试信息
	common.SysLog(fmt.Sprintf("[RelayTaskSubmit] 保存token信息: token_name=%s, token_id=%d", tokenName, tokenId))

	// 重新序列化为JSON
	if updatedData, err := json.Marshal(taskDataMap); err == nil {
		task.Data = updatedData
		common.SysLog("[RelayTaskSubmit] 任务数据已更新，包含token信息")
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

	return nil
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

	// 检查任务是否成功，如果成功且是豆包火山平台，需要处理实际token消耗和补扣费
	if originTask.Status == model.TaskStatusSuccess && originTask.Platform == "doubao" {
		common.SysLog("[VideoTask] 检测到豆包火山任务成功，开始处理实际token消耗")

		// 获取渠道信息
		channel, err := model.GetChannelById(originTask.ChannelId, true)
		if err != nil {
			common.SysError(fmt.Sprintf("[VideoTask] 获取渠道信息失败: %v", err))
		} else {
			// 解析任务数据中的usage信息
			if originTask.Data != nil {
				var taskData map[string]interface{}
				if err := json.Unmarshal(originTask.Data, &taskData); err == nil {
					// 检查是否有usage信息
					if usageData, ok := taskData["usage"].(map[string]interface{}); ok {
						if totalTokens, exists := usageData["total_tokens"].(float64); exists && totalTokens > 0 {
							common.SysLog(fmt.Sprintf("[VideoTask] 发现实际token消耗: %d", int(totalTokens)))

							// 构建TaskInfo用于补扣费处理
							taskResult := &relaycommon.TaskInfo{
								TaskID:      originTask.TaskID,
								Status:      "SUCCESS",
								TotalTokens: int(totalTokens),
								Url:         originTask.FailReason, // 视频URL存储在FailReason字段中
							}

							// 调用补扣费处理逻辑
							if err := handleVideoTaskBillingInQuery(c, originTask, taskResult, channel); err != nil {
								common.SysError(fmt.Sprintf("[VideoTask] 补扣费处理失败: %v", err))
							} else {
								common.SysLog("[VideoTask] 补扣费处理完成")
							}
						}
					}
				}
			}
		}
	}

	// 转换为统一视频生成接口文档格式
	response := convertToUnifiedVideoResponse(originTask)
	common.SysLog(fmt.Sprintf("[VideoTask] 转换后的响应格式: TaskId=%s, Status=%s, Url=%s",
		response.TaskId, response.Status, response.Url))

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

	// 如枟任务成功且有数据
	if task.Status == model.TaskStatusSuccess && task.Data != nil {
		// 解析原始响应数据（支持各厂商格式）
		var rawData map[string]interface{}
		if err := json.Unmarshal(task.Data, &rawData); err == nil {
			// 尝试从不同厂商格式中提取视频URL
			if videoURL := extractVideoURL(rawData); videoURL != "" {
				response.Url = videoURL
			}
			// 将全部原始数据作为metadata返回，不做任何结构化处理
			response.Metadata = rawData
		}
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

	// 获取原始模型名称
	var modelName string
	if task.Data != nil {
		// 尝试从任务数据中提取原始模型名称
		var taskData map[string]interface{}
		if err := json.Unmarshal(task.Data, &taskData); err == nil {
			if model, ok := taskData["model"].(string); ok && model != "" {
				modelName = model
			}
		}
	}

	// 如果没有找到模型名称，使用平台-动作组合作为备选
	if modelName == "" {
		modelName = fmt.Sprintf("%s-%s", task.Platform, task.Action)
	}

	common.SysLog(fmt.Sprintf("[VideoTask] Task %s using model name: %s", task.TaskID, modelName))

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

		// 豆包火山视频模型：按输出token计费，输入免费
		outputTokens := taskResult.TotalTokens // 豆包返回的total_tokens就是输出token

		// 根据模型价格类型计算实际quota
		if modelPrice == -1 {
			// 按量计费：根据实际token消耗计算
			actualQuota = int(float64(outputTokens) * modelRatio * completionRatio * groupRatio)
		} else {
			// 固定价格：按固定价格计费
			actualQuota = int(float64(outputTokens) * modelPrice * common.QuotaPerUnit * groupRatio)
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
	otherMap["video_url"] = taskResult.Url
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
