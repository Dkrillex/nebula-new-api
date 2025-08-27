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
	Model   string        `json:"model"`
	Content []ContentItem `json:"content"`
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

// 豆包视频生成响应结构体
type responsePayload struct {
	TaskID string `json:"task_id"`
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
}

// 豆包任务查询响应结构体
type taskQueryResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		TaskID string `json:"task_id"`
		Status string `json:"status"`
		Result struct {
			VideoURL string `json:"video_url"`
		} `json:"result"`
		Reason string `json:"reason"`
	} `json:"data"`
}

type TaskAdaptor struct {
	ChannelType int
	apiKey      string
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.TaskRelayInfo) {
	a.ChannelType = info.ChannelType
	a.apiKey = info.ApiKey
	a.baseURL = info.BaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.TaskRelayInfo) (taskErr *dto.TaskError) {
	common.SysLog("[Doubao] ValidateRequestAndSetAction - 开始验证请求")

	// 从上下文中获取已解析的请求，如果不存在则解析
	var request dto.VideoRequest
	if req, exists := c.Get("parsed_video_request"); exists {
		request = req.(dto.VideoRequest)
		common.SysLog(fmt.Sprintf("[Doubao] ValidateRequestAndSetAction - 从上下文获取请求: %+v", request))
	} else {
		if err := c.ShouldBindJSON(&request); err != nil {
			common.SysError(fmt.Sprintf("[Doubao] ValidateRequestAndSetAction - 解析请求失败: %v", err))
			return &dto.TaskError{
				StatusCode: http.StatusBadRequest,
				Code:       "invalid_request",
				Message:    fmt.Sprintf("Invalid request format: %v", err),
			}
		}
		common.SysLog(fmt.Sprintf("[Doubao] ValidateRequestAndSetAction - 解析请求成功: %+v", request))
		// 将解析的请求存储到上下文中，供后续使用
		c.Set("parsed_video_request", request)
	}

	if request.Prompt == "" && request.Image == "" {
		common.SysError("[Doubao] ValidateRequestAndSetAction - 缺少必需字段: prompt 或 image")
		return &dto.TaskError{
			StatusCode: http.StatusBadRequest,
			Code:       "missing_required_field",
			Message:    "prompt or image is required",
		}
	}

	info.Action = "video_generation"
	common.SysLog("[Doubao] ValidateRequestAndSetAction - 验证成功")
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.TaskRelayInfo) (string, error) {
	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL)
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestURL - 构建请求URL: %s", url))
	return url, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.TaskRelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestHeader - 设置请求头: Content-Type=application/json, Authorization=Bearer %s...", a.apiKey[:min(len(a.apiKey), 10)]))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.TaskRelayInfo) (io.Reader, error) {
	common.SysLog("[Doubao] BuildRequestBody - 开始构建请求体")

	// 从上下文中获取已解析的请求
	req, exists := c.Get("parsed_video_request")
	if !exists {
		common.SysError("[Doubao] BuildRequestBody - 上下文中未找到解析的视频请求")
		return nil, fmt.Errorf("parsed video request not found in context")
	}
	request := req.(dto.VideoRequest)

	payload := convertToRequestPayload(&request)
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestBody - 转换后的请求载荷: %+v", payload))

	jsonData, err := json.Marshal(payload)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] BuildRequestBody - 序列化请求失败: %v", err))
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestBody - 请求体JSON: %s", string(jsonData)))
	return bytes.NewReader(jsonData), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.TaskRelayInfo, requestBody io.Reader) (*http.Response, error) {
	common.SysLog("[Doubao] DoRequest - 开始发送请求")

	url, err := a.BuildRequestURL(info)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 构建URL失败: %v", err))
		return nil, err
	}

	req, err := http.NewRequest("POST", url, requestBody)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 创建HTTP请求失败: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	err = a.BuildRequestHeader(c, req, info)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - 设置请求头失败: %v", err))
		return nil, err
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoRequest - 发送POST请求到: %s", url))
	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoRequest - HTTP请求失败: %v", err))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoRequest - 收到响应，状态码: %d", resp.StatusCode))
	return resp, nil
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.TaskRelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 开始处理响应，状态码: %d", resp.StatusCode))

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 读取响应体失败: %v", err))
		return "", nil, &dto.TaskError{
			StatusCode: http.StatusInternalServerError,
			Code:       "read_response_failed",
			Message:    fmt.Sprintf("Failed to read response body: %v", err),
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 原始响应体: %s", string(body)))

	var response responsePayload
	if err := json.Unmarshal(body, &response); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - 解析响应JSON失败: %v", err))
		return "", nil, &dto.TaskError{
			StatusCode: http.StatusInternalServerError,
			Code:       "unmarshal_response_failed",
			Message:    fmt.Sprintf("Failed to unmarshal response: %v", err),
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 解析后的响应: Code=%d, Msg=%s, TaskID=%s", response.Code, response.Msg, response.TaskID))

	if response.Code != 0 {
		common.SysError(fmt.Sprintf("[Doubao] DoResponse - API返回错误: %s (code: %d)", response.Msg, response.Code))
		return "", nil, &dto.TaskError{
			StatusCode: http.StatusBadRequest,
			Code:       "api_error",
			Message:    fmt.Sprintf("API error: %s (code: %d)", response.Msg, response.Code),
		}
	}

	// 返回JSON响应给客户端
	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 成功获取TaskID: %s，返回给客户端", response.TaskID))
	c.JSON(http.StatusOK, gin.H{"task_id": response.TaskID})
	return response.TaskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 开始查询任务，参数: %+v", body))

	taskID, ok := body["task_id"].(string)
	if !ok {
		common.SysError("[Doubao] FetchTask - 无效的task_id参数")
		return nil, fmt.Errorf("invalid task_id")
	}

	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 查询任务ID: %s", taskID))

	url := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 查询URL: %s", url))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - 创建HTTP请求失败: %v", err))
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 设置请求头，Authorization=Bearer %s...", key[:min(len(key), 10)]))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	common.SysLog("[Doubao] FetchTask - 发送GET请求")
	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] FetchTask - HTTP请求失败: %v", err))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 收到响应，状态码: %d", resp.StatusCode))
	return resp, nil
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"doubao-seedance-lite",
		"doubao-seedance-1-0-lite-t2v",
		"doubao-seedance-1-0-lite-i2v",
		"doubao-seedance-pronew",
		"doubao-seaweed",
		"wan2-1-14b-t2v",
		"wan2-1-14b-i2v",
		"wan2-1-14b-flf2v",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "doubao"
}

// 辅助函数：将请求转换为豆包API格式
func convertToRequestPayload(request *dto.VideoRequest) *requestPayload {
	common.SysLog(fmt.Sprintf("[Doubao] convertToRequestPayload - 开始转换请求，输入: %+v", request))

	payload := &requestPayload{
		Model:   request.Model,
		Content: []ContentItem{},
	}

	// 添加文本内容
	if request.Prompt != "" {
		textContent := ContentItem{
			Type: "text",
			Text: request.Prompt,
		}

		// 添加参数到文本中
		if request.Metadata != nil {
			common.SysLog(fmt.Sprintf("[Doubao] convertToRequestPayload - 处理元数据: %+v", request.Metadata))
			if duration, ok := request.Metadata["duration"]; ok {
				textContent.Text += fmt.Sprintf(" --duration %v", duration)
			}
			if aspectRatio, ok := request.Metadata["aspect_ratio"]; ok {
				textContent.Text += fmt.Sprintf(" --aspect_ratio %v", aspectRatio)
			}
			if resolution, ok := request.Metadata["resolution"]; ok {
				textContent.Text += fmt.Sprintf(" --resolution %v", resolution)
			}
		}

		common.SysLog(fmt.Sprintf("[Doubao] convertToRequestPayload - 添加文本内容: %s", textContent.Text))
		payload.Content = append(payload.Content, textContent)
	}

	// 添加图片内容
	if request.Image != "" {
		imageContent := ContentItem{
			Type: "image_url",
			ImageURL: &ImageURL{
				URL: request.Image,
			},
			Role: "first_frame",
		}
		common.SysLog(fmt.Sprintf("[Doubao] convertToRequestPayload - 添加图片内容: %s", request.Image))
		payload.Content = append(payload.Content, imageContent)
	}

	common.SysLog(fmt.Sprintf("[Doubao] convertToRequestPayload - 转换完成，输出: %+v", payload))
	return payload
}

// 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	common.SysLog("[Doubao] ParseTaskResult - 开始解析任务结果")
	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 原始响应体: %s", string(respBody)))

	var response taskQueryResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - 解析响应JSON失败: %v", err))
		return nil, fmt.Errorf("failed to unmarshal task response: %w", err)
	}

	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 解析后的响应: Code=%d, Msg=%s, TaskID=%s, Status=%s", response.Code, response.Msg, response.Data.TaskID, response.Data.Status))

	if response.Code != 0 {
		common.SysError(fmt.Sprintf("[Doubao] ParseTaskResult - API返回错误: %s (code: %d)", response.Msg, response.Code))
		return nil, fmt.Errorf("API error: %s (code: %d)", response.Msg, response.Code)
	}

	taskInfo := &relaycommon.TaskInfo{
		Code:   response.Code,
		TaskID: response.Data.TaskID,
		Status: mapStatus(response.Data.Status),
		Reason: response.Data.Reason,
	}

	// 如果任务完成，添加视频URL
	if response.Data.Status == "success" && response.Data.Result.VideoURL != "" {
		taskInfo.Url = response.Data.Result.VideoURL
		common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务完成，视频URL: %s", response.Data.Result.VideoURL))
	}

	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 解析完成，任务信息: %+v", taskInfo))
	return taskInfo, nil
}

// 映射豆包状态到内部状态
func mapStatus(doubaoStatus string) string {
	switch strings.ToLower(doubaoStatus) {
	case "pending", "running", "processing":
		return "pending"
	case "success", "completed":
		return "completed"
	case "failed", "error":
		return "failed"
	default:
		return "pending"
	}
}

// 默认字符串值
func defaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// 默认整数值
func defaultInt(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}
