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
	"one-api/service"
	"one-api/setting/ratio_setting"

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
	modelPrice, success := ratio_setting.GetModelPrice(modelName, true)
	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelRatioMap()[modelName]
		if !ok {
			modelPrice = 0.1
		} else {
			modelPrice = defaultPrice
		}
	}

	// 预扣
	groupRatio := ratio_setting.GetGroupRatio(info.UsingGroup)
	var ratio float64
	userGroupRatio, hasUserGroupRatio := ratio_setting.GetGroupGroupRatio(info.UserGroup, info.UsingGroup)
	if hasUserGroupRatio {
		ratio = modelPrice * userGroupRatio
	} else {
		ratio = modelPrice * groupRatio
	}
	userQuota, err := model.GetUserQuota(info.UserId, false)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "get_user_quota_failed", http.StatusInternalServerError)
		return
	}
	quota := int(ratio * common.QuotaPerUnit)
	if userQuota-quota < 0 {
		taskErr = service.TaskErrorWrapperLocal(errors.New("user quota is not enough"), "quota_not_enough", http.StatusForbidden)
		return
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
		taskErr = service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
		return
	}
	// do request
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
		return
	}
	// handle response
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		taskErr = service.TaskErrorWrapper(fmt.Errorf(string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
		return
	}

	defer func() {
		// release quota
		if info.ConsumeQuota && taskErr == nil {

			err := service.PostConsumeQuota(info, quota, 0, true)
			if err != nil {
				common.SysLog("error consuming token remain quota: " + err.Error())
			}
			if quota != 0 {
				tokenName := c.GetString("token_name")
				gRatio := groupRatio
				if hasUserGroupRatio {
					gRatio = userGroupRatio
				}
				logContent := fmt.Sprintf("模型固定价格 %.2f，分组倍率 %.2f，操作 %s", modelPrice, gRatio, info.Action)
				other := make(map[string]interface{})
				other["model_price"] = modelPrice
				other["group_ratio"] = groupRatio
				if hasUserGroupRatio {
					other["user_group_ratio"] = userGroupRatio
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
		}
	}()

	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return
	}
	info.ConsumeQuota = true
	// insert task
	task := model.InitTask(platform, info)
	task.TaskID = taskID
	task.Quota = quota
	task.Data = taskData
	task.Action = info.Action
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
	common.SysLog(fmt.Sprintf("[TaskFetch] 请求方法: %s", c.Request.Method))

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

	common.SysLog(fmt.Sprintf("[TaskFetch] 响应构建成功，响应体长度: %d bytes", len(respBody)))
	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		common.SysError(fmt.Sprintf("[TaskFetch] 复制响应体失败: %v", err))
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	common.SysLog("[TaskFetch] RelayTaskFetch 完成")
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
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	common.SysLog("[VideoTask] videoFetchByIDRespBodyBuilder - 开始查询视频任务")

	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
		common.SysLog(fmt.Sprintf("[VideoTask] 从上下文获取task_id: %s", taskId))
	} else {
		common.SysLog(fmt.Sprintf("[VideoTask] 从参数获取task_id: %s", taskId))
	}

	userId := c.GetInt("id")
	common.SysLog(fmt.Sprintf("[VideoTask] 用户ID: %d, 任务ID: %s", userId, taskId))

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

	common.SysLog(fmt.Sprintf("[VideoTask] 开始从数据库查询任务: userId=%d, taskId=%s", userId, taskId))
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

	// 转换为统一视频生成接口文档格式
	common.SysLog("[VideoTask] 开始转换为统一格式")
	response := convertToUnifiedVideoResponse(originTask)
	common.SysLog(fmt.Sprintf("[VideoTask] 转换后的响应格式: TaskId=%s, Status=%s, Url=%s",
		response.TaskId, response.Status, response.Url))

	respBody, err = json.Marshal(response)
	if err != nil {
		common.SysError(fmt.Sprintf("[VideoTask] JSON序列化失败: %v", err))
		taskResp = service.TaskErrorWrapper(err, "json_marshal_failed", http.StatusInternalServerError)
		return
	}
	common.SysLog(fmt.Sprintf("[VideoTask] 查询完成，返回响应长度: %d bytes", len(respBody)))
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
