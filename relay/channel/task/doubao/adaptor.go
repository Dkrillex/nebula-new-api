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
	"strings"
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
		Code    int    `json:"code"`
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

	// 打印详细的请求头信息用于调试
	common.SysLog("[Doubao] BuildRequestHeader - 设置请求头完成")
	common.SysLog("[Doubao] 请求头信息:")
	common.SysLog(fmt.Sprintf("[Doubao]   Authorization: Bearer %s", maskApiKey(a.apiKey)))
	common.SysLog(fmt.Sprintf("[Doubao]   Content-Type: %s", req.Header.Get("Content-Type")))
	common.SysLog(fmt.Sprintf("[Doubao]   User-Agent: %s", req.Header.Get("User-Agent")))

	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	common.SysLog("[Doubao] BuildRequestBody - 开始构建请求体")

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

	// 打印详细的请求体信息用于调试
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestBody - 请求体长度: %d bytes", len(jsonData)))
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestBody - 请求体内容: %s", string(jsonData)))

	// 打印原始VideoRequest信息用于对比
	common.SysLog("[Doubao] 原始VideoRequest信息:")
	common.SysLog(fmt.Sprintf("[Doubao]   Model: %s", req.Model))
	common.SysLog(fmt.Sprintf("[Doubao]   Prompt: %s", req.Prompt))
	common.SysLog(fmt.Sprintf("[Doubao]   Image: %s", req.Image))
	common.SysLog(fmt.Sprintf("[Doubao]   Duration: %.1f", req.Duration))
	common.SysLog(fmt.Sprintf("[Doubao]   Width: %d", req.Width))
	common.SysLog(fmt.Sprintf("[Doubao]   Height: %d", req.Height))
	common.SysLog(fmt.Sprintf("[Doubao]   Metadata: %+v", req.Metadata))

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
	common.SysLog("[Doubao] 完整请求信息:")
	common.SysLog(fmt.Sprintf("[Doubao]   Method: %s", req.Method))
	common.SysLog(fmt.Sprintf("[Doubao]   URL: %s", req.URL.String()))
	common.SysLog(fmt.Sprintf("[Doubao]   Headers: %+v", req.Header))
	if len(bodyBytes) > 0 {
		common.SysLog(fmt.Sprintf("[Doubao]   Body: %s", string(bodyBytes)))
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	common.SysLog("[Doubao] DoRequest - 正在发送HTTP请求...")
	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求发送失败: %v", err))
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求URL: %s", req.URL.String()))
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求方法: %s", req.Method))
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求头: %+v", req.Header))
		if len(bodyBytes) > 0 {
			common.SysError(fmt.Sprintf("[Doubao] DoRequest - 请求体: %s", string(bodyBytes)))
		}
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoRequest - HTTP请求成功，响应状态码: %d", resp.StatusCode))
	common.SysLog(fmt.Sprintf("[Doubao] DoRequest - 响应头: %+v", resp.Header))

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
			common.SysLog(fmt.Sprintf("[Doubao] DoRequest - 200响应体预览: %s", responsePreview))
		}
	}

	return resp, nil
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	common.SysLog("[Doubao] DoResponse - 开始处理响应")

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 读取响应失败: %v", err))
		return "", nil, &dto.TaskError{
			Code:    "response_error",
			Message: "读取响应失败",
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 响应体: %s", string(body)))

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - HTTP请求失败，状态码: %d", resp.StatusCode))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误响应头: %+v", resp.Header))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误响应体长度: %d bytes", len(body)))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 错误响应体内容: %s", string(body)))

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
			errorMessage = fmt.Sprintf("请求参数错误(400): %s", string(body))
		case 401:
			errorMessage = fmt.Sprintf("认证失败(401): API密钥无效或过期，响应: %s", string(body))
		case 403:
			errorMessage = fmt.Sprintf("权限不足(403): 模型未开通或配额不足，响应: %s", string(body))
		case 429:
			errorMessage = fmt.Sprintf("请求频率超限(429): %s", string(body))
		case 500:
			errorMessage = fmt.Sprintf("服务器内部错误(500): %s", string(body))
		default:
			errorMessage = fmt.Sprintf("API请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
		}

		return "", nil, &dto.TaskError{
			Code:    "api_error",
			Message: errorMessage,
		}
	}

	// 解析响应 - 豆包提交任务响应只包含 id 字段
	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 开始解析成功响应，响应体长度: %d bytes", len(body)))
	var response responsePayload
	if err := json.Unmarshal(body, &response); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 解析响应失败: %v", err))
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 原始响应体: %s", string(body)))
		return "", nil, &dto.TaskError{
			Code:    "parse_error",
			Message: fmt.Sprintf("解析响应失败: %v", err),
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 响应解析成功，解析结果: %+v", response))

	// 检查任务ID
	if response.TaskID == "" {
		common.SysError("[Doubao] DoResponse - 响应中缺少任务ID")
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 完整响应内容: %s", string(body)))
		return "", nil, &dto.TaskError{
			Code:    "missing_task_id",
			Message: "响应中缺少任务ID",
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 任务创建成功，任务ID: %s", response.TaskID))
	common.SysLog("[Doubao] DoResponse - 准备返回统一格式响应")

	// 按照统一视频生成接口文档的格式发送响应
	responseData := gin.H{
		"task_id": response.TaskID,
		"status":  "submitted",
	}
	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 发送统一格式响应: %+v", responseData))
	c.JSON(http.StatusOK, responseData)

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 处理完成，返回任务ID: %s", response.TaskID))
	return response.TaskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	common.SysLog("[Doubao] FetchTask - 开始查询任务状态")

	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 请求参数: baseUrl=%s, key=%s", baseUrl, maskApiKey(key)))
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 请求体: %+v", body))

	// 从body中获取任务ID
	taskID, ok := body["task_id"].(string)
	if !ok {
		common.SysError("[Doubao] FetchTask - task_id不存在或类型无效")
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - body内容: %+v", body))
		return nil, fmt.Errorf("task_id not found or invalid")
	}

	if taskID == "" {
		common.SysError("[Doubao] FetchTask - task_id为空")
		return nil, fmt.Errorf("task_id is empty")
	}

	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 提取到任务ID: %s", taskID))

	// 构建查询URL - 使用豆包官方的查询端点
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 查询URL: %s", url))

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 创建请求失败: %v", err))
		return nil, err
	}

	// 设置请求头
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	common.SysLog("[Doubao] FetchTask - 请求头设置完成")
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - Authorization: Bearer %s", maskApiKey(key)))
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - Content-Type: %s", req.Header.Get("Content-Type")))

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	common.SysLog("[Doubao] FetchTask - 正在发送GET请求...")
	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 查询任务HTTP请求失败: %v", err))
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 请求URL: %s", url))
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 请求头: %+v", req.Header))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - HTTP请求成功，响应状态码: %d", resp.StatusCode))
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 响应头: %+v", resp.Header))

	// 读取响应体用于日志记录（不影响后续处理）
	bodyBytes, err := io.ReadAll(resp.Body)
	if err == nil {
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes)) // 重新包装供后续使用
		responsePreview := string(bodyBytes)
		if len(responsePreview) > 1000 {
			responsePreview = responsePreview[:1000] + "...[截断]"
		}
		common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 响应体长度: %d bytes", len(bodyBytes)))
		common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 响应体内容: %s", responsePreview))

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

	common.SysLog("[Doubao] 开始转换VideoRequest到豆包格式")
	common.SysLog(fmt.Sprintf("[Doubao] Model: %s", request.Model))
	// 使用截断函数处理Base64内容
	metadataStr := truncateBase64InMetadata(request.Metadata)
	common.SysLog(fmt.Sprintf("[Doubao] Metadata: %s", metadataStr))

	// 从 Metadata 中提取 content
	if request.Metadata != nil {
		// 提取 content 数组
		if contentInterface, ok := request.Metadata["content"]; ok {
			common.SysLog(fmt.Sprintf("[Doubao] 找到content字段，类型: %T", contentInterface))
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
			} else {
				common.SysLog(fmt.Sprintf("[Doubao] 成功从metadata提取content，共%d个项目", len(payload.Content)))
				// 打印每个内容项的类型（但要截断图片内容）
				for i, item := range payload.Content {
					if item.Type == "image_url" && item.ImageURL != nil {
						truncatedURL := truncateBase64Content(item.ImageURL.URL)
						common.SysLog(fmt.Sprintf("[Doubao] Content[%d]: type=%s, role=%s, url=%s", i, item.Type, item.Role, truncatedURL))
					} else {
						common.SysLog(fmt.Sprintf("[Doubao] Content[%d]: type=%s, role=%s, text=%s", i, item.Type, item.Role, item.Text))
					}
				}
			}
		} else {
			common.SysLog("[Doubao] metadata中未找到content字段")
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
			common.SysLog(fmt.Sprintf("[Doubao] 设置callback_url: %s", callbackURL))
		}
	} else {
		common.SysLog("[Doubao] Metadata为空，使用默认content")
		// 如果没有metadata，使用默认格式
		payload.Content = []ContentItem{
			{
				Type: "text",
				Text: "视频生成请求", // 默认文本
			},
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] 最终转换结果 - Model: %s, Content项目数: %d, CallbackURL: %s",
		payload.Model, len(payload.Content), payload.CallbackURL))

	// 打印最终的请求体（截断图片内容）
	if jsonBytes, err := json.Marshal(payload); err == nil {
		truncatedJSON := truncateBase64Content(string(jsonBytes))
		common.SysLog(fmt.Sprintf("[Doubao] 最终请求体: %s", truncatedJSON))
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
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 响应体长度: %d bytes", len(respBody)))

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
		common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 原始响应体: %s", string(respBody)))
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	common.SysLog("[Doubao] ParseTaskResult - JSON解析成功")
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务ID: %s", doubaoResp.ID))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务状态: %s", doubaoResp.Status))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 模型: %s", doubaoResp.Model))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 创建时间: %d", doubaoResp.CreatedAt))
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 更新时间: %d", doubaoResp.UpdatedAt))

	// 转换为通用任务信息格式
	common.SysLog("[Doubao] ParseTaskResult - 开始转换为统一格式")
	taskInfo := &relaycommon.TaskInfo{
		Code:        0, // 默认成功
		TaskID:      doubaoResp.ID,
		Status:      mapStatus(doubaoResp.Status),
		TotalTokens: doubaoResp.Usage.TotalTokens, // 设置实际消耗的token数量
	}
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 统一状态转换: %s -> %s", doubaoResp.Status, taskInfo.Status))
	if taskInfo.TotalTokens > 0 {
		common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 设置token消耗: %d", taskInfo.TotalTokens))
	}

	// 处理成功状态
	if doubaoResp.Status == "succeeded" {
		common.SysLog("[Doubao] ParseTaskResult - 处理成功状态")
		taskInfo.Url = doubaoResp.Content.VideoURL
		taskInfo.Progress = "100%" // 成功完成

		common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 视频URL: %s", taskInfo.Url))
		if taskInfo.Url == "" {
			common.SysError("[Doubao] ParseTaskResult - 警告: 任务成功但视频URL为空")
		}

		// 记录附加信息
		if doubaoResp.Seed > 0 {
			common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 随机种子: %d", doubaoResp.Seed))
		}
		if doubaoResp.Resolution != "" {
			common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 分辨率: %s", doubaoResp.Resolution))
		}
		if doubaoResp.Duration > 0 {
			common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 时长: %d秒", doubaoResp.Duration))
		}
		if doubaoResp.Usage.TotalTokens > 0 {
			common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 消耗token: %d", doubaoResp.Usage.TotalTokens))
		}
	}

	// 处理进行中状态
	if doubaoResp.Status == "queued" || doubaoResp.Status == "running" {
		common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 处理进行中状态: %s", doubaoResp.Status))
		// 设置适当的进度
		switch doubaoResp.Status {
		case "queued":
			taskInfo.Progress = "20%" // 排队中
			common.SysLog("[Doubao] ParseTaskResult - 任务排队中，进度: 20%")
		case "running":
			taskInfo.Progress = "50%" // 任务运行中
			common.SysLog("[Doubao] ParseTaskResult - 任务运行中，进度: 50%")
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
			common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 错误码: %d", doubaoResp.Error.Code))
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

	common.SysLog("[Doubao] ParseTaskResult - 统一格式转换完成")
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 最终结果: TaskID=%s, Status=%s, Code=%d, Progress=%s",
		taskInfo.TaskID, taskInfo.Status, taskInfo.Code, taskInfo.Progress))
	if taskInfo.Url != "" {
		common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 视频URL长度: %d", len(taskInfo.Url)))
	}
	if taskInfo.Reason != "" {
		common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 失败原因: %s", taskInfo.Reason))
	}

	common.SysLog("[Doubao] ParseTaskResult - 解析完成")
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
	common.SysLog(fmt.Sprintf("[Doubao] convertInterfaceToContent - 输入类型: %T", contentInterface))

	// 先将interface{}转换为JSON字节，再反序列化为[]ContentItem
	jsonBytes, err := json.Marshal(contentInterface)
	if err != nil {
		return fmt.Errorf("序列化content失败: %v", err)
	}

	// 截断图片内容后打印
	truncatedJSON := truncateBase64Content(string(jsonBytes))
	common.SysLog(fmt.Sprintf("[Doubao] Content JSON: %s", truncatedJSON))

	if err := json.Unmarshal(jsonBytes, target); err != nil {
		return fmt.Errorf("反序列化content失败: %v", err)
	}

	common.SysLog(fmt.Sprintf("[Doubao] 成功转换为%d个ContentItem", len(*target)))
	return nil
}

// truncateBase64Content 截断字符串中的base64内容，保留其他信息
func truncateBase64Content(content string) string {
	const base64Prefix = "data:image/"
	const base64Marker = ";base64,"
	const maxBase64Length = 50

	var result strings.Builder
	startIndex := 0

	for {
		// 查找base64前缀
		base64Index := strings.Index(content[startIndex:], base64Prefix)
		if base64Index == -1 {
			break
		}
		base64Index += startIndex

		// 添加base64前的内容
		result.WriteString(content[startIndex:base64Index])

		// 查找base64标记
		markerIndex := strings.Index(content[base64Index:], base64Marker)
		if markerIndex == -1 {
			// 没找到base64标记，保持原样
			result.WriteString(content[base64Index:])
			break
		}
		markerIndex += base64Index

		// 找到下一个引号、空格、逗号或大括号作为结束位置
		endIndex := markerIndex + len(base64Marker)
		actualEnd := len(content)

		for _, delimiter := range []string{"\"", " ", ",", "}"} {
			if pos := strings.Index(content[endIndex:], delimiter); pos != -1 {
				pos += endIndex
				if pos < actualEnd {
					actualEnd = pos
				}
			}
		}

		// 如果base64数据长度超过指定长度，则截断
		if actualEnd-endIndex > maxBase64Length {
			result.WriteString(content[base64Index:endIndex])
			result.WriteString("[base64数据已截断]")
			startIndex = actualEnd
		} else {
			// 短数据保持原样
			result.WriteString(content[base64Index:actualEnd])
			startIndex = actualEnd
		}
	}

	// 添加剩余内容
	result.WriteString(content[startIndex:])
	return result.String()
}

// truncateBase64InMetadata 处理metadata中的Base64内容截断
func truncateBase64InMetadata(metadata map[string]any) string {
	if metadata == nil {
		return "nil"
	}

	if jsonBytes, err := json.Marshal(metadata); err == nil {
		return truncateBase64Content(string(jsonBytes))
	}
	return fmt.Sprintf("%+v", metadata)
}
