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
	TaskID string `json:"id"`
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
}

// 豆包任务查询响应结构体（原始豆包API响应格式）
type taskQueryResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Status  string `json:"status"`
	Content struct {
		VideoURL string `json:"video_url"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
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

	// 验证提示词
	if request.Prompt == "" {
		return &dto.TaskError{
			Code:    "invalid_request",
			Message: "提示词不能为空",
		}
	}

	common.SysLog("[Doubao] ValidateRequestAndSetAction - 验证通过")
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	url := fmt.Sprintf("%s/api/v3/video/generation", a.baseURL)
	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestURL: %s", url))
	return url, nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	common.SysLog("[Doubao] BuildRequestHeader - 设置请求头完成")
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

	common.SysLog(fmt.Sprintf("[Doubao] BuildRequestBody - 请求体: %s", string(jsonData)))
	return bytes.NewReader(jsonData), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	common.SysLog("[Doubao] DoRequest - 开始发送请求")

	// 构建请求URL
	url, err := a.BuildRequestURL(info)
	if err != nil {
		return nil, err
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, requestBody)
	if err != nil {
		return nil, err
	}

	// 设置请求头
	err = a.BuildRequestHeader(c, req, info)
	if err != nil {
		return nil, err
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 请求失败: %v", err))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoRequest - 响应状态码: %d", resp.StatusCode))
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
		common.SysError(fmt.Sprintf("[Doubao] HTTP错误，状态码: %d, 响应: %s", resp.StatusCode, string(body)))
		return "", nil, &dto.TaskError{
			Code:    "api_error",
			Message: fmt.Sprintf("API请求失败，状态码: %d", resp.StatusCode),
		}
	}

	// 解析响应
	var response responsePayload
	if err := json.Unmarshal(body, &response); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 解析响应失败: %v", err))
		return "", nil, &dto.TaskError{
			Code:    "parse_error",
			Message: "解析响应失败",
		}
	}

	// 检查业务错误
	if response.Code != 0 {
		common.SysError(fmt.Sprintf("[Doubao] 业务错误，代码: %d, 消息: %s", response.Code, response.Msg))
		return "", nil, &dto.TaskError{
			Code:    "business_error",
			Message: response.Msg,
		}
	}

	// 检查任务ID
	if response.TaskID == "" {
		common.SysError("[Doubao] 响应中缺少任务ID")
		return "", nil, &dto.TaskError{
			Code:    "missing_task_id",
			Message: "响应中缺少任务ID",
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] DoResponse - 任务创建成功，ID: %s", response.TaskID))
	return response.TaskID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	common.SysLog("[Doubao] FetchTask - 开始查询任务状态")

	// 从body中获取任务ID
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("task_id not found or invalid")
	}

	// 构建查询URL
	url := fmt.Sprintf("%s/api/v3/video/generation/%s", baseUrl, taskID)
	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 查询URL: %s", url))

	// 创建请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
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
		common.SysError(fmt.Sprintf("[Doubao] 查询任务失败: %v", err))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Doubao] FetchTask - 响应状态码: %d", resp.StatusCode))
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

// convertToDoubaoRequestPayload 将通用视频请求转换为豆包API格式
func convertToDoubaoRequestPayload(request *relaycommon.TaskSubmitReq) *requestPayload {
	payload := &requestPayload{
		Model: request.Model,
	}

	// 处理提示词
	if request.Prompt != "" {
		payload.Content = append(payload.Content, ContentItem{
			Type: "text",
			Text: request.Prompt,
			Role: "user",
		})
	}

	// 处理图片输入
	if request.Image != "" {
		payload.Content = append(payload.Content, ContentItem{
			Type: "image_url",
			ImageURL: &ImageURL{
				URL: request.Image,
			},
			Role: "user",
		})
	}

	return payload
}

// convertVideoRequestToDoubaoPayload 将 VideoRequest 转换为豆包请求格式
func convertVideoRequestToDoubaoPayload(request *dto.VideoRequest) *requestPayload {
	payload := &requestPayload{
		Model: request.Model,
	}

	// 处理提示词
	if request.Prompt != "" {
		payload.Content = append(payload.Content, ContentItem{
			Type: "text",
			Text: request.Prompt,
			Role: "user",
		})
	}

	// 处理图片输入
	if request.Image != "" {
		payload.Content = append(payload.Content, ContentItem{
			Type: "image_url",
			ImageURL: &ImageURL{
				URL: request.Image,
			},
			Role: "user",
		})
	}

	return payload
}

// ParseTaskResult 解析豆包任务查询结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	common.SysLog("[Doubao] ParseTaskResult - 开始解析任务结果")

	// 解析豆包响应
	var doubaoResp taskQueryResponse
	if err := json.Unmarshal(respBody, &doubaoResp); err != nil {
		common.SysError(fmt.Sprintf("[Doubao] 解析响应失败: %v", err))
		return nil, fmt.Errorf("解析响应失败: %v", err)
	}

	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 任务状态: %s", doubaoResp.Status))

	// 转换为通用任务信息格式
	taskInfo := &relaycommon.TaskInfo{
		Code:   0, // 默认成功
		TaskID: doubaoResp.ID,
		Status: mapStatus(doubaoResp.Status),
	}

	// 处理成功状态
	if doubaoResp.Status == "succeeded" {
		taskInfo.Url = doubaoResp.Content.VideoURL
	}

	// 处理失败状态
	if doubaoResp.Status == "failed" {
		taskInfo.Code = 1 // 失败状态码
		if doubaoResp.Error != nil {
			taskInfo.Reason = doubaoResp.Error.Message
		} else if doubaoResp.Reason != "" {
			taskInfo.Reason = doubaoResp.Reason
		} else {
			taskInfo.Reason = "任务执行失败"
		}
	}

	common.SysLog(fmt.Sprintf("[Doubao] ParseTaskResult - 解析完成，状态: %s", taskInfo.Status))
	return taskInfo, nil
}

// mapStatus 将豆包状态映射为通用状态
func mapStatus(doubaoStatus string) string {
	switch doubaoStatus {
	case "pending":
		return "pending"
	case "processing":
		return "processing"
	case "succeeded":
		return "completed"
	case "failed":
		return "failed"
	case "cancelled":
		return "cancelled"
	default:
		return "unknown"
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
