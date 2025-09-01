package middleware

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	relayconstant "one-api/relay/constant"
	"strings"
)

func DoubaoRequestConvert() func(c *gin.Context) {
	return func(c *gin.Context) {
		// 解析原始请求
		var originalReq map[string]interface{}
		if err := common.UnmarshalBodyReusable(c, &originalReq); err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, "Invalid request body")
			return
		}

		// 提取模型和提示词
		model, _ := originalReq["model"].(string)
		prompt, _ := originalReq["prompt"].(string)

		// 构建统一请求格式
		unifiedReq := map[string]interface{}{
			"model":    model,
			"prompt":   prompt,
			"metadata": originalReq,
		}

		// 处理图片输入
		if content, ok := originalReq["content"].([]interface{}); ok {
			var imageInputs []map[string]interface{}
			for _, item := range content {
				if contentItem, ok := item.(map[string]interface{}); ok {
					if contentType, ok := contentItem["type"].(string); ok && contentType == "image_url" {
						if imageURL, ok := contentItem["image_url"].(map[string]interface{}); ok {
							if url, ok := imageURL["url"].(string); ok {
								imageInputs = append(imageInputs, map[string]interface{}{
									"url": url,
								})
							}
						}
					}
				}
			}
			if len(imageInputs) > 0 {
				unifiedReq["image_inputs"] = imageInputs
			}
		}

		// 序列化请求
		jsonData, err := json.Marshal(unifiedReq)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusInternalServerError, "Failed to marshal request body")
			return
		}

		// 更新请求体和路径
		c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonData))
		c.Set(common.KeyRequestBody, jsonData)

		// 判断是否为文生视频还是图生视频
		if len(unifiedReq) > 0 {
			if imageInputs, ok := unifiedReq["image_inputs"]; ok && imageInputs != nil {
				// 有图片输入，设置为图生视频
				c.Set("action", constant.TaskActionGenerate)
			} else {
				// 无图片输入，设置为文生视频
				c.Set("action", constant.TaskActionTextGenerate)
			}
		}

		// 处理任务查询请求
		if strings.Contains(c.Request.URL.Path, "/videos/") && c.Request.Method == http.MethodGet {
			// 提取task_id
			pathParts := strings.Split(c.Request.URL.Path, "/")
			if len(pathParts) > 0 {
				taskId := pathParts[len(pathParts)-1]
				c.Request.URL.Path = "/v1/video/generations/" + taskId
				c.Set("task_id", taskId)
				c.Set("relay_mode", relayconstant.RelayModeVideoFetchByID)
			}
		} else {
			// 生成请求
			c.Request.URL.Path = "/v1/video/generations"
		}

		c.Next()
	}
}
