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
	"one-api/service"
	"one-api/setting/ratio_setting"
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
	} else if taskResult, err = adaptor.ParseTaskResult(responseBody); err != nil {
		return fmt.Errorf("parseTaskResult failed for task %s: %w", taskId, err)
	} else {
		task.Data = responseBody
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
		task.FailReason = taskResult.Url

		// 处理任务成功后的实际token消耗和补扣费
		if taskResult.TotalTokens > 0 {
			logger.LogInfo(ctx, fmt.Sprintf("Task %s succeeded, actual tokens consumed: %d", task.TaskID, taskResult.TotalTokens))

			// 根据实际token消耗进行补扣费处理
			if err := handleVideoTaskBilling(ctx, task, taskResult, channel); err != nil {
				logger.LogError(ctx, fmt.Sprintf("Failed to handle billing for task %s: %v", task.TaskID, err))
			}
		} else {
			logger.LogInfo(ctx, fmt.Sprintf("Task %s succeeded but no token usage reported", task.TaskID))
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

	logger.LogInfo(ctx, fmt.Sprintf("Task %s using model name: %s", task.TaskID, modelName))

	// 获取模型价格和倍率信息
	modelPrice, hasPrice := ratio_setting.GetModelPrice(modelName, true)
	if !hasPrice {
		// 如果找不到具体模型定价，尝试使用默认定价
		defaultPrice, ok := ratio_setting.GetDefaultModelRatioMap()[modelName]
		if !ok {
			modelPrice = 0.1 // 最终默认值
		} else {
			modelPrice = defaultPrice
		}
		logger.LogInfo(ctx, fmt.Sprintf("Model %s price not found, using default: %.2f", modelName, modelPrice))
	}

	// 获取分组倍率
	groupRatio := ratio_setting.GetGroupRatio(user.Group)

	// 根据实际token消耗重新计算quota
	// 对于视频任务，通常按固定价格计费，不是按token计费
	// 但如果有实际token消耗，我们可以记录下来
	actualQuota := int(modelPrice * common.QuotaPerUnit * groupRatio)

	// 计算quota差值
	quotaDelta := actualQuota - task.Quota

	logger.LogInfo(ctx, fmt.Sprintf("Task %s billing: actual_quota=%d, pre_quota=%d, delta=%d, tokens=%d",
		task.TaskID, actualQuota, task.Quota, quotaDelta, taskResult.TotalTokens))

	// 如果有quota差值，进行补扣费或退费
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

	// 记录详细的消费日志
	logContent := fmt.Sprintf("视频任务完成，实际消耗token: %d，模型价格: %.2f，分组倍率: %.2f，预扣费: %d，实际扣费: %d",
		taskResult.TotalTokens, modelPrice, groupRatio, task.Quota, actualQuota)

	// 构建其他信息
	other := make(map[string]interface{})
	other["task_id"] = task.TaskID
	other["platform"] = task.Platform
	other["action"] = task.Action
	other["actual_tokens"] = taskResult.TotalTokens
	other["model_price"] = modelPrice
	other["group_ratio"] = groupRatio
	other["pre_consumed_quota"] = task.Quota
	other["actual_quota"] = actualQuota
	other["quota_delta"] = quotaDelta
	other["video_url"] = taskResult.Url

	// 记录消费日志
	otherStr := common.MapToJsonStr(other)
	consumeLog := &model.Log{
		UserId:           task.UserId,
		Username:         user.Username,
		CreatedAt:        common.GetTimestamp(),
		Type:             model.LogTypeConsume,
		Content:          logContent,
		ChannelId:        task.ChannelId,
		PromptTokens:     taskResult.TotalTokens,
		CompletionTokens: 0,
		TokenName:        user.Username,
		ModelName:        modelName,
		Quota:            actualQuota,
		UseTime:          int(task.FinishTime - task.StartTime),
		IsStream:         false,
		Group:            user.Group,
		TokenId:          0,
		Ip:               "",
		Other:            otherStr,
	}

	// 插入消费日志
	if err := model.LOG_DB.Create(consumeLog).Error; err != nil {
		logger.LogError(ctx, fmt.Sprintf("Failed to insert consume log for task %s: %v", task.TaskID, err))
	}

	// 记录系统日志
	systemLogContent := fmt.Sprintf("视频任务 %s 补扣费完成，实际消耗token: %d，quota差值: %d",
		task.TaskID, taskResult.TotalTokens, quotaDelta)
	model.RecordLog(task.UserId, model.LogTypeSystem, systemLogContent)

	logger.LogInfo(ctx, fmt.Sprintf("Task %s billing completed successfully", task.TaskID))
	return nil
}
