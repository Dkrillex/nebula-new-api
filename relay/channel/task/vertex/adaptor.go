package vertex

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/model"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"one-api/constant"
	"one-api/dto"
	"one-api/relay/channel"
	vertexcore "one-api/relay/channel/vertex"
	relaycommon "one-api/relay/common"
	"one-api/service"
)

// ============================
// Request / Response structures
// ============================

type requestPayload struct {
	Instances  []map[string]any `json:"instances"`
	Parameters map[string]any   `json:"parameters,omitempty"`
}

type submitResponse struct {
	Name string `json:"name"`
}

type operationVideo struct {
	MimeType           string `json:"mimeType"`
	BytesBase64Encoded string `json:"bytesBase64Encoded"`
	Encoding           string `json:"encoding"`
}

type operationResponse struct {
	Name     string `json:"name"`
	Done     bool   `json:"done"`
	Response struct {
		Type                  string           `json:"@type"`
		RaiMediaFilteredCount int              `json:"raiMediaFilteredCount"`
		Videos                []operationVideo `json:"videos"`
		BytesBase64Encoded    string           `json:"bytesBase64Encoded"`
		Encoding              string           `json:"encoding"`
		Video                 string           `json:"video"`
	} `json:"response"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
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

// ValidateRequestAndSetAction parses body, validates fields and sets default action.
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// 提取请求中的 Veo 参数并重新组装到 metadata 中
	cachedBody, err := common.GetRequestBody(c)
	if err == nil {
		var requestMap map[string]interface{}
		if json.Unmarshal(cachedBody, &requestMap) == nil {
			// 检查是否有 Veo 参数（从根层级提取）
			metadata := make(map[string]interface{})

			// 提取时长参数（支持驼峰 durationSeconds 和下划线 duration_seconds）
			var durationSec int
			if ds, ok := requestMap["durationSeconds"].(float64); ok && ds > 0 {
				durationSec = int(ds)
			} else if ds, ok := requestMap["durationSeconds"].(int); ok && ds > 0 {
				durationSec = ds
			} else if ds, ok := requestMap["duration_seconds"].(float64); ok && ds > 0 {
				durationSec = int(ds)
			} else if ds, ok := requestMap["duration_seconds"].(int); ok && ds > 0 {
				durationSec = ds
			}
			if durationSec > 0 {
				metadata["durationSeconds"] = durationSec
				c.Set("video_seconds", durationSec)
				common.SysLog(fmt.Sprintf("[Veo] 提取时长参数: %d秒", durationSec))
			}

			// 提取宽高比（支持驼峰 aspectRatio 和下划线 aspect_ratio）
			var aspectRatio string
			if ar, ok := requestMap["aspectRatio"].(string); ok && ar != "" {
				aspectRatio = ar
			} else if ar, ok := requestMap["aspect_ratio"].(string); ok && ar != "" {
				aspectRatio = ar
			}
			if aspectRatio != "" {
				metadata["aspectRatio"] = aspectRatio
				common.SysLog(fmt.Sprintf("[Veo] 提取宽高比: %s", aspectRatio))
			}

			// 提取分辨率
			if resolution, ok := requestMap["resolution"].(string); ok && resolution != "" {
				metadata["resolution"] = resolution
				common.SysLog(fmt.Sprintf("[Veo] 提取分辨率: %s", resolution))
			}

			// 提取帧率
			var fps interface{}
			if f, ok := requestMap["fps"].(float64); ok && f > 0 {
				fps = f
			} else if f, ok := requestMap["fps"].(int); ok && f > 0 {
				fps = f
			}
			if fps != nil {
				metadata["fps"] = fps
				common.SysLog(fmt.Sprintf("[Veo] 提取帧率: %v", fps))
			}

			// 提取 sampleCount
			var sampleCount int
			if sc, ok := requestMap["sampleCount"].(float64); ok && sc > 0 {
				sampleCount = int(sc)
			} else if sc, ok := requestMap["sampleCount"].(int); ok && sc > 0 {
				sampleCount = sc
			} else if sc, ok := requestMap["sample_count"].(float64); ok && sc > 0 {
				sampleCount = int(sc)
			} else if sc, ok := requestMap["sample_count"].(int); ok && sc > 0 {
				sampleCount = sc
			}
			if sampleCount > 0 {
				metadata["sampleCount"] = sampleCount
				common.SysLog(fmt.Sprintf("[Veo] 提取 sampleCount: %d", sampleCount))
			}

			// 提取首帧图片
			if image, ok := requestMap["image"].(string); ok && image != "" {
				metadata["image"] = image
				common.SysLog("[Veo] 提取首帧图片")
			}

			// 提取尾帧图片（支持驼峰 lastFrame 和下划线 last_frame）
			var lastFrame string
			if lf, ok := requestMap["lastFrame"].(string); ok && lf != "" {
				lastFrame = lf
			} else if lf, ok := requestMap["last_frame"].(string); ok && lf != "" {
				lastFrame = lf
			}
			if lastFrame != "" {
				metadata["lastFrame"] = lastFrame
				common.SysLog("[Veo] 提取尾帧图片")
			}

			// 如果提取到了参数，将 metadata 放入 requestMap
			if len(metadata) > 0 {
				// 获取现有的 metadata（如果有）并合并
				if existingMeta, ok := requestMap["metadata"].(map[string]interface{}); ok {
					for k, v := range metadata {
						existingMeta[k] = v
					}
					requestMap["metadata"] = existingMeta
				} else {
					requestMap["metadata"] = metadata
				}

				// 更新缓存的请求体（这是关键，必须先更新才能被 ValidateBasicTaskRequest 读取）
				if updatedBody, err := json.Marshal(requestMap); err == nil {
					// 清除旧的缓存，设置新的请求体
					c.Set("request_body", updatedBody)
					common.SysLog(fmt.Sprintf("[Veo] 已将 %d 个参数组装到 metadata", len(metadata)))

					// 强制重新缓存请求体（确保 GetRequestBody 读取到最新的）
					c.Request.Body = io.NopCloser(bytes.NewReader(updatedBody))
				}
			}
		}
	}

	// Use the standard validation method for TaskSubmitReq（会读取 request_body）
	taskErr = relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
	if taskErr != nil {
		return taskErr
	}

	common.SysLog("[Veo] ValidateBasicTaskRequest 完成")

	// 手动更新 task_request.Metadata（因为 ValidateBasicTaskRequest 可能使用了缓存）
	common.SysLog("[Veo] 开始手动更新 task_request.Metadata")

	v, ok := c.Get("task_request")
	if !ok {
		common.SysLog("[Veo] ❌ 无法从上下文获取 task_request")
		return nil
	}
	common.SysLog("[Veo] ✅ 成功获取 task_request")

	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		common.SysLog("[Veo] ❌ task_request 类型转换失败")
		return nil
	}
	common.SysLog(fmt.Sprintf("[Veo] ✅ task_request 类型转换成功，当前 Metadata 长度: %d", len(req.Metadata)))

	// 重新从 request_body 读取最新的 metadata
	cachedBody2, err2 := common.GetRequestBody(c)
	if err2 != nil {
		common.SysLog(fmt.Sprintf("[Veo] ❌ 无法读取 request_body: %v", err2))
		return nil
	}
	common.SysLog(fmt.Sprintf("[Veo] ✅ 成功读取 request_body，长度: %d", len(cachedBody2)))

	var requestMap map[string]interface{}
	if err := json.Unmarshal(cachedBody2, &requestMap); err != nil {
		common.SysLog(fmt.Sprintf("[Veo] ❌ 解析 request_body JSON 失败: %v", err))
		return nil
	}
	common.SysLog(fmt.Sprintf("[Veo] ✅ 成功解析 request_body，包含 %d 个字段", len(requestMap)))

	// 打印 requestMap 的键
	keys := make([]string, 0, len(requestMap))
	for k := range requestMap {
		keys = append(keys, k)
	}
	common.SysLog(fmt.Sprintf("[Veo] request_body 包含的键: %v", keys))

	// 方案：直接从根层级提取 Veo 参数并构造 metadata
	metaData := make(map[string]interface{})

	// 从根层级提取参数（驼峰格式）
	if val, ok := requestMap["durationSeconds"]; ok {
		metaData["durationSeconds"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 durationSeconds: %v", val))
	}
	if val, ok := requestMap["aspectRatio"]; ok {
		metaData["aspectRatio"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 aspectRatio: %v", val))
	}
	if val, ok := requestMap["resolution"]; ok {
		metaData["resolution"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 resolution: %v", val))
	}
	if val, ok := requestMap["fps"]; ok {
		metaData["fps"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 fps: %v", val))
	}
	if val, ok := requestMap["image"]; ok {
		metaData["image"] = val
		common.SysLog("[Veo] 从根层级提取 image")
	}
	if val, ok := requestMap["sampleCount"]; ok {
		metaData["sampleCount"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 sampleCount: %v", val))
	}
	if val, ok := requestMap["sample_count"]; ok {
		metaData["sampleCount"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 sample_count: %v", val))
	}
	if val, ok := requestMap["lastFrame"]; ok {
		metaData["lastFrame"] = val
		common.SysLog("[Veo] 从根层级提取 lastFrame")
	}
	if val, ok := requestMap["generateAudio"]; ok {
		metaData["generateAudio"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 generateAudio: %v", val))
	} else if val, ok := requestMap["generate_audio"]; ok {
		metaData["generateAudio"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 generate_audio: %v", val))
	}
	if val, ok := requestMap["personGeneration"]; ok {
		metaData["personGeneration"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 personGeneration: %v", val))
	} else if val, ok := requestMap["person_generation"]; ok {
		metaData["personGeneration"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 person_generation: %v", val))
	}
	if val, ok := requestMap["addWatermark"]; ok {
		metaData["addWatermark"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 addWatermark: %v", val))
	} else if val, ok := requestMap["add_watermark"]; ok {
		metaData["addWatermark"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 add_watermark: %v", val))
	}
	if val, ok := requestMap["seed"]; ok {
		metaData["seed"] = val
		common.SysLog(fmt.Sprintf("[Veo] 从根层级提取 seed: %v", val))
	}

	if len(metaData) == 0 {
		common.SysLog("[Veo] ⚠️ 未提取到任何 Veo 参数")
		return nil
	}

	common.SysLog(fmt.Sprintf("[Veo] ✅ 从根层级提取了 %d 个 Veo 参数", len(metaData)))

	// 更新 req.Metadata
	req.Metadata = metaData
	c.Set("task_request", req)
	common.SysLog("[Veo] ✅ 手动更新 task_request.Metadata 成功")

	// 调试：打印 metadata 内容
	if metadataJSON, err := json.Marshal(metaData); err == nil {
		truncated := common.TruncateBase64Content(string(metadataJSON))
		common.SysLog(fmt.Sprintf("[Veo] 最终 Metadata 内容: %s", truncated))
	}

	return nil
}

// BuildRequestURL constructs the upstream URL.
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	adc := &vertexcore.Credentials{}
	if err := json.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return "", fmt.Errorf("failed to decode credentials: %w", err)
	}
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = "veo-3.0-generate-001"
	}

	region := vertexcore.GetModelRegion(info.ApiVersion, modelName)
	if strings.TrimSpace(region) == "" {
		region = "global"
	}
	if region == "global" {
		return fmt.Sprintf(
			"https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:predictLongRunning",
			adc.ProjectID,
			modelName,
		), nil
	}
	return fmt.Sprintf(
		"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:predictLongRunning",
		region,
		adc.ProjectID,
		region,
		modelName,
	), nil
}

// BuildRequestHeader sets required headers.
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	adc := &vertexcore.Credentials{}
	if err := json.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return fmt.Errorf("failed to decode credentials: %w", err)
	}

	token, err := vertexcore.AcquireAccessToken(*adc, "")
	if err != nil {
		return fmt.Errorf("failed to acquire access token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-goog-user-project", adc.ProjectID)
	return nil
}

// BuildRequestBody converts request into Vertex specific format.
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// 初始化 instances
	instance := map[string]any{"prompt": req.Prompt}

	body := requestPayload{
		Instances:  []map[string]any{instance},
		Parameters: map[string]any{},
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}

	modelName := deriveModelName(info, req)
	supportsAudioParam := supportsGenerateAudioParameter(modelName)

	if metadataJSON, err := json.Marshal(metadata); err == nil {
		truncated := common.TruncateBase64Content(string(metadataJSON))
		common.SysLog(fmt.Sprintf("[Veo] req.Metadata 内容: %s", truncated))
	} else {
		common.SysLog("[Veo] req.Metadata 解析失败，使用空对象")
	}

	durationSeconds := sanitizeDurationSecondsFromMetadata(metadata)
	metadata["durationSeconds"] = durationSeconds
	body.Parameters["durationSeconds"] = durationSeconds
	common.SysLog(fmt.Sprintf("[Veo] 设置时长参数: %d秒", durationSeconds))

	aspectRatio := sanitizeAspectRatioFromMetadata(metadata)
	metadata["aspectRatio"] = aspectRatio
	body.Parameters["aspectRatio"] = aspectRatio
	common.SysLog(fmt.Sprintf("[Veo] 设置宽高比: %s", aspectRatio))

	resolutionOption := sanitizeResolutionOptionFromMetadata(metadata)
	metadata["resolution"] = resolutionOption
	body.Parameters["resolution"] = resolutionOption
	common.SysLog(fmt.Sprintf("[Veo] 设置分辨率: %s", resolutionOption))

	fps := sanitizeFpsFromMetadata(metadata)
	metadata["fps"] = fps
	body.Parameters["fps"] = fps

	sampleCount := sanitizeSampleCountFromMetadata(metadata)
	metadata["sampleCount"] = sampleCount
	body.Parameters["sampleCount"] = sampleCount
	common.SysLog(fmt.Sprintf("[Veo] 设置 sampleCount: %d", sampleCount))

	if imageVal, ok := metadata["image"]; ok {
		if imageStr, ok := imageVal.(string); ok && imageStr != "" {
			base64Data := convertToBase64(imageStr)
			instance["image"] = map[string]any{
				"bytesBase64Encoded": base64Data,
				"mimeType":           detectImageMimeType(imageStr),
			}
			common.SysLog(fmt.Sprintf("[Veo] 添加首帧图片 (base64 长度: %d)", len(base64Data)))
		}
	}

	if lastFrameVal, ok := metadata["lastFrame"]; ok {
		if lastFrameStr, ok := lastFrameVal.(string); ok && lastFrameStr != "" {
			base64Data := convertToBase64(lastFrameStr)
			instance["lastFrame"] = map[string]any{
				"bytesBase64Encoded": base64Data,
				"mimeType":           detectImageMimeType(lastFrameStr),
			}
			common.SysLog(fmt.Sprintf("[Veo] 添加尾帧图片 (base64 长度: %d)", len(base64Data)))
		}
	}

	generateAudio := sanitizeGenerateAudio(metadata, req.GenerateAudio)
	if !supportsAudioParam {
		generateAudio = true
	}
	metadata["generateAudio"] = generateAudio
	metadata["generate_audio"] = generateAudio
	if supportsAudioParam {
		body.Parameters["generateAudio"] = generateAudio
		common.SysLog(fmt.Sprintf("[Veo] 设置 generateAudio: %v", generateAudio))
	} else {
		common.SysLog(fmt.Sprintf("[Veo] 模型 %s 默认包含音频，忽略 generateAudio 参数", modelName))
	}

	personGeneration := sanitizePersonGenerationFromMetadata(metadata)
	metadata["personGeneration"] = personGeneration
	metadata["person_generation"] = personGeneration
	body.Parameters["personGeneration"] = personGeneration
	common.SysLog(fmt.Sprintf("[Veo] 设置 personGeneration: %s", personGeneration))

	addWatermark := sanitizeBoolMetadata(metadata, []string{"addWatermark", "add_watermark"}, false)
	metadata["addWatermark"] = addWatermark
	metadata["add_watermark"] = addWatermark
	body.Parameters["addWatermark"] = addWatermark
	common.SysLog(fmt.Sprintf("[Veo] 设置 addWatermark: %v", addWatermark))

	if seed := extractIntFromMetadata(metadata, "seed"); seed > 0 {
		metadata["seed"] = seed
		body.Parameters["seed"] = seed
		common.SysLog(fmt.Sprintf("[Veo] 设置 seed: %d", seed))
	}

	req.Metadata = metadata
	c.Set("task_request", req)

	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	// 打印最终请求体（截断 base64）
	truncatedBody := common.TruncateBase64Content(string(data))
	common.SysLog(fmt.Sprintf("[Veo] 构建的请求体: %s", truncatedBody))

	return bytes.NewReader(data), nil
}

func toBool(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		normalized := strings.TrimSpace(strings.ToLower(v))
		if normalized == "true" || normalized == "1" || normalized == "yes" {
			return true, true
		}
		if normalized == "false" || normalized == "0" || normalized == "no" {
			return false, true
		}
	case float64:
		return v != 0, true
	case float32:
		return v != 0, true
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint:
		return v != 0, true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f != 0, true
		}
	}
	return false, false
}

func toString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toInt(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case string:
		if v == "" {
			return 0, false
		}
		if num, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return num, true
		}
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
	}
	return 0, false
}

// convertToBase64 将 URL 或 data URI 转换为纯 base64 字符串
func convertToBase64(input string) string {
	// 如果已经是 data URI，提取 base64 部分
	if strings.HasPrefix(input, "data:") {
		// 格式: data:image/jpeg;base64,/9j/4AAQSkZJRg...
		parts := strings.SplitN(input, ",", 2)
		if len(parts) == 2 {
			return parts[1] // 返回 base64 部分
		}
	}

	// 如果是 HTTP/HTTPS URL，下载并转换为 base64
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		common.SysLog(fmt.Sprintf("[Veo] 检测到 URL，开始下载: %s", input))

		// 下载图片
		resp, err := http.Get(input)
		if err != nil {
			common.SysLog(fmt.Sprintf("[Veo] 下载图片失败: %v", err))
			return input // 返回原始 URL（虽然会失败，但至少不会 panic）
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			common.SysLog(fmt.Sprintf("[Veo] 下载图片失败，HTTP状态: %d", resp.StatusCode))
			return input
		}

		// 读取图片内容
		imageData, err := io.ReadAll(resp.Body)
		if err != nil {
			common.SysLog(fmt.Sprintf("[Veo] 读取图片内容失败: %v", err))
			return input
		}

		// 转换为 base64
		base64Str := base64.StdEncoding.EncodeToString(imageData)
		common.SysLog(fmt.Sprintf("[Veo] ✅ URL 下载并转换成功，base64 长度: %d", len(base64Str)))
		return base64Str
	}

	// 否则假设已经是纯 base64
	return input
}

// detectImageMimeType 检测图片的 MIME 类型
func detectImageMimeType(input string) string {
	// 如果是 data URI，提取 MIME 类型
	if strings.HasPrefix(input, "data:") {
		// 格式: data:image/jpeg;base64,/9j/4AAQSkZJRg...
		parts := strings.Split(input, ";")
		if len(parts) >= 1 {
			mimeType := strings.TrimPrefix(parts[0], "data:")
			if mimeType != "" {
				return mimeType
			}
		}
	}

	// 根据 base64 头部检测（魔数）
	if len(input) > 10 {
		// JPEG: /9j/
		if strings.HasPrefix(input, "/9j/") || strings.HasPrefix(input, "/9j4") {
			return "image/jpeg"
		}
		// PNG: iVBORw0KGgo
		if strings.HasPrefix(input, "iVBORw0KGgo") {
			return "image/png"
		}
		// WEBP: UklGR
		if strings.HasPrefix(input, "UklGR") {
			return "image/webp"
		}
	}

	// 默认返回 JPEG
	return "image/jpeg"
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse handles upstream response, returns taskID etc.
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var s submitResponse
	if err := json.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Name) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing operation name"), "invalid_response", http.StatusInternalServerError)
	}
	localID := encodeLocalTaskID(s.Name)
	c.JSON(http.StatusOK, gin.H{"task_id": localID})
	return localID, responseBody, nil
}

func (a *TaskAdaptor) GetModelList() []string { return []string{"veo-3.0-generate-001"} }
func (a *TaskAdaptor) GetChannelName() string { return "vertex" }

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	upstreamName, err := decodeLocalTaskID(taskID)
	if err != nil {
		return nil, fmt.Errorf("decode task_id failed: %w", err)
	}
	region := extractRegionFromOperationName(upstreamName)
	if region == "" {
		region = "us-central1"
	}
	project := extractProjectFromOperationName(upstreamName)
	modelName := extractModelFromOperationName(upstreamName)
	if project == "" || modelName == "" {
		return nil, fmt.Errorf("cannot extract project/model from operation name")
	}
	var url string
	if region == "global" {
		url = fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:fetchPredictOperation", project, modelName)
	} else {
		url = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:fetchPredictOperation", region, project, region, modelName)
	}
	payload := map[string]string{"operationName": upstreamName}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	adc := &vertexcore.Credentials{}
	if err := json.Unmarshal([]byte(key), adc); err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %w", err)
	}
	token, err := vertexcore.AcquireAccessToken(*adc, "")
	if err != nil {
		return nil, fmt.Errorf("failed to acquire access token: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-goog-user-project", adc.ProjectID)
	return service.GetHttpClient().Do(req)
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op operationResponse
	if err := json.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	ti := &relaycommon.TaskInfo{}
	if op.Error.Message != "" {
		ti.Status = string(model.TaskStatusFailure)
		ti.Reason = op.Error.Message
		ti.Progress = "100%"
		return ti, nil
	}
	if !op.Done {
		ti.Status = string(model.TaskStatusInProgress)
		ti.Progress = "50%"
		return ti, nil
	}
	ti.Status = string(model.TaskStatusSuccess)
	ti.Progress = "100%"

	dataURIs := make([]string, 0)
	for _, video := range op.Response.Videos {
		if video.BytesBase64Encoded == "" {
			continue
		}
		mime := strings.TrimSpace(video.MimeType)
		if mime == "" {
			enc := strings.TrimSpace(video.Encoding)
			if enc == "" {
				enc = "mp4"
			}
			if strings.Contains(enc, "/") {
				mime = enc
			} else {
				mime = "video/" + enc
			}
		}
		dataURIs = append(dataURIs, "data:"+mime+";base64,"+video.BytesBase64Encoded)
	}

	if op.Response.BytesBase64Encoded != "" {
		enc := strings.TrimSpace(op.Response.Encoding)
		if enc == "" {
			enc = "mp4"
		}
		mime := enc
		if !strings.Contains(enc, "/") {
			mime = "video/" + enc
		}
		dataURIs = append(dataURIs, "data:"+mime+";base64,"+op.Response.BytesBase64Encoded)
	}
	if op.Response.Video != "" {
		enc := strings.TrimSpace(op.Response.Encoding)
		if enc == "" {
			enc = "mp4"
		}
		mime := enc
		if !strings.Contains(enc, "/") {
			mime = "video/" + enc
		}
		dataURIs = append(dataURIs, "data:"+mime+";base64,"+op.Response.Video)
	}

	if len(dataURIs) > 0 {
		ti.Url = dataURIs[0]
		ti.Urls = dataURIs
		ti.Usage = &relaycommon.VideoUsage{
			VideoCount: len(dataURIs),
		}
	}

	return ti, nil
}

// ============================
// helpers
// ============================

func encodeLocalTaskID(name string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(name))
}

func decodeLocalTaskID(local string) (string, error) {
	b, err := base64.RawURLEncoding.DecodeString(local)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

var regionRe = regexp.MustCompile(`locations/([a-z0-9-]+)/`)

func extractRegionFromOperationName(name string) string {
	m := regionRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

var modelRe = regexp.MustCompile(`models/([^/]+)/operations/`)

func extractModelFromOperationName(name string) string {
	m := modelRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	idx := strings.Index(name, "models/")
	if idx >= 0 {
		s := name[idx+len("models/"):]
		if p := strings.Index(s, "/operations/"); p > 0 {
			return s[:p]
		}
	}
	return ""
}

var projectRe = regexp.MustCompile(`projects/([^/]+)/locations/`)

func extractProjectFromOperationName(name string) string {
	m := projectRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func deriveModelName(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) string {
	if info != nil && strings.TrimSpace(info.OriginModelName) != "" {
		return info.OriginModelName
	}
	if strings.TrimSpace(req.Model) != "" {
		return req.Model
	}
	return ""
}

func sanitizeFpsFromMetadata(metadata map[string]interface{}) int {
	fps := extractIntFromMetadata(metadata, "fps")
	if fps <= 0 {
		return 24
	}
	return fps
}

func supportsGenerateAudioParameter(modelName string) bool {
	if modelName == "" {
		return true
	}
	lower := strings.ToLower(modelName)
	return !(strings.Contains(lower, "-fast-") || strings.Contains(lower, "fast-"))
}

func sanitizeDurationSecondsFromMetadata(metadata map[string]interface{}) int {
	if seconds := extractIntFromMetadata(metadata, "durationSeconds", "duration_seconds"); seconds != 0 {
		switch seconds {
		case 4, 6, 8, 12:
			return seconds
		}
	}
	return 4
}

func sanitizeSampleCountFromMetadata(metadata map[string]interface{}) int {
	if count := extractIntFromMetadata(metadata, "sampleCount", "sample_count"); count > 0 {
		if count > 4 {
			return 4
		}
		return count
	}
	return 1
}

func sanitizeAspectRatioFromMetadata(metadata map[string]interface{}) string {
	ratio := strings.ReplaceAll(extractStringFromMetadata(metadata, "aspectRatio", "aspect_ratio"), " ", "")
	ratio = strings.ToLower(ratio)
	switch ratio {
	case "9:16", "9/16", "9-16":
		return "9:16"
	case "16:9", "16/9", "16-9":
		return "16:9"
	default:
		return "16:9"
	}
}

func sanitizeResolutionOptionFromMetadata(metadata map[string]interface{}) string {
	res := strings.ToLower(strings.TrimSpace(extractStringFromMetadata(metadata, "resolution")))
	switch {
	case strings.Contains(res, "720"):
		return "720p"
	case strings.Contains(res, "1080"):
		return "1080p"
	default:
		return "1080p"
	}
}

func resolveVertexResolution(option, aspectRatio string) string {
	switch option {
	case "720p":
		if aspectRatio == "9:16" {
			return "720x1280"
		}
		return "1280x720"
	case "1080p":
		if aspectRatio == "9:16" {
			return "1080x1920"
		}
		return "1920x1080"
	default:
		if aspectRatio == "9:16" {
			return "1080x1920"
		}
		return "1920x1080"
	}
}

func sanitizeGenerateAudio(metadata map[string]interface{}, fallback bool) bool {
	if val, ok := metadata["generateAudio"]; ok {
		if parsed, ok2 := toBool(val); ok2 {
			fallback = parsed
		}
	}
	if val, ok := metadata["generate_audio"]; ok {
		if parsed, ok2 := toBool(val); ok2 {
			fallback = parsed
		}
	}
	return fallback
}

func sanitizePersonGenerationFromMetadata(metadata map[string]interface{}) string {
	value := strings.ToLower(strings.TrimSpace(extractStringFromMetadata(metadata, "personGeneration", "person_generation")))
	switch value {
	case "allow_adult", "dont_allow":
		return value
	default:
		return "allow_all"
	}
}

func sanitizeBoolMetadata(metadata map[string]interface{}, keys []string, defaultValue bool) bool {
	result := defaultValue
	for _, key := range keys {
		if val, ok := metadata[key]; ok {
			if parsed, ok2 := toBool(val); ok2 {
				result = parsed
				break
			}
		}
	}
	return result
}

func extractIntFromMetadata(metadata map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if val, ok := metadata[key]; ok {
			if parsed, ok2 := toInt(val); ok2 {
				return parsed
			}
		}
	}
	return 0
}

func extractStringFromMetadata(metadata map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := metadata[key]; ok {
			if str := toString(val); str != "" {
				return str
			}
		}
	}
	return ""
}

func extractPositiveInt(m map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if val, ok := m[key]; ok {
			if parsed, ok2 := toInt(val); ok2 && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}
