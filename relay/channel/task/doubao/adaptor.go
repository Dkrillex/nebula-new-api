package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"time"

	"github.com/gin-gonic/gin"
)

// 豆包视频生成请求结构体
type requestPayload struct {
	Model       string        `json:"model"`
	Content     []ContentItem `json:"content"`
	CallbackURL string        `json:"callback_url,omitempty"` // 可选的回调URL
}

type ContentItem struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
	Role     string    `json:"role,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

// 豆包视频生成任务提交响应结构体
type responsePayload struct {
	TaskID string `json:"id"`
}

// 豆包任务查询响应结构体（匹配官方API响应格式）
type taskQueryResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Usage struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	Seed           int    `json:"seed"`
	Resolution     string `json:"resolution"`
	Duration       int    `json:"duration"`
	Ratio          string `json:"ratio"`
	FramePerSecond int    `json:"framespersecond"`
	// 错误信息
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
	} `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"` // 失败原因
}

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	common.SysLog("[Doubao] ValidateRequestAndSetAction - 开始验证请求")

	// 从上下文中获取已解析的请求，如果不存在则解析
	var request *dto.VideoRequest
	if req, exists := c.Get("task_request"); exists {
		request = req.(*dto.VideoRequest)
	} else {
		// 解析请求体
		request = &dto.VideoRequest{}
		if err := c.ShouldBindJSON(request); err != nil {
			common.SysError(fmt.Sprintf("[Doubao] 解析请求失败: %v", err))
			return &dto.TaskError{
				Code:    "invalid_request",
				Message: "请求格式错误",
			}
		}
		c.Set("task_request", request)
	}

	// 验证模型
	if request.Model == "" {
		return &dto.TaskError{
			Code:    "invalid_request",
			Message: "模型参数不能为空",
		}
	}

	common.SysLog("[Doubao] ValidateRequestAndSetAction - 验证通过")
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// 使用豆包官方的API端点
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL)
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestURL: %s", url))
	return url, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {

	// 从上下文获取请求
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(*dto.VideoRequest)

	// 转换为豆包API格式
	payload := convertVideoRequestToDoubaoPayload(req)

	// 序列化为JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 序列化请求失败: %v", err))
		return nil, err
	}
	return bytes.NewReader(jsonData), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	common.SysLog("[Doubao] DoRequest - 开始发送请求")

	// 构建请求URL
	url, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}

	// 读取请求体内容用于调试（需要重新创建Reader）
	var bodyBytes []byte
	if requestBody != nil {
		bodyBytes, _ = io.ReadAll(requestBody)
		requestBody = bytes.NewReader(bodyBytes) // 重新创建Reader
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, requestBody)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 创建请求失败: %v", err))
		return nil, err
	}

	// 设置请求头
	err = a.BuildRequestHeader(c, req, info)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 设置请求头失败: %v", err))
		return nil, err
	}

	// 打印完整的请求信息用于调试
	common.SysLog(fmt.Sprintf("[Doubao]   URL: %s", req.URL.String()))
	common.SysLog(fmt.Sprintf("[Doubao]   Headers: %+v", req.Header))
	if len(bodyBytes) > 0 {
		truncatedBody := common.TruncateBase64Content(string(bodyBytes))
		common.SysLog(fmt.Sprintf("[Doubao]   Body: %s", truncatedBody))
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求发送失败: %v", err))
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求URL: %s", req.URL.String()))
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求方法: %s", req.Method))
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求头: %+v", req.Header))
		if len(bodyBytes) > 0 {
			truncatedBody := common.TruncateBase64Content(string(bodyBytes))
			common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求体: %s", truncatedBody))
		}
		return nil, err
	}

	// 对于非200状态码，读取响应体进行详细日志记录
	if resp.StatusCode != http.StatusOK {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // 重新包装供后续使用
			common.SysError(fmt.Sprintf("[Doubao] DoRequest - 非200状态码(%d)响应体: %s", resp.StatusCode, string(bodyBytes)))
		} else {
			common.SysError(fmt.Sprintf("[Doubao] DoRequest - 读取错误响应体失败: %v", err))
		}

		// 根据状态码进行分类日志记录
		switch resp.StatusCode {
		case 400:
			common.SysError("[Doubao] DoRequest - 400 Bad Request: 请求参数错误或格式不正确")
		case 401:
			common.SysError("[Doubao] DoRequest - 401 Unauthorized: API密钥无效或未授权")
		case 403:
			common.SysError("[Doubao] DoRequest - 403 Forbidden: 权限不足，可能是模型未开通")
		case 429:
			common.SysError("[Doubao] DoRequest - 429 Too Many Requests: 请求频率超限")
		case 500:
			common.SysError("[Doubao] DoRequest - 500 Internal Server Error: 服务器内部错误")
		default:
			common.SysError(fmt.Sprintf("[Doubao] DoRequest - %d: 未知错误状态码", resp.StatusCode))
		}
	} else {
		// 200状态码也读取一小部分响应体用于日志（但不影响后续处理）
		bodyBytes, err := io.ReadAll(resp.Body)
		if err == nil {
			resp.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // 重新包装供后续使用
			responsePreview := string(bodyBytes)
			if len(responsePreview) > 500 {
				responsePreview = responsePreview[:500] + "...[截断]"
			}
		}
	}

	return resp, nil
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 读取响应失败: %v", err))
		return "", nil, &dto.TaskError{
			Code:    "response_error",
			Message: "读取响应失败",
		}
	}

	// 使用截断函数处理base64内容，保留其他信息
	truncatedContent := common.TruncateBase64Content(string(body))
	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 响应体: %s", truncatedContent))

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - HTTP请求失败，状态码: %d", resp.StatusCode))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误响应头: %+v", resp.Header))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误响应体长度: %d bytes", len(body)))
		truncatedErrorBody := common.TruncateBase64Content(string(body))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误响应体内容: %s", truncatedErrorBody))

		// 尝试解析错误响应获取更详细的错误信息
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			common.SysError(fmt.Sprintf("[Doubao] DoResponse - 解析后的错误信息: %+v", errorResp))

			// 提取常见的错误字段
			if errorCode, ok := errorResp["error"].(map[string]interface{}); ok {
				if code, exists := errorCode["code"]; exists {
					common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误代码: %v", code))
				}
				if message, exists := errorCode["message"]; exists {
					common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误消息: %v", message))
				}
				if errorType, exists := errorCode["type"]; exists {
					common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误类型: %v", errorType))
				}
			}
		} else {
			common.SysError(fmt.Sprintf("[Doubao] DoResponse - 解析错误响应失败: %v", err))
		}

		// 根据状态码提供具体的错误信息
		var errorMessage string
		switch resp.StatusCode {
		case 400:
			truncatedBody := common.TruncateBase64Content(string(body))
			errorMessage = fmt.Sprintf("请求参数错误(400): %s", truncatedBody)
		case 401:
			truncatedBody := common.TruncateBase64Content(string(body))
			errorMessage = fmt.Sprintf("认证失败(401): API密钥无效或过期，响应: %s", truncatedBody)
		case 403:
			truncatedBody := common.TruncateBase64Content(string(body))
			errorMessage = fmt.Sprintf("权限不足(403): 模型未开通或配额不足，响应: %s", truncatedBody)
		case 429:
			truncatedBody := common.TruncateBase64Content(string(body))
			errorMessage = fmt.Sprintf("请求频率超限(429): %s", truncatedBody)
		case 500:
			truncatedBody := common.TruncateBase64Content(string(body))
			errorMessage = fmt.Sprintf("服务器内部错误(500): %s", truncatedBody)
		default:
			truncatedBody := common.TruncateBase64Content(string(body))
			errorMessage = fmt.Sprintf("API请求失败，状态码: %d, 响应: %s", resp.StatusCode, truncatedBody)
		}

		return "", nil, &dto.TaskError{
			Code:    "api_error",
			Message: errorMessage,
		}
	}

	// 解析响应 - 豆包提交任务响应只包含 id 字段
	var response responsePayload
	if err := json.Unmarshal(body, &response); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 解析响应失败: %v", err))
		return "", nil, &dto.TaskError{
			Code:    "parse_error",
			Message: fmt.Sprintf("解析响应失败: %v", err),
		}
	}

	// 检查任务ID
	if response.TaskID == "" {
		truncatedFullBody := common.TruncateBase64Content(string(body))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 完整响应内容: %s", truncatedFullBody))
		return "", nil, &dto.TaskError{
			Code:    "missing_task_id",
			Message: "响应中缺少任务ID",
		}
	}

	// 按照统一视频生成接口文档的格式发送响应
	responseData := gin.H{
		"task_id": response.TaskID,
		"status":  "submitted",
	}
	c.JSON(http.StatusOK, responseData)

	return response.TaskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {

	// 从body中获取任务ID
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("task_id not found or invalid")
	}

	if taskID == "" {
		return nil, fmt.Errorf("task_id is empty")
	}

	// 构建查询URL - 使用豆包官方的查询端点
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 创建请求失败: %v", err))
		return nil, err
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 查询任务HTTP请求失败: %v", err))
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 请求URL: %s", url))
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 请求头: %+v", req.Header))
		return nil, err
	}

	// 读取响应体用于日志记录（不影响后续处理）
	bodyBytes, err := io.ReadAll(resp.Body)
	if err == nil {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // 重新包装供后续使用
		responsePreview := string(bodyBytes)
		if len(responsePreview) > 1000 {
			responsePreview = responsePreview[:1000] + "...[截断]"
		}
		// 根据状态码提供详细的日志
		if resp.StatusCode != http.StatusOK {
			switch resp.StatusCode {
			case 400:
				common.SysError("[Doubao] FetchTask - 400 Bad Request: 任务ID格式错误或参数无效")
			case 401:
				common.SysError("[Doubao] FetchTask - 401 Unauthorized: API密钥无效或未授权")
			case 404:
				common.SysError(fmt.Sprintf("[Doubao] FetchTask - 404 Not Found: 任务ID(%s)不存在", taskID))
			case 429:
				common.SysError("[Doubao] FetchTask - 429 Too Many Requests: 查询频率超限")
			case 500:
				common.SysError("[Doubao] FetchTask - 500 Internal Server Error: 服务器内部错误")
			default:
				common.SysError(fmt.Sprintf("[Doubao] FetchTask - %d: 未知错误状态码", resp.StatusCode))
			}
		}
	} else {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 读取响应体失败: %v", err))
	}

	common.SysLog("[Doubao] FetchTask - 查询请求完成")
	return resp, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"video-01",
		"video-01-turbo",
		"video-01-plus",
		"video-01-pro",
		"video-01-ultra",
		"video-01-max",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "doubao"
}

// convertVideoRequestToDoubaoPayload 将 VideoRequest 转换为豆包请求格式
// 现在直接从 Metadata 中提取 content 和 callback_url，按照豆包官方格式传递
func convertVideoRequestToDoubaoPayload(request *dto.VideoRequest) *requestPayload {
	payload := &requestPayload{
		Model: request.Model,
	}

	common.SysLog(fmt.Sprintf("[Doubao] Model: %s", request.Model))

	// 从 Metadata 中提取 content
	if request.Metadata != nil {
		// 提取 content 数组
		if contentInterface, ok := request.Metadata["content"]; ok {
			// 将 interface{} 转换为 []ContentItem
			if err := convertInterfaceToContent(contentInterface, &payload.Content); err != nil {
				common.SysError(fmt.Sprintf("[Doubao] 转换content失败: %v", err))
				common.SysError(fmt.Sprintf("[Doubao] 原始数据类型: %T", contentInterface))
				common.SysError(fmt.Sprintf("[Doubao] 原始数据内容: %+v", contentInterface))
				// 如果转换失败，使用默认格式
				payload.Content = []ContentItem{
					{
						Type: "text",
						Text: "视频生成请求", // 默认文本
					},
				}
			}
		} else {
			// 如果没有content，使用默认格式
			payload.Content = []ContentItem{
				{
					Type: "text",
					Text: "视频生成请求", // 默认文本
				},
			}
		}

		// 提取 callback_url
		if callbackURL, ok := request.Metadata["callback_url"].(string); ok && callbackURL != "" {
			payload.CallbackURL = callbackURL
		}
	} else {
		// 如果没有metadata，使用默认格式
		payload.Content = []ContentItem{
			{
				Type: "text",
				Text: "视频生成请求", // 默认文本
			},
		}
	}
	return payload
}

// maskApiKey 遮蔽API密钥的敏感部分用于日志打印
func maskApiKey(apiKey string) string {
	if len(apiKey) <= 8 {
		return "***"
	}
	return apiKey[:4] + "***" + apiKey[len(apiKey)-4:]
}

// ParseTaskResult 解析豆包任务查询结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	common.SysLog("[Doubao] ParseTaskResult - 开始解析任务结果")

	// 打印响应体内容用于调试（截断长内容）
	responsePreview := string(respBody)
	if len(responsePreview) > 1500 {
		responsePreview = responsePreview[:1500] + "...[截断]"
	}
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 响应体内容: %s", responsePreview))

	// 解析豆包响应
	var doubaoResp taskQueryResponse
	if err := json.Unmarshal(respBody, &doubaoResp); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - JSON解析响应失败: %v", err))
		truncatedRespBody := common.TruncateBase64Content(string(respBody))
		common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 原始响应体: %s", truncatedRespBody))
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	common.SysLog("[Doubao] ParseTaskResult - JSON解析成功")
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务ID: %s", doubaoResp.ID))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务状态: %s", doubaoResp.Status))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 模型: %s", doubaoResp.Model))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 创建时间: %d", doubaoResp.CreatedAt))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 更新时间: %d", doubaoResp.UpdatedAt))

	// 转换为通用任务信息格式
	taskInfo := &relaycommon.TaskInfo{
		Code:        0, // 默认成功
		TaskID:      doubaoResp.ID,
		Status:      mapStatus(doubaoResp.Status),
		TotalTokens: doubaoResp.Usage.TotalTokens, // 设置实际消耗的token数量
	}

	// 处理成功状态
	if doubaoResp.Status == "succeeded" {
		taskInfo.Url = doubaoResp.Content.VideoURL
		taskInfo.Progress = "100%" // 成功完成

		if taskInfo.Url == "" {
			common.SysError("[Doubao] ParseTaskResult - 警告: 任务成功但视频URL为空")
		}
	}

	// 处理进行中状态
	if doubaoResp.Status == "queued" || doubaoResp.Status == "running" {
		// 设置适当的进度
		switch doubaoResp.Status {
		case "queued":
			taskInfo.Progress = "20%" // 排队中
		case "running":
			taskInfo.Progress = "50%" // 任务运行中
		}
	}

	// 处理失败状态
	if doubaoResp.Status == "failed" || doubaoResp.Status == "cancelled" {
		common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 处理失败状态: %s", doubaoResp.Status))
		taskInfo.Code = 1          // 失败状态码
		taskInfo.Progress = "100%" // 失败也算完成

		// 详细分析失败原因
		if doubaoResp.Status == "cancelled" {
			taskInfo.Reason = "任务已取消"
			common.SysError("[Doubao] ParseTaskResult - 任务被取消")
		} else if doubaoResp.Error != nil {
			taskInfo.Reason = doubaoResp.Error.Message
			common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 错误信息: %+v", doubaoResp.Error))
			common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 错误码: %s", doubaoResp.Error.Code))
			common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 错误消息: %s", doubaoResp.Error.Message))
			common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 错误类型: %s", doubaoResp.Error.Type))
		} else if doubaoResp.Reason != "" {
			taskInfo.Reason = doubaoResp.Reason
			common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 失败原因: %s", doubaoResp.Reason))
		} else {
			taskInfo.Reason = "任务执行失败"
			common.SysError("[Doubao] ParseTaskResult - 未知失败原因")
		}
	}

	return taskInfo, nil
}

// mapStatus 将豆包状态映射为系统状态常量
func mapStatus(doubaoStatus string) string {
	switch doubaoStatus {
	case "queued":
		return "QUEUED" // 排队中
	case "running":
		return "IN_PROGRESS" // 任务运行中
	case "succeeded":
		return "SUCCESS" // 任务成功
	case "failed":
		return "FAILURE" // 任务失败
	case "cancelled":
		return "FAILURE" // 取消任务，视为失败
	default:
		return "UNKNOWN"
	}
}

// defaultString 返回默认字符串值
func defaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// defaultInt 返回默认整数值
func defaultInt(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

// convertInterfaceToContent 将interface{}转换为[]ContentItem
func convertInterfaceToContent(contentInterface interface{}, target *[]ContentItem) error {

	// 先将interface{}转换为JSON字节，再反序列化为[]ContentItem
	jsonBytes, err := json.Marshal(contentInterface)
	if err != nil {
		return fmt.Errorf("序列化content失败: %v", err)
	}

	// 截断图片内容后打印
	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return fmt.Errorf("反序列化content失败: %v", err)
	}

	return nil
}

// truncateBase64InMetadata 处理metadata中的Base64内容截断
func truncateBase64InMetadata(metadata map[string]any) string {
	if metadata == nil {
		return "nil"
	}

	if jsonBytes, err := json.Marshal(metadata); err == nil {
		return common.TruncateBase64Content(string(jsonBytes))
	}
	return fmt.Sprintf("%+v", metadata)
}
