package ali

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
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// TaskAdaptor 阿里云通义万相任务适配器
type TaskAdaptor struct {
	ChannelType int
	baseURL     string
}

// Init 初始化适配器
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
}

// ValidateRequestAndSetAction 验证请求并设置Action
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestBody 构建请求体
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, _ *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// 打印原始请求参数（截断base64）
	common.SysLog(fmt.Sprintf("[Ali Video] 原始请求参数: model=%s, prompt=%s, duration=%d, size=%s",
		req.Model, req.Prompt, req.Duration, req.Size))
	if len(req.Images) > 0 {
		common.SysLog(fmt.Sprintf("[Ali Video] 图片数量: %d, 第一张图片: %s...",
			len(req.Images), common.TruncateBase64Content(req.Images[0])))
	}
	if req.Metadata != nil {
		metaBytes, _ := json.Marshal(req.Metadata)
		common.SysLog(fmt.Sprintf("[Ali Video] Metadata: %s", common.TruncateBase64Content(string(metaBytes))))
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Ali Video] 转换请求失败: %v", err))
		return nil, err
	}

	data, err := json.Marshal(body)
	if err != nil {
		common.SysError(fmt.Sprintf("[Ali Video] 序列化请求失败: %v", err))
		return nil, err
	}

	// 打印转换后的请求体（截断base64）
	truncatedData := common.TruncateBase64Content(string(data))
	common.SysLog(fmt.Sprintf("[Ali Video] 转换后的请求体: %s", truncatedData))

	return bytes.NewReader(data), nil
}

// BuildRequestURL 构建请求URL
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// 通义万相视频生成API端点
	url := fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL)
	common.SysLog(fmt.Sprintf("[Ali Video] 请求URL: %s", url))
	return url, nil
}

// BuildRequestHeader 构建请求头
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable")
	return nil
}

// DoRequest 执行请求
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理响应
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, _ *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		common.SysError(fmt.Sprintf("[Ali Video] 读取响应失败: %v", err))
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 打印原始响应（截断base64）
	truncatedResp := common.TruncateBase64Content(string(responseBody))
	common.SysLog(fmt.Sprintf("[Ali Video] HTTP状态码: %d, 原始响应: %s", resp.StatusCode, truncatedResp))

	var submitResp SubmitResponse
	err = json.Unmarshal(responseBody, &submitResp)
	if err != nil {
		common.SysError(fmt.Sprintf("[Ali Video] 解析响应失败: %v, 原始响应: %s", err, string(responseBody)))
		taskErr = service.TaskErrorWrapper(errors.Wrap(err, fmt.Sprintf("%s", responseBody)), "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}

	// 阿里云API响应结构：
	// 成功时：{"request_id":"xxx","output":{"task_id":"xxx","task_status":"PENDING"}}
	// 失败时：{"status_code":400,"request_id":"xxx","code":"InvalidParameter","message":"..."}

	// 优先检查是否有task_id（成功的标志）
	if submitResp.Output.TaskID != "" {
		common.SysLog(fmt.Sprintf("[Ali Video] 任务提交成功: task_id=%s, request_id=%s", submitResp.Output.TaskID, submitResp.RequestID))
	} else {
		// 没有task_id，说明是错误响应
		if submitResp.StatusCode != 0 && submitResp.StatusCode != 200 {
			common.SysError(fmt.Sprintf("[Ali Video] 任务提交失败: status_code=%d, message=%s", submitResp.StatusCode, submitResp.Message))
			taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task submit failed: %s", submitResp.Message), "task_submit_failed", http.StatusBadRequest)
			return
		}
		if submitResp.Code != "" {
			common.SysError(fmt.Sprintf("[Ali Video] 任务提交失败: code=%s, message=%s", submitResp.Code, submitResp.Message))
			taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task submit failed: code=%s, message=%s", submitResp.Code, submitResp.Message), "task_submit_failed", http.StatusBadRequest)
			return
		}
		// 如果既没有task_id也没有错误信息，返回通用错误
		common.SysError(fmt.Sprintf("[Ali Video] 未知响应格式: %s", truncatedResp))
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("unknown response format"), "unknown_response", http.StatusInternalServerError)
		return
	}

	// 返回完整对象，同时确保包含统一接口文档要求的字段
	response := gin.H{}
	// 先复制原始响应的所有字段
	responseBytes, _ := json.Marshal(submitResp)
	json.Unmarshal(responseBytes, &response)
	// 确保包含统一接口文档规范的字段
	response["task_id"] = submitResp.Output.TaskID
	response["status"] = "submitted"

	c.JSON(http.StatusOK, response)
	return submitResp.Output.TaskID, responseBody, nil
}

// FetchTask 轮询任务状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		common.SysError("[Ali Video] task_id无效或不存在")
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)
	common.SysLog(fmt.Sprintf("[Ali Video] 轮询任务状态: task_id=%s, url=%s", taskID, url))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("[Ali Video] 创建轮询请求失败: %v", err))
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	return service.GetHttpClient().Do(req)
}

// ParseTaskResult 解析任务结果
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	// 打印轮询响应（截断base64）
	truncatedResp := common.TruncateBase64Content(string(respBody))
	common.SysLog(fmt.Sprintf("[Ali Video] 轮询响应: %s", truncatedResp))

	var taskResp TaskResponse
	err := json.Unmarshal(respBody, &taskResp)
	if err != nil {
		common.SysError(fmt.Sprintf("[Ali Video] 解析轮询响应失败: %v", err))
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	// 检查是否有错误响应（只有错误时才有status_code字段）
	if taskResp.StatusCode != 0 && taskResp.StatusCode != 200 {
		common.SysError(fmt.Sprintf("[Ali Video] 任务查询失败: status_code=%d, code=%s, message=%s",
			taskResp.StatusCode, taskResp.Code, taskResp.Message))
		return nil, fmt.Errorf("task query failed: %s", taskResp.Message)
	}

	// 映射任务状态（基于阿里云官方文档）
	// PENDING：任务排队中
	// RUNNING：任务处理中
	// SUCCEEDED：任务执行成功
	// FAILED：任务执行失败
	// CANCELED：任务已取消
	// UNKNOWN：任务不存在或状态未知
	status := taskResp.Output.TaskStatus
	common.SysLog(fmt.Sprintf("[Ali Video] 任务状态: %s, task_id=%s", status, taskResp.Output.TaskID))

	switch status {
	case "PENDING":
		taskInfo.Status = string(model.TaskStatusInProgress)
		common.SysLog(fmt.Sprintf("[Ali Video] 任务排队中: task_id=%s", taskResp.Output.TaskID))
	case "RUNNING":
		taskInfo.Status = string(model.TaskStatusInProgress)
		common.SysLog(fmt.Sprintf("[Ali Video] 任务处理中: task_id=%s", taskResp.Output.TaskID))
	case "SUCCEEDED":
		taskInfo.Status = string(model.TaskStatusSuccess)
		taskInfo.Url = taskResp.Output.VideoURL

		common.SysLog(fmt.Sprintf("[Ali Video] 任务成功: video_url=%s",
			common.TruncateBase64Content(taskResp.Output.VideoURL)))

		// ⚠️ 关键：设置使用量信息用于计费
		// i2v 和 t2v 模型的返回格式不同：
		// i2v: { "SR": 720, "duration": 5, "video_count": 1 }
		// t2v: { "video_duration": 10, "video_ratio": "832*480", "video_count": 1 }
		// 通过检查 Usage 字段来判断模型类型
		var duration int
		var resolution string

		// 优先检查 t2v 格式（video_duration 和 video_ratio）
		if taskResp.Usage.VideoDuration > 0 && taskResp.Usage.VideoRatio != "" {
			// t2v 模型格式
			duration = taskResp.Usage.VideoDuration
			// 将 "832*480" 转换为 "480p" 格式用于计费
			resolution = parseVideoRatioToResolution(taskResp.Usage.VideoRatio)
			taskInfo.Usage = &relaycommon.VideoUsage{
				VideoCount: taskResp.Usage.VideoCount,
				Duration:   duration,
				Resolution: resolution,
			}
			common.SysLog(fmt.Sprintf("[Ali Video] t2v 使用量信息: duration=%d秒, video_ratio=%s -> resolution=%s, video_count=%d",
				duration, taskResp.Usage.VideoRatio, resolution, taskResp.Usage.VideoCount))
		} else if taskResp.Usage.Duration > 0 && taskResp.Usage.SR > 0 {
			// i2v 模型格式
			duration = taskResp.Usage.Duration
			resolution = fmt.Sprintf("%dp", taskResp.Usage.SR)
			taskInfo.Usage = &relaycommon.VideoUsage{
				VideoCount: taskResp.Usage.VideoCount,
				Duration:   duration,
				Resolution: resolution,
			}
			common.SysLog(fmt.Sprintf("[Ali Video] i2v 使用量信息: duration=%d秒, resolution=%dp, video_count=%d",
				duration, taskResp.Usage.SR, taskResp.Usage.VideoCount))
		} else {
			common.SysError(fmt.Sprintf("[Ali Video] 警告：Usage信息不完整: duration=%d, SR=%d, video_duration=%d, video_ratio=%s",
				taskResp.Usage.Duration, taskResp.Usage.SR, taskResp.Usage.VideoDuration, taskResp.Usage.VideoRatio))
		}
	case "FAILED":
		taskInfo.Status = string(model.TaskStatusFailure)
		if taskResp.Code != "" {
			taskInfo.Reason = fmt.Sprintf("code=%s, message=%s", taskResp.Code, taskResp.Message)
		} else if taskResp.Message != "" {
			taskInfo.Reason = taskResp.Message
		} else {
			taskInfo.Reason = "任务执行失败"
		}
		common.SysError(fmt.Sprintf("[Ali Video] 任务失败: %s", taskInfo.Reason))
	case "CANCELED":
		taskInfo.Status = string(model.TaskStatusFailure)
		taskInfo.Reason = "任务已取消"
		common.SysError(fmt.Sprintf("[Ali Video] 任务已取消: task_id=%s", taskResp.Output.TaskID))
	case "UNKNOWN":
		taskInfo.Status = string(model.TaskStatusFailure)
		taskInfo.Reason = "任务不存在或状态未知"
		common.SysError(fmt.Sprintf("[Ali Video] 任务状态未知: task_id=%s", taskResp.Output.TaskID))
	default:
		common.SysError(fmt.Sprintf("[Ali Video] 未识别的任务状态: %s", status))
		return nil, fmt.Errorf("unknown task status: %s", status)
	}

	return taskInfo, nil
}

// GetModelList 获取支持的模型列表
func (a *TaskAdaptor) GetModelList() []string {
	return []string{"wan2.5-i2v-preview", "wan2.5-t2v-preview"}
}

// GetChannelName 获取渠道名称
func (a *TaskAdaptor) GetChannelName() string {
	return "ali"
}

// ============================
// helpers
// ============================

// convertToRequestPayload 转换请求为阿里云API格式
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*SubmitRequest, error) {
	payload := &SubmitRequest{
		Model: defaultString(req.Model, "wan2.5-i2v-preview"),
		Input: Input{
			Prompt: req.Prompt,
		},
		Parameters: Parameters{
			Duration: defaultInt(req.Duration, 5),
			// Resolution 和 Size 会根据模型类型在后面设置
		},
	}

	// 处理图片输入（i2v 模型必需，t2v 模型不需要）
	if len(req.Images) > 0 {
		imageData := req.Images[0]

		// 阿里云万相视频API对图片格式的要求
		// 1. HTTP/HTTPS URL（推荐，避免内容审核长度限制）
		// 2. OSS URL（oss://...，来自上传策略返回）
		// 3. 纯base64（不带data:前缀，但可能触发长度限制）

		if strings.HasPrefix(imageData, "http://") || strings.HasPrefix(imageData, "https://") {
			// HTTP URL，直接使用（Java已上传到阿里云或公网）
			common.SysLog("[Ali Video] 使用HTTP URL格式的图片（临时/公网）")
			payload.Input.ImgURL = imageData
		} else if strings.HasPrefix(imageData, "oss://") {
			// OSS URL，直接透传
			common.SysLog("[Ali Video] 使用OSS URL格式的图片（oss://...）")
			payload.Input.ImgURL = imageData
		} else if strings.HasPrefix(imageData, "data:image/") {
			// data URL格式，提取纯base64部分
			// 注意：大图片可能触发 InvalidParameter.DataInspection 错误
			common.SysLog("[Ali Video] 检测到data URL格式，提取纯base64（注意：大图片可能失败）")
			parts := strings.SplitN(imageData, ",", 2)
			if len(parts) == 2 {
				payload.Input.ImgURL = parts[1] // 只要base64部分
				imgSize := len(parts[1])
				common.SysLog(fmt.Sprintf("[Ali Video] 提取后的base64长度: %d (约%dKB)", imgSize, imgSize*3/4/1024))
				if imgSize > 500000 { // 约375KB
					common.SysError(fmt.Sprintf("[Ali Video] 警告：图片base64过大(%dKB)，可能触发内容审核长度限制", imgSize*3/4/1024))
				}
			} else {
				return nil, fmt.Errorf("无效的data URL格式")
			}
		} else {
			// 纯base64，直接使用
			imgSize := len(imageData)
			common.SysLog(fmt.Sprintf("[Ali Video] 纯base64格式，长度: %d (约%dKB)", imgSize, imgSize*3/4/1024))
			if imgSize > 500000 { // 约375KB
				common.SysError(fmt.Sprintf("[Ali Video] 警告：图片base64过大(%dKB)，可能触发内容审核长度限制", imgSize*3/4/1024))
			}
			payload.Input.ImgURL = imageData
		}

		// 打印图片格式信息（截断显示）
		imgPreview := payload.Input.ImgURL
		if len(imgPreview) > 100 {
			imgPreview = imgPreview[:100] + "...[已截断]"
		}
		common.SysLog(fmt.Sprintf("[Ali Video] 最终图片URL: %s", imgPreview))
	} else {
		// t2v 模型不需要图片，这是正常的
		if strings.Contains(strings.ToLower(req.Model), "t2v") {
			common.SysLog("[Ali Video] t2v 模型，无需图片输入")
		} else {
			// i2v 模型如果没有图片，记录警告但不报错（让API自己处理）
			common.SysLog("[Ali Video] 警告：i2v 模型未提供图片，API可能会返回错误")
		}
	}

	// 处理wan2.5额外参数（Java直接发送顶层字段，已映射到TaskSubmitReq结构体）
	isT2V := strings.Contains(strings.ToLower(req.Model), "t2v")

	// 1. 分辨率/尺寸参数：i2v 使用 resolution，t2v 使用 size
	if isT2V {
		// t2v 模型：使用 size 参数（如 "1280*720"）
		if req.Size != "" {
			payload.Parameters.Size = req.Size
			common.SysLog(fmt.Sprintf("[Ali Video] t2v 读取尺寸: %s", req.Size))
		} else if req.Resolution != "" {
			// 如果传了 resolution，尝试转换为 size 格式
			size := convertResolutionToSize(req.Resolution)
			if size != "" {
				payload.Parameters.Size = size
				common.SysLog(fmt.Sprintf("[Ali Video] t2v 转换分辨率: %s -> size: %s", req.Resolution, size))
			} else {
				// 默认值：720p -> 1280*720
				payload.Parameters.Size = "1280*720"
				common.SysLog("[Ali Video] t2v 使用默认尺寸: 1280*720")
			}
		} else {
			// 默认值：720p -> 1280*720
			payload.Parameters.Size = "1280*720"
			common.SysLog("[Ali Video] t2v 使用默认尺寸: 1280*720")
		}
	} else {
		// i2v 模型：使用 resolution 参数（如 "720P"）
		if req.Resolution != "" {
			payload.Parameters.Resolution = strings.ToUpper(req.Resolution)
			common.SysLog(fmt.Sprintf("[Ali Video] i2v 读取分辨率: %s -> %s",
				req.Resolution, payload.Parameters.Resolution))
		} else {
			// 默认值：720P
			payload.Parameters.Resolution = "720P"
			common.SysLog("[Ali Video] i2v 使用默认分辨率: 720P")
		}
	}

	// 2. 音频URL（支持OSS HTTPS URL）
	if req.AudioURL != "" {
		payload.Input.AudioURL = req.AudioURL
		common.SysLog(fmt.Sprintf("[Ali Video] 读取音频URL: %s",
			common.TruncateBase64Content(req.AudioURL)))
	}

	// 3. 随机种子
	if req.Seed > 0 {
		payload.Parameters.Seed = req.Seed
		common.SysLog(fmt.Sprintf("[Ali Video] 读取随机种子: %d", req.Seed))
	}

	// 4. 映射参数名（Java用smart_rewrite/generate_audio，API用prompt_extend/audio）
	payload.Parameters.PromptExtend = req.SmartRewrite
	payload.Parameters.Audio = req.GenerateAudio
	common.SysLog(fmt.Sprintf("[Ali Video] 读取参数: smart_rewrite=%v -> prompt_extend=%v, generate_audio=%v -> audio=%v",
		req.SmartRewrite, payload.Parameters.PromptExtend,
		req.GenerateAudio, payload.Parameters.Audio))

	// 打印最终参数（包括audio_url）
	audioURLInfo := "无"
	if payload.Input.AudioURL != "" {
		audioURLInfo = common.TruncateBase64Content(payload.Input.AudioURL)
	}
	if isT2V {
		common.SysLog(fmt.Sprintf("[Ali Video] t2v 最终参数: duration=%d, size=%s, prompt_extend=%v, audio=%v, audio_url=%s",
			payload.Parameters.Duration, payload.Parameters.Size,
			payload.Parameters.PromptExtend, payload.Parameters.Audio, audioURLInfo))
	} else {
		common.SysLog(fmt.Sprintf("[Ali Video] i2v 最终参数: duration=%d, resolution=%s, prompt_extend=%v, audio=%v, audio_url=%s",
			payload.Parameters.Duration, payload.Parameters.Resolution,
			payload.Parameters.PromptExtend, payload.Parameters.Audio, audioURLInfo))
	}

	return payload, nil
}

// parseVideoRatioToResolution 将 t2v 模型的 video_ratio (如 "832*480") 转换为计费用的分辨率格式 (如 "480p")
func parseVideoRatioToResolution(videoRatio string) string {
	// 完整的 video_ratio → resolution 映射表（根据官方文档）
	// 480P档位
	switch videoRatio {
	case "832*480", "480*832", "624*624":
		return "480p"
	// 720P档位
	case "1280*720", "720*1280", "960*960", "1088*832", "832*1088":
		return "720p"
	// 1080P档位
	case "1920*1080", "1080*1920", "1440*1440", "1632*1248", "1248*1632":
		return "1080p"
	}

	// 如果不在映射表中，尝试通过高度/宽度判断（兼容未知格式）
	parts := strings.Split(videoRatio, "*")
	if len(parts) == 2 {
		widthStr := strings.TrimSpace(parts[0])
		heightStr := strings.TrimSpace(parts[1])

		// 优先检查高度，如果高度不匹配则检查宽度（处理正方形等情况）
		if strings.Contains(heightStr, "480") || strings.Contains(widthStr, "480") {
			return "480p"
		} else if strings.Contains(heightStr, "720") || strings.Contains(widthStr, "720") {
			return "720p"
		} else if strings.Contains(heightStr, "1080") || strings.Contains(widthStr, "1080") {
			return "1080p"
		}
	}

	common.SysError(fmt.Sprintf("[Ali Video] 无法识别 video_ratio 分辨率: %s", videoRatio))
	return "720p" // 默认值
}

// convertResolutionToSize 将 resolution (如 "720p") 转换为 t2v 模型的 size 格式 (如 "1280*720")
func convertResolutionToSize(resolution string) string {
	resolution = strings.ToLower(strings.TrimSpace(resolution))

	// 移除 "p" 后缀
	resolution = strings.TrimSuffix(resolution, "p")

	// 根据分辨率档位返回对应的 size
	switch resolution {
	case "480":
		return "832*480" // 16:9 格式
	case "720":
		return "1280*720" // 16:9 格式
	case "1080":
		return "1920*1080" // 16:9 格式
	default:
		return "" // 无法转换
	}
}

func defaultString(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func defaultInt(value, defaultValue int) int {
	if value == 0 {
		return defaultValue
	}
	return value
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
