package sora

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	"one-api/relay/channel"
	relaycommon "one-api/relay/common"
	"one-api/service"
	"one-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

type ContentItem struct {
	Type     string    `json:"type"`                // "text" or "image_url"
	Text     string    `json:"text,omitempty"`      // for text type
	ImageURL *ImageURL `json:"image_url,omitempty"` // for image_url type
}

type ImageURL struct {
	URL string `json:"url"`
}

type Generation struct {
	Object    string `json:"object"`
	ID        string `json:"id"`
	JobID     string `json:"job_id"`
	CreatedAt int64  `json:"created_at"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	NSeconds  int    `json:"n_seconds"`
	Prompt    string `json:"prompt"`
	URL       string `json:"url,omitempty"` // 视频URL（如果有）
}

type responseTask struct {
	ID                 string       `json:"id"`
	TaskID             string       `json:"task_id,omitempty"` //兼容旧接口
	Object             string       `json:"object"`
	Model              string       `json:"model"`
	Status             string       `json:"status"`
	Progress           int          `json:"progress"`
	CreatedAt          int64        `json:"created_at"`
	CompletedAt        int64        `json:"completed_at,omitempty"`
	FinishedAt         int64        `json:"finished_at,omitempty"` // Azure 使用 finished_at
	ExpiresAt          int64        `json:"expires_at,omitempty"`
	Seconds            string       `json:"seconds,omitempty"`
	Size               string       `json:"size,omitempty"`
	RemixedFromVideoID string       `json:"remixed_from_video_id,omitempty"`
	Generations        []Generation `json:"generations,omitempty"` // Azure 返回的生成结果
	Prompt             string       `json:"prompt,omitempty"`
	NVariants          int          `json:"n_variants,omitempty"`
	NSeconds           int          `json:"n_seconds,omitempty"`
	Height             int          `json:"height,omitempty"`
	Width              int          `json:"width,omitempty"`
	FailureReason      string       `json:"failure_reason,omitempty"`
	Error              *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
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
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateMultipartDirect(c, info)
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var url string
	// Azure OpenAI Sora API
	if a.ChannelType == constant.ChannelTypeAzure {
		// Azure 格式: /openai/v1/video/generations/jobs?api-version=preview
		apiVersion := info.ApiVersion
		if apiVersion == "" {
			apiVersion = "preview" // Azure Sora 默认使用 preview 版本
		}
		url = fmt.Sprintf("%s/openai/v1/video/generations/jobs?api-version=%s", a.baseURL, apiVersion)
	} else {
		// 原生 OpenAI Sora API
		url = fmt.Sprintf("%s/v1/videos", a.baseURL)
	}

	common.SysLog(fmt.Sprintf("[Sora] 提交任务请求URL: %s", url))
	return url, nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	// Azure OpenAI 使用 Api-key header
	if a.ChannelType == constant.ChannelTypeAzure {
		req.Header.Set("Api-key", a.apiKey)
	} else {
		// 原生 OpenAI 使用 Bearer token
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	cachedBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	return bytes.NewReader(cachedBody), nil
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := json.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		if dResp.TaskID == "" {
			taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
			return
		}
		dResp.ID = dResp.TaskID
		dResp.TaskID = ""
	}

	c.JSON(http.StatusOK, dResp)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var uri string
	// Azure OpenAI Sora API
	if a.ChannelType == constant.ChannelTypeAzure {
		// Azure 格式: /openai/v1/video/generations/jobs/{job_id}?api-version=preview
		apiVersion := "preview"
		uri = fmt.Sprintf("%s/openai/v1/video/generations/jobs/%s?api-version=%s", baseUrl, taskID, apiVersion)
	} else {
		// 原生 OpenAI Sora API
		uri = fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)
	}

	common.SysLog(fmt.Sprintf("[Sora] 查询任务状态URL: %s", uri))

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	// 设置认证 header
	if a.ChannelType == constant.ChannelTypeAzure {
		req.Header.Set("Api-key", key)
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	return service.GetHttpClient().Do(req)
}

// FetchVideoContent 获取生成的视频内容（Azure 需要额外的 API 调用）
func (a *TaskAdaptor) FetchVideoContent(baseUrl, key, jobID, genID string) (*http.Response, error) {
	var uri string

	if a.ChannelType == constant.ChannelTypeAzure {
		// Azure 格式: /openai/v1/video/generations/jobs/{job_id}/generations/{gen_id}/content?api-version=preview
		apiVersion := "preview"
		uri = fmt.Sprintf("%s/openai/v1/video/generations/jobs/%s/generations/%s/content?api-version=%s",
			baseUrl, jobID, genID, apiVersion)
	} else {
		// 原生 OpenAI: /v1/videos/{video_id}/content
		uri = fmt.Sprintf("%s/v1/videos/%s/content", baseUrl, jobID)
	}

	common.SysLog(fmt.Sprintf("[Sora] 获取视频内容URL: %s", uri))

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	// 设置认证 header
	if a.ChannelType == constant.ChannelTypeAzure {
		req.Header.Set("Api-key", key)
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := json.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	common.SysLog(fmt.Sprintf("[Sora] 解析任务结果 - TaskID: %s, Status: %s, Generations数量: %d",
		resTask.ID, resTask.Status, len(resTask.Generations)))

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress", "preprocessing":
		taskResult.Status = model.TaskStatusInProgress
	case "completed", "succeeded": // Azure 使用 "succeeded"
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"

		// 尝试获取视频URL
		if len(resTask.Generations) > 0 && resTask.Generations[0].ID != "" {
			// Azure: generations 数组存在但没有直接的 URL
			// 需要通过额外的 content API 获取视频
			genID := resTask.Generations[0].ID
			jobID := resTask.ID

			common.SysLog(fmt.Sprintf("[Sora] Azure 任务成功 - JobID: %s, GenID: %s", jobID, genID))

			// 返回系统代理 URL，由系统处理实际的视频下载
			taskResult.Url = fmt.Sprintf("%s/v1/video/generations/%s/content/%s",
				system_setting.ServerAddress, jobID, genID)

			common.SysLog(fmt.Sprintf("[Sora] 生成代理URL: %s", taskResult.Url))
		} else {
			// 原生 OpenAI: 使用标准的 content endpoint
			taskResult.Url = fmt.Sprintf("%s/v1/videos/%s/content",
				system_setting.ServerAddress, resTask.ID)
			common.SysLog(fmt.Sprintf("[Sora] OpenAI 标准URL: %s", taskResult.Url))
		}

	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else if resTask.FailureReason != "" {
			taskResult.Reason = resTask.FailureReason
		} else {
			taskResult.Reason = "task failed"
		}
		common.SysLog(fmt.Sprintf("[Sora] 任务失败 - 原因: %s", taskResult.Reason))
	default:
		// 未知状态，保持进行中
		taskResult.Status = model.TaskStatusInProgress
		common.SysLog(fmt.Sprintf("[Sora] 未知状态: %s", resTask.Status))
	}

	// 设置进度
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) (*relaycommon.OpenAIVideo, error) {
	openAIVideo := &relaycommon.OpenAIVideo{}
	err := json.Unmarshal(task.Data, openAIVideo)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal to OpenAIVideo failed")
	}
	return openAIVideo, nil
}
