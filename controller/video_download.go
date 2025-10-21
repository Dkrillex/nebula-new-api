package controller

import (
	"encoding/base64"
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

// VideoDownloadBase64 下载视频并返回 base64 编码（用于前端直接展示）
func VideoDownloadBase64(c *gin.Context) {
	taskID := c.Param("task_id")
	genID := c.Param("gen_id")

	if taskID == "" || genID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "task_id and generation_id are required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	common.SysLog(fmt.Sprintf("[VideoDownload] 开始下载视频 base64 - TaskID: %s, GenID: %s", taskID, genID))

	// 获取用户ID
	userId := c.GetInt("id")

	// 查询任务
	task, exists, err := model.GetByTaskId(userId, taskID)
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
		logger.LogError(c.Request.Context(), fmt.Sprintf("Task not found: %s", taskID))
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Task not found",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	common.SysLog(fmt.Sprintf("[VideoDownload] 找到任务 - TaskID: %s, ChannelID: %d", task.TaskID, task.ChannelId))

	// 获取渠道信息
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

	// 构建 Azure 视频下载 URL
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	apiVersion := channel.Other
	if apiVersion == "" {
		apiVersion = "preview"
	}

	videoURL := fmt.Sprintf("%s/openai/v1/video/generations/%s/content/video?api-version=%s",
		baseURL, genID, apiVersion)

	common.SysLog(fmt.Sprintf("[VideoDownload] Azure 视频 URL: %s", videoURL))

	// 创建 HTTP 客户端，设置超时时间（视频文件较大，需要较长超时）
	client := &http.Client{
		Timeout: 8 * time.Minute, // 8分钟超时
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, videoURL, nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to create request: %s", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to create download request",
				"type":    "server_error",
			},
		})
		return
	}

	// 设置认证头
	if channel.Type == constant.ChannelTypeAzure {
		req.Header.Set("Api-key", channel.Key)
	} else {
		req.Header.Set("Authorization", "Bearer "+channel.Key)
	}

	common.SysLog("[VideoDownload] 开始从 Azure 下载视频...")

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to download video: %s", err.Error()))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": "Failed to download video from upstream",
				"type":    "server_error",
			},
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.LogError(c.Request.Context(), fmt.Sprintf("Upstream returned status %d: %s", resp.StatusCode, string(bodyBytes)))
		c.JSON(http.StatusBadGateway, gin.H{
			"error": gin.H{
				"message": fmt.Sprintf("Upstream service returned status %d", resp.StatusCode),
				"type":    "server_error",
				"details": string(bodyBytes),
			},
		})
		return
	}

	// 读取视频数据
	videoData, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Failed to read video data: %s", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to read video data",
				"type":    "server_error",
			},
		})
		return
	}

	common.SysLog(fmt.Sprintf("[VideoDownload] 视频下载成功，大小: %d bytes (%.2f MB)",
		len(videoData), float64(len(videoData))/1024/1024))

	// 转换为 base64
	videoBase64 := base64.StdEncoding.EncodeToString(videoData)

	common.SysLog(fmt.Sprintf("[VideoDownload] Base64 编码完成，长度: %d", len(videoBase64)))

	// 返回 JSON 格式的 base64 数据
	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"generation_id": genID,
		"task_id":       task.TaskID,
		"format":        "mp4",
		"size":          len(videoData),
		"base64":        videoBase64,
		"data_url":      fmt.Sprintf("data:video/mp4;base64,%s", videoBase64),
	})

	common.SysLog(fmt.Sprintf("[VideoDownload] 响应已发送 - GenID: %s", genID))
}
