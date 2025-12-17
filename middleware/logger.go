package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"one-api/common"

	"github.com/gin-gonic/gin"
)

// responseWriter 包装 gin.ResponseWriter 以捕获响应体
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func SetUpLogger(server *gin.Engine) {
	server.Use(func(c *gin.Context) {
		// 记录请求开始时间
		start := time.Now()

		// 创建响应写入器包装器
		writer := &responseWriter{
			ResponseWriter: c.Writer,
			body:           &bytes.Buffer{},
		}
		c.Writer = writer

		c.Next()

		// 计算延迟时间
		latency := time.Since(start)

		// 获取请求ID
		var requestID string
		if c.Keys != nil {
			if id, ok := c.Keys[common.RequestIdKey]; ok {
				if idStr, ok := id.(string); ok {
					requestID = idStr
				}
			}
		}
		if requestID == "" {
			requestID = "SYSTEM"
		}

		// 构建基本日志信息
		statusCode := c.Writer.Status()
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path

		// 无论状态码如何，都尝试从响应体中提取错误信息
		var errorInfo string
		bodyBytes := writer.body.Bytes()
		if len(bodyBytes) > 0 {
			// 尝试解析 JSON 响应
			var jsonData map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &jsonData); err == nil {
				// 尝试提取错误信息（按优先级顺序）
				// 1. 检查 error.message (OpenAI 格式)
				if errorObj, ok := jsonData["error"].(map[string]interface{}); ok {
					if msg, ok := errorObj["message"].(string); ok && msg != "" {
						errorInfo = msg
					} else if msg, ok := errorObj["description"].(string); ok && msg != "" {
						errorInfo = msg
					}
				}
				// 2. 检查 success=false 的情况（API 错误响应）
				if errorInfo == "" {
					if success, ok := jsonData["success"].(bool); ok && !success {
						if msg, ok := jsonData["message"].(string); ok && msg != "" {
							errorInfo = msg
						}
					}
				}
				// 3. 检查 message 字段（通用错误消息）
				if errorInfo == "" {
					if msg, ok := jsonData["message"].(string); ok && msg != "" {
						errorInfo = msg
					}
				}
				// 4. 检查 description 字段（Midjourney 等格式）
				if errorInfo == "" {
					if desc, ok := jsonData["description"].(string); ok && desc != "" {
						errorInfo = desc
					}
				}
				// 5. 检查其他可能的错误字段
				if errorInfo == "" {
					if errMsg, ok := jsonData["error_msg"].(string); ok && errMsg != "" {
						errorInfo = errMsg
					} else if errStr, ok := jsonData["err"].(string); ok && errStr != "" {
						errorInfo = errStr
					} else if msg, ok := jsonData["msg"].(string); ok && msg != "" {
						errorInfo = msg
					}
				}
				// 如果提取到错误信息，截断过长的内容
				if len(errorInfo) > 200 {
					errorInfo = errorInfo[:200] + "..."
				}
			} else {
				// 如果不是 JSON，检查响应体是否看起来像错误信息
				bodyStr := string(bodyBytes)
				// 如果响应体很短且包含错误关键词，可能是错误信息
				if len(bodyStr) < 500 && (strings.Contains(strings.ToLower(bodyStr), "error") ||
					strings.Contains(strings.ToLower(bodyStr), "fail") ||
					strings.Contains(strings.ToLower(bodyStr), "denied") ||
					strings.Contains(strings.ToLower(bodyStr), "forbidden") ||
					strings.Contains(strings.ToLower(bodyStr), "unauthorized")) {
					if len(bodyStr) > 200 {
						bodyStr = bodyStr[:200] + "..."
					}
					errorInfo = bodyStr
				}
			}
		}

		// 构建日志输出
		logLine := fmt.Sprintf("[GIN] %s | %s | %3d | %13v | %15s | %7s %s",
			start.Format("2006/01/02 - 15:04:05"),
			requestID,
			statusCode,
			latency,
			clientIP,
			method,
			path,
		)

		// 如果有错误信息，添加到日志中
		if errorInfo != "" {
			// 清理错误信息中的换行符，避免破坏日志格式
			errorInfo = strings.ReplaceAll(errorInfo, "\n", " ")
			errorInfo = strings.ReplaceAll(errorInfo, "\r", " ")
			logLine += fmt.Sprintf(" | ERROR: %s", errorInfo)
		}

		logLine += "\n"

		// 输出日志
		if statusCode >= 400 {
			gin.DefaultErrorWriter.Write([]byte(logLine))
		} else {
			gin.DefaultWriter.Write([]byte(logLine))
		}
	})
}
