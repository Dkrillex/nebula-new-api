package controller

import (
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/logger"
	"one-api/model"
	"time"

	"github.com/gin-gonic/gin"
)

func VideoProxy(c *gin.Context) {
	taskID := c.Param("task_id")
	genID := c.Param("gen_id") // 可选，用于 Azure Sora

	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "task_id is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	common.SysLog(fmt.Sprintf("[VideoProxy] 获取视频内容 - TaskID: %s, GenID: %s", taskID, genID))

	task, exists, err := model.GetByOnlyTaskId(taskID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to query task %s: %s", taskID, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to query task",
				"type":    "server_error",
			},
		})
		return
	}
	if !exists || task == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get task %s: task not found", taskID))
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Task not found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	if task.Status != model.TaskStatusSuccess {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Task is not completed yet, current status: %s", task.Status),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	channel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to get channel %d: %s", task.ChannelId, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to retrieve channel information",
				"type":    "server_error",
			},
		})
		return
	}

	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	var videoURL string
	var authHeader string

	// 根据渠道类型构建不同的 URL
	if channel.Type == constant.ChannelTypeAzure {
		// Azure Sora 2: 使用 video_id (taskID)，加上 variant=video 和 api-version 参数
		apiVersion := channel.Other
		if apiVersion == "" {
			apiVersion = "preview"
		}
		// 如果配置了模型特定的 API 版本，优先使用模型特定的版本
		otherSettings := channel.GetOtherSettings()
		if len(otherSettings.AzureModelApiVersions) > 0 && task.ModelName != "" {
			if modelApiVersion, exists := otherSettings.AzureModelApiVersions[task.ModelName]; exists && modelApiVersion != "" {
				apiVersion = modelApiVersion
				common.SysLog(fmt.Sprintf("[VideoProxy] 使用模型特定的 API 版本: %s (模型: %s)", apiVersion, task.ModelName))
			}
		}
		videoURL = fmt.Sprintf("%s/openai/v1/videos/%s/content?variant=video&api-version=%s", baseURL, taskID, apiVersion)
		authHeader = "Api-key"
		common.SysLog(fmt.Sprintf("[VideoProxy] Azure Sora2 URL: %s", videoURL))
	} else {
		// 原生 OpenAI Sora 或其他渠道
		videoURL = fmt.Sprintf("%s/v1/videos/%s/content?variant=video", baseURL, task.TaskID)
		authHeader = "Authorization"
		common.SysLog(fmt.Sprintf("[VideoProxy] 其他渠道 URL: %s", videoURL))
	}

	client := &http.Client{
		Timeout: 8 * time.Minute, // 8分钟超时，视频文件可能较大
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, videoURL, nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create request for %s: %s", videoURL, err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to create proxy request",
				"type":    "server_error",
			},
		})
		return
	}

	// 设置认证头
	if authHeader == "Api-key" {
		req.Header.Set("Api-key", channel.Key)
	} else {
		req.Header.Set("Authorization", "Bearer "+channel.Key)
	}

	common.SysLog(fmt.Sprintf("[VideoProxy] 发送请求到上游 - URL: %s", videoURL))

	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to fetch video from %s: %s", videoURL, err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to fetch video content",
				"type":    "server_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	common.SysLog(fmt.Sprintf("[VideoProxy] 上游响应状态码: %d", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d for %s, body: %s",
			resp.StatusCode, videoURL, string(bodyBytes)))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Upstream service returned status %d", resp.StatusCode),
				"type":    "server_error",
				"details": string(bodyBytes),
			},
		})
		return
	}

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	c.Writer.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	c.Writer.WriteHeader(resp.StatusCode)

	// 流式传输视频内容
	written, err := io.Copy(c.Writer, resp.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to stream video content: %s", err.Error()))
	} else {
		common.SysLog(fmt.Sprintf("[VideoProxy] 成功传输视频内容，大小: %d bytes", written))
	}
}
