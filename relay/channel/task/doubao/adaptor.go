package doubao

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/service"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// 豆包视频生成请求结构体
type requestPayload struct {
	Model           string        `json:"model"`
	Content         []ContentItem `json:"content"`
	CallbackURL     string        `json:"callback_url,omitempty"`      // 可选的回调URL
	GenerateAudio   *bool         `json:"generate_audio,omitempty"`    // 是否生成音频（1.5 Pro 新增）
	ReturnLastFrame *bool         `json:"return_last_frame,omitempty"` // 是否返回最后一帧（1.5 Pro 新增）
	TaskType        string        `json:"task_type,omitempty"`         // 任务类型：t2v / i2v 等
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

type responsePayload struct {
	TaskID string `json:"id"`
}

type taskQueryResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	FramesPerSecond int `json:"framespersecond"`
	Usage           struct {
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	Seed       int    `json:"seed"`
	Resolution string `json:"resolution"`
	Duration   int    `json:"duration"`
	Ratio      string `json:"ratio"`
	// 错误信息
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param"`
	} `json:"error,omitempty"`
	Reason string `json:"reason,omitempty"` // 失败原因
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
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

	// 将原始请求体完整写入 Metadata，保留 content / 其他上游参数
	if request.Metadata == nil {
		if bodyBytes, err := common.GetRequestBody(c); err == nil && len(bodyBytes) > 0 {
			var meta map[string]any
			if err := json.Unmarshal(bodyBytes, &meta); err == nil {
				request.Metadata = meta
			} else {
				common.SysError(fmt.Sprintf("[Doubao] 解析原始请求体到metadata失败: %v", err))
			}
		}
	}

	// 将中间件中判定好的 action 写入 RelayInfo，便于后续 task_type 推断和计费使用
	if actionVal, exists := c.Get("action"); exists {
		if actionStr, ok := actionVal.(string); ok && actionStr != "" {
			info.Action = actionStr
		}
	}

	common.SysLog("[Doubao] ValidateRequestAndSetAction - 验证通过")
	return nil
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")

	return nil
}

// BuildRequestBody converts request into Doubao specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {

	// 从上下文获取请求
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(*dto.VideoRequest)

	// 获取映射后的模型名称（用于发送到上游API）
	upstreamModel := info.UpstreamModelName
	if upstreamModel == "" {
		upstreamModel = req.Model
	}

	// 推断任务类型（t2v / i2v）
	taskType := inferDoubaoTaskType(req, info)

	// 转换为豆包API格式，传入原始模型名、映射后的模型名以及任务类型
	payload := convertVideoRequestToDoubaoPayload(req, upstreamModel, taskType)

	// 序列化为JSON
	jsonData, err := json.Marshal(payload)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 序列化请求失败: %v", err))
		return nil, err
	}
	return bytes.NewReader(jsonData), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {

	// 构建请求URL，并根据模型/动作决定 task_type 查询参数
	baseURL, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}
	url := baseURL

	// 从上下文还原 VideoRequest，用于推断 task_type
	if v, exists := c.Get("task_request"); exists {
		if req, ok := v.(*dto.VideoRequest); ok {
			taskType := inferDoubaoTaskType(req, info)
			if taskType != "" {
				sep := "?"
				if strings.Contains(url, "?") {
					sep = "&"
				}
				url = fmt.Sprintf("%s%stask_type=%s", url, sep, taskType)
			}
		}
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

	// 发送请求 - 对于视频生成任务，使用较长的超时时间
	// 视频生成请求可能包含较大的图片数据，需要更长的上传时间
	client := service.GetHttpClient()
	// 对于视频上传场景，使用5分钟超时（上传2-3MB的base64数据可能需要较长时间）
	if client.Timeout < 300*time.Second {
		client = &http.Client{
			Timeout:       300 * time.Second, // 5分钟超时，足够上传大型base64图片
			CheckRedirect: client.CheckRedirect,
			Transport:     client.Transport, // 复制Transport配置
		}
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

// DoResponse handles upstream response, returns taskID etc.
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
	_ = resp.Body.Close()

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

// FetchTask fetch task status
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
	req.Header.Set("Authorization", "Bearer "+key)

	// 发送请求 - 视频任务查询也使用较长的超时时间
	client := service.GetHttpClient()
	// 如果配置的超时时间小于60秒，则使用60秒（专门针对视频任务查询场景）
	if client.Timeout < 60*time.Second {
		client = &http.Client{
			Timeout:       60 * time.Second, // 查询任务用60秒超时
			CheckRedirect: client.CheckRedirect,
			Transport:     client.Transport, // 复制Transport配置
		}
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
// request: 原始请求（包含原始模型名，用于判断参数）
// upstreamModel: 映射后的模型名（用于发送到上游API）
// taskType: 任务类型（t2v/i2v），用于传递给上游
func convertVideoRequestToDoubaoPayload(request *dto.VideoRequest, upstreamModel string, taskType string) *requestPayload {
	payload := &requestPayload{
		Model:    upstreamModel, // 使用映射后的模型名发送到上游
		TaskType: taskType,
	}

	common.SysLog(fmt.Sprintf("[Doubao] Original Model: %s, Upstream Model: %s", request.Model, upstreamModel))

	// 根据模型名称判断是否生成音频（仅针对 1.5 Pro 系列）
	// doubao-seedance-1-5-pro-251215-noAudio → generate_audio: false（写死）
	// doubao-seedance-1-5-pro-251215 → generate_audio: true（默认）
	if strings.Contains(request.Model, "doubao-seedance-1-5-pro-251215") {
		if strings.Contains(request.Model, "-noAudio") {
			generateAudio := false
			payload.GenerateAudio = &generateAudio
			common.SysLog("[Doubao] 检测到 -noAudio 后缀，设置 generate_audio: false")
		} else {
			generateAudio := true
			payload.GenerateAudio = &generateAudio
			common.SysLog("[Doubao] doubao-seedance-1-5-pro-251215 模型，设置 generate_audio: true")
		}

		// 默认启用 return_last_frame
		returnLastFrame := true
		payload.ReturnLastFrame = &returnLastFrame
		common.SysLog("[Doubao] doubao-seedance-1-5-pro-251215 模型，设置 return_last_frame: true")
	}

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

		// 提取 return_last_frame (1.5 Pro 系列支持)
		if strings.Contains(request.Model, "doubao-seedance-1-5-pro-251215") {
			if returnLastFrameVal, ok := request.Metadata["return_last_frame"]; ok {
				if returnLastFrame, ok2 := returnLastFrameVal.(bool); ok2 {
					payload.ReturnLastFrame = &returnLastFrame
				}
			}
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

	//common.SysLog("[Doubao] ParseTaskResult - JSON解析成功")
	//common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务ID: %s", doubaoResp.ID))
	//common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务状态: %s", doubaoResp.Status))
	//common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 模型: %s", doubaoResp.Model))
	//common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 创建时间: %d", doubaoResp.CreatedAt))
	//common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 更新时间: %d", doubaoResp.UpdatedAt))

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
	// 优先走通用 JSON 编解码路径
	jsonBytes, err := json.Marshal(contentInterface)
	if err == nil {
		if err := json.Unmarshal(jsonBytes, target); err == nil && len(*target) > 0 {
			return nil
		}
	}

	// 如果 JSON 路径失败，做一次手工解析，尽量保留文本和图片信息
	slice, ok := contentInterface.([]interface{})
	if !ok {
		return fmt.Errorf("unsupported content type: %T", contentInterface)
	}

	items := make([]ContentItem, 0, len(slice))
	for _, elem := range slice {
		m, ok := elem.(map[string]interface{})
		if !ok {
			continue
		}
		item := ContentItem{}
		if t, ok := m["type"].(string); ok {
			item.Type = t
		}
		if text, ok := m["text"].(string); ok {
			item.Text = text
		}
		if imageVal, ok := m["image_url"].(map[string]interface{}); ok {
			if url, ok := imageVal["url"].(string); ok && url != "" {
				item.ImageURL = &ImageURL{URL: url}
			}
		}
		// 只保留至少有 type/text/image_url 之一的项
		if item.Type != "" || item.Text != "" || item.ImageURL != nil {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		return fmt.Errorf("no valid content items parsed")
	}

	*target = items
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

// inferDoubaoTaskType 推断豆包视频任务类型（t2v / i2v）
func inferDoubaoTaskType(request *dto.VideoRequest, info *relaycommon.RelayInfo) string {
	// 1. 先看模型名称（包含 i2v / t2v 直观标识）
	modelName := strings.ToLower(request.Model)
	if info != nil && info.OriginModelName != "" {
		modelName = strings.ToLower(info.OriginModelName)
	}
	if strings.Contains(modelName, "i2v") {
		return "i2v"
	}
	if strings.Contains(modelName, "t2v") {
		return "t2v"
	}

	// 2. 其次看动作类型（由中间件 / 通用校验写入）
	if info != nil {
		switch info.Action {
		case constant.TaskActionGenerate, constant.TaskActionFirstTailGenerate, constant.TaskActionReferenceGenerate:
			return "i2v"
		case constant.TaskActionTextGenerate:
			return "t2v"
		}
	}

	// 3. 再看 Metadata 中是否包含图片输入
	if request.Metadata != nil {
		if contentInterface, ok := request.Metadata["content"]; ok {
			if hasImageInContent(contentInterface) {
				return "i2v"
			}
		}
		if imageInputs, ok := request.Metadata["image_inputs"]; ok && imageInputs != nil {
			return "i2v"
		}
	}

	// 4. 默认按文生视频处理
	return "t2v"
}

// hasImageInContent 判断 content 结构中是否包含 image_url
func hasImageInContent(contentInterface interface{}) bool {
	slice, ok := contentInterface.([]interface{})
	if !ok {
		return false
	}
	for _, elem := range slice {
		m, ok := elem.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := m["type"].(string); ok && t == "image_url" {
			if imageVal, ok := m["image_url"].(map[string]interface{}); ok {
				if url, ok := imageVal["url"].(string); ok && url != "" {
					return true
				}
			}
		}
	}
	return false
}
