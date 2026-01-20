package sora

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	"one-api/relay/channel"
	relaycommon "one-api/relay/common"
	"one-api/service"
	"one-api/setting/system_setting"
	"strings"

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
	relayInfo   *relaycommon.RelayInfo // 保存 RelayInfo 以便后续使用模型特定的 API 版本
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
	a.relayInfo = info // 保存 RelayInfo 以便后续使用
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// 提取请求中的seconds参数用于计费
	cachedBody, err := common.GetRequestBody(c)
	if err == nil {
		var requestMap map[string]interface{}
		if json.Unmarshal(cachedBody, &requestMap) == nil {
			// 兼容前端传递的两种参数名：seconds 或 n_seconds
			videoSeconds := 0
			if seconds, ok := requestMap["seconds"].(float64); ok && seconds > 0 {
				videoSeconds = int(seconds)
				common.SysLog(fmt.Sprintf("[Sora] 从请求中提取时长参数 seconds: %d", videoSeconds))
			} else if nSeconds, ok := requestMap["n_seconds"].(float64); ok && nSeconds > 0 {
				videoSeconds = int(nSeconds)
				common.SysLog(fmt.Sprintf("[Sora] 从请求中提取时长参数 n_seconds: %d", videoSeconds))
			}

			if videoSeconds > 0 {
				c.Set("video_seconds", videoSeconds)
				common.SysLog(fmt.Sprintf("[Sora] 设置计费时长: %d秒", videoSeconds))
			}
		}
	}

	return relaycommon.ValidateMultipartDirect(c, info)
}

// getAPIVersion 获取正确的 API 版本，优先使用模型特定的版本配置
func (a *TaskAdaptor) getAPIVersion(defaultVersion string) string {
	if a.ChannelType != constant.ChannelTypeAzure {
		return defaultVersion
	}

	// 如果配置了模型特定的 API 版本，优先使用模型特定的版本
	if a.relayInfo != nil {
		// 检查是否有模型特定的 API 版本配置
		if len(a.relayInfo.ChannelOtherSettings.AzureModelApiVersions) > 0 {
			if modelApiVersion, exists := a.relayInfo.ChannelOtherSettings.AzureModelApiVersions[a.relayInfo.UpstreamModelName]; exists && modelApiVersion != "" {
				common.SysLog(fmt.Sprintf("[Sora2] 使用模型特定的 API 版本: %s (模型: %s)", modelApiVersion, a.relayInfo.UpstreamModelName))
				return modelApiVersion
			}
		}
	}

	// 使用默认版本
	if defaultVersion == "" {
		return "preview"
	}
	return defaultVersion
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var url string
	// Azure OpenAI Sora 2 API
	if a.ChannelType == constant.ChannelTypeAzure {
		apiVersion := a.getAPIVersion(info.ApiVersion)
		url = fmt.Sprintf("%s/openai/v1/videos?api-version=%s", a.baseURL, apiVersion)
	} else {
		// 原生 OpenAI Sora API
		url = fmt.Sprintf("%s/v1/videos", a.baseURL)
	}

	common.SysLog(fmt.Sprintf("[Sora2] 提交任务请求URL: %s", url))
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

	// 检查是否是 multipart 请求（有图片上传）
	if isMultipart, exists := c.Get("is_multipart"); exists && isMultipart.(bool) {
		if boundary, exists := c.Get("multipart_boundary"); exists {
			contentType := fmt.Sprintf("multipart/form-data; boundary=%s", boundary.(string))
			req.Header.Set("Content-Type", contentType)
			common.SysLog(fmt.Sprintf("[Sora] 设置multipart Content-Type: %s", contentType))
		}
	} else {
		// 普通JSON请求（无图片）
		req.Header.Set("Content-Type", "application/json")
	}

	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	cachedBody, err := common.GetRequestBody(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}

	// 解析请求体，进行参数转换
	var requestMap map[string]interface{}
	if err := json.Unmarshal(cachedBody, &requestMap); err != nil {
		common.SysError(fmt.Sprintf("[Sora] 解析请求体失败: %v", err))
		return bytes.NewReader(cachedBody), nil
	}

	// Sora 2 统一参数处理
	// 1. 统一处理 seconds 参数（支持多种字段名和格式）
	// 优先级：seconds > n_seconds > 默认值
	secondsValue := ""

	// 1.1 优先使用 seconds 参数
	if seconds, exists := requestMap["seconds"]; exists && seconds != nil {
		switch v := seconds.(type) {
		case string:
			if v != "" {
				secondsValue = v
				common.SysLog(fmt.Sprintf("[Sora2] 使用seconds参数: %s", secondsValue))
			}
		case int:
			secondsValue = fmt.Sprintf("%d", v)
			common.SysLog(fmt.Sprintf("[Sora2] 使用seconds参数(int): %d", v))
		case float64:
			secondsValue = fmt.Sprintf("%.0f", v)
			common.SysLog(fmt.Sprintf("[Sora2] 使用seconds参数(float64): %.0f", v))
		}
	}

	// 1.2 如果没有seconds，尝试n_seconds
	if secondsValue == "" {
		if nSeconds, exists := requestMap["n_seconds"]; exists && nSeconds != nil {
			switch v := nSeconds.(type) {
			case string:
				if v != "" {
					secondsValue = v
					common.SysLog(fmt.Sprintf("[Sora2] 使用n_seconds参数: %s", secondsValue))
				}
			case int:
				secondsValue = fmt.Sprintf("%d", v)
				common.SysLog(fmt.Sprintf("[Sora2] 使用n_seconds参数(int): %d", v))
			case float64:
				secondsValue = fmt.Sprintf("%.0f", v)
				common.SysLog(fmt.Sprintf("[Sora2] 使用n_seconds参数(float64): %.0f", v))
			}
			// 删除n_seconds，统一使用seconds
			delete(requestMap, "n_seconds")
		}
	}

	// 1.3 验证seconds值（Sora 2只支持4/8/12秒）
	validSeconds := []string{"4", "8", "12"}
	if secondsValue != "" {
		isValidSeconds := false
		for _, validSec := range validSeconds {
			if secondsValue == validSec {
				isValidSeconds = true
				break
			}
		}
		if !isValidSeconds {
			common.SysError(fmt.Sprintf("[Sora2] 无效的seconds参数: %s，仅支持: %v，使用默认值4秒", secondsValue, validSeconds))
			secondsValue = "4"
		}
	} else {
		secondsValue = "4" // 默认值
		common.SysLog("[Sora2] 未提供seconds/n_seconds参数，使用默认值: 4秒")
	}

	// 1.4 设置最终的seconds参数
	requestMap["seconds"] = secondsValue

	// 2. 验证并转换 size 参数（Sora 2 只支持 720x1280 和 1280x720）
	// 优先级：size > width+height > 默认值

	// 首先检查是否已有 size 参数
	if size, exists := requestMap["size"]; exists {
		// 情况1：有 size，直接验证
		sizeStr, ok := size.(string)
		if ok && sizeStr != "" {
			if sizeStr != "720x1280" && sizeStr != "1280x720" {
				common.SysError(fmt.Sprintf("[Sora2] size参数错误: %s, 仅支持 720x1280(竖屏) 或 1280x720(横屏)，已自动修正为720x1280", sizeStr))
				requestMap["size"] = "720x1280"
			} else {
				common.SysLog(fmt.Sprintf("[Sora2] size参数验证通过: %s", sizeStr))
			}
			// 删除 width 和 height（如果有的话）
			delete(requestMap, "width")
			delete(requestMap, "height")
		} else {
			// size 存在但为空，尝试从 width/height 获取
			delete(requestMap, "size")
		}
	}

	// 如果还没有 size，尝试从 width 和 height 转换
	if _, exists := requestMap["size"]; !exists {
		width := 0
		height := 0
		if w, ok := requestMap["width"].(float64); ok {
			width = int(w)
		} else if w, ok := requestMap["width"].(int); ok {
			width = w
		}
		if h, ok := requestMap["height"].(float64); ok {
			height = int(h)
		} else if h, ok := requestMap["height"].(int); ok {
			height = h
		}

		if width > 0 && height > 0 {
			// 情况2：没有 size，但有 width 和 height，转换为 size
			sizeStr := fmt.Sprintf("%dx%d", width, height)
			if sizeStr != "720x1280" && sizeStr != "1280x720" {
				common.SysError(fmt.Sprintf("[Sora2] 从 width=%d, height=%d 转换的 size=%s 不符合要求，使用默认值 720x1280", width, height, sizeStr))
				requestMap["size"] = "720x1280"
			} else {
				requestMap["size"] = sizeStr
				common.SysLog(fmt.Sprintf("[Sora2] 参数转换: width=%d, height=%d → size=%s", width, height, sizeStr))
			}
		} else {
			// 情况3：既没有 size，也没有 width/height，使用默认值
			requestMap["size"] = "720x1280"
			common.SysLog("[Sora2] 未提供size/width/height参数，使用默认值: 720x1280")
		}

		// 删除 width 和 height 参数
		delete(requestMap, "width")
		delete(requestMap, "height")
	}

	// 3. 统一处理 remix 参数（支持多种字段名）
	isRemixMode := false
	remixVideoID := ""

	// 3.1 检查多种可能的 remix 参数名
	remixFields := []string{"remix_video_id", "remixed_video_id", "remixedVideoId", "remixVideoId"}
	for _, field := range remixFields {
		if remixID, exists := requestMap[field]; exists && remixID != nil {
			if remixStr, ok := remixID.(string); ok && remixStr != "" {
				// 验证格式（必须是video_开头）
				if strings.HasPrefix(remixStr, "video_") {
					remixVideoID = remixStr
					isRemixMode = true
					common.SysLog(fmt.Sprintf("[Sora2] 检测到Remix模式 - 字段: %s, VideoID: %s", field, remixStr))
					break
				} else {
					common.SysError(fmt.Sprintf("[Sora2] %s格式错误: %s, 必须是video_开头的字符串", field, remixStr))
					delete(requestMap, field)
				}
			}
		}
	}

	// 3.2 处理 Remix 模式
	if isRemixMode {
		// Remix 模式：将 video_id 存到 context，供 BuildRequestURL 和 DoRequest 使用
		c.Set("sora_remix_video_id", remixVideoID)

		// ⚠️ Remix API 只接受 prompt 参数！其他参数（size、seconds、model）都不能传
		promptValue := requestMap["prompt"]
		requestMap = map[string]interface{}{
			"prompt": promptValue,
		}
		common.SysLog("[Sora2] Remix模式：仅保留 prompt 参数，删除所有其他参数")
	}

	// 4. 如果不是 Remix 模式，使用白名单方式只保留 Sora2 API 支持的参数
	if !isRemixMode {
		// Sora2 API 支持的参数白名单（根据官方文档）
		// 支持: model, prompt, seconds, size, input_reference, user
		// width 和 height 已转换为 size，不再需要
		allowedParams := map[string]bool{
			"model":           true,
			"prompt":          true,
			"seconds":         true,
			"size":            true,
			"input_reference": true,
			"user":            true,
		}

		// 创建新的请求体，只包含白名单中的参数
		filteredMap := make(map[string]interface{})
		for key, value := range requestMap {
			if allowedParams[key] {
				filteredMap[key] = value
			}
		}
		requestMap = filteredMap
		common.SysLog("[Sora2] 已使用白名单过滤，只保留 Sora2 API 支持的参数")
	}

	// 打印最终请求参数（用于调试）
	if model, exists := requestMap["model"]; exists {
		sizeStr := ""
		if size, ok := requestMap["size"].(string); ok {
			sizeStr = size
		}
		secondsStr := ""
		if seconds, ok := requestMap["seconds"].(string); ok {
			secondsStr = seconds
		}
		common.SysLog(fmt.Sprintf("[Sora2] 最终参数 - 模型: %v, 分辨率: %s, 时长: %s秒", model, sizeStr, secondsStr))
	}

	// 检查是否有 input_reference 参数（参考图）
	if inputRef, exists := requestMap["input_reference"]; exists && inputRef != nil {
		inputRefStr, ok := inputRef.(string)
		if !ok {
			common.SysError("[Sora] input_reference 必须是字符串类型")
			delete(requestMap, "input_reference")
		} else {
			common.SysLog("[Sora] 检测到 input_reference 参数，准备转换为文件流")

			// 将图片（URL/base64）转换为字节数组
			imageBytes, mimeType, err := convertImageToBytes(inputRefStr)
			if err != nil {
				common.SysError(fmt.Sprintf("[Sora] 图片转换失败: %v", err))
				// 转换失败，删除参数，继续以纯文本模式提交
				delete(requestMap, "input_reference")
			} else {
				// 转换成功，构建 multipart/form-data 请求
				common.SysLog("[Sora] 图片转换成功，使用multipart/form-data格式")
				return buildMultipartRequest(c, requestMap, imageBytes, mimeType)
			}

			// ========== 以下是JSON+base64方案代码（保留备用）==========
			// 注释说明：Azure Sora API 目前需要multipart格式，暂不支持JSON中直接传base64
			// 如果未来Azure支持JSON格式，可以启用以下代码：
			//
			// if strings.HasPrefix(inputRefStr, "http://") || strings.HasPrefix(inputRefStr, "https://") {
			//     // URL格式 → 下载并转换为base64
			//     imageBytes, mimeType, _ := convertImageToBytes(inputRefStr)
			//     base64Str := base64.StdEncoding.EncodeToString(imageBytes)
			//     requestMap["input_reference"] = fmt.Sprintf("data:%s;base64,%s", mimeType, base64Str)
			// } else if !strings.HasPrefix(inputRefStr, "data:image/") {
			//     // 纯base64 → 添加data:前缀
			//     requestMap["input_reference"] = fmt.Sprintf("data:image/jpeg;base64,%s", inputRefStr)
			// }
			// // 已经是data:image/格式 → 保持不变
			// ========================================================
		}
	}

	// 没有图片，使用 JSON 格式
	convertedBody, err := json.Marshal(requestMap)
	if err != nil {
		common.SysError(fmt.Sprintf("[Sora] 序列化转换后的请求体失败: %v", err))
		return bytes.NewReader(cachedBody), nil
	}

	// 打印转换后的请求体（用于调试）
	truncatedBody := common.TruncateBase64Content(string(convertedBody))
	common.SysLog(fmt.Sprintf("[Sora] 转换后的请求体(JSON): %s", truncatedBody))

	return bytes.NewReader(convertedBody), nil
}

// buildMultipartRequest 构建 multipart/form-data 格式的请求
func buildMultipartRequest(c *gin.Context, params map[string]interface{}, imageBytes []byte, mimeType string) (io.Reader, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	common.SysLog("[Sora] 开始构建multipart请求")

	// 1. 添加所有非input_reference参数为表单字段
	for key, value := range params {
		if key == "input_reference" {
			continue // 跳过，单独处理
		}

		// 将值转换为字符串
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case float64:
			valueStr = fmt.Sprintf("%.0f", v)
		case int:
			valueStr = fmt.Sprintf("%d", v)
		default:
			// 复杂对象转JSON
			jsonBytes, _ := json.Marshal(v)
			valueStr = string(jsonBytes)
		}

		if err := writer.WriteField(key, valueStr); err != nil {
			return nil, fmt.Errorf("写入字段 %s 失败: %v", key, err)
		}
		// 仅打印字段名，避免在日志中泄露表单值
		common.SysLog(fmt.Sprintf("[Sora] 添加表单字段: %s", key))
	}

	// 2. 添加图片文件（手动设置正确的MIME类型）
	fileName := "reference_image" + getFileExtensionFromMimeType(mimeType)

	// 使用CreatePart手动设置Content-Type，而不是CreateFormFile（它会设置为application/octet-stream）
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="input_reference"; filename="%s"`, fileName))
	h.Set("Content-Type", mimeType) // ⚠️ 关键：设置正确的MIME类型（image/jpeg, image/png, image/webp）

	part, err := writer.CreatePart(h)
	if err != nil {
		common.SysError(fmt.Sprintf("[Sora] 创建文件字段失败: %v", err))
		return nil, err
	}

	if _, err := part.Write(imageBytes); err != nil {
		common.SysError(fmt.Sprintf("[Sora] 写入图片数据失败: %v", err))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Sora] ✅ 添加图片文件: %s, 大小: %d bytes, MIME: %s", fileName, len(imageBytes), mimeType))

	// 3. 关闭writer
	if err := writer.Close(); err != nil {
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Sora] multipart请求构建完成，Content-Type: %s", writer.FormDataContentType()))

	// 4. 将boundary存储到context中，供BuildRequestHeader使用
	c.Set("multipart_boundary", writer.Boundary())
	c.Set("is_multipart", true)

	return body, nil
}

// getFileExtensionFromMimeType 根据MIME类型获取文件扩展名
func getFileExtensionFromMimeType(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg"
	}
}

// DoRequest delegates to common helper.
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	// 检查是否是 Remix 模式，需要修改 URL
	if remixVideoID, exists := c.Get("sora_remix_video_id"); exists && remixVideoID != nil {
		if remixStr, ok := remixVideoID.(string); ok && remixStr != "" {
			// Remix 模式：修改 URL 为 /videos/{video_id}/remix
			apiVersion := a.getAPIVersion(info.ApiVersion)

			remixURL := fmt.Sprintf("%s/openai/v1/videos/%s/remix?api-version=%s", a.baseURL, remixStr, apiVersion)
			common.SysLog(fmt.Sprintf("[Sora2] Remix模式 - 修改URL为: %s", remixURL))

			// 直接构建 Remix 请求
			req, err := http.NewRequest(http.MethodPost, remixURL, requestBody)
			if err != nil {
				return nil, err
			}

			// 设置请求头
			req.Header.Set("Api-key", a.apiKey)
			req.Header.Set("Content-Type", "application/json")

			// 发送请求
			return service.GetHttpClient().Do(req)
		}
	}

	// 普通模式（文生/图生）使用标准流程
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

	// 按照统一视频生成接口文档的格式发送响应，包含原厂响应metadata
	// 将原厂响应解析为 map 作为 metadata
	var metadata map[string]interface{}
	if err := json.Unmarshal(responseBody, &metadata); err != nil {
		metadata = nil
		common.SysError(fmt.Sprintf("[Sora] 解析原厂响应为metadata失败: %v", err))
	}

	responseData := gin.H{
		"task_id":  dResp.ID,
		"status":   "submitted",
		"format":   "mp4",
		"metadata": metadata, // 添加原厂完整响应
	}
	c.JSON(http.StatusOK, responseData)
	return dResp.ID, responseBody, nil
}

// FetchTask fetch task status
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	var uri string
	// Azure OpenAI Sora 2 API
	if a.ChannelType == constant.ChannelTypeAzure {
		// 使用模型特定的 API 版本，如果没有配置则使用默认版本
		apiVersion := a.getAPIVersion("")
		uri = fmt.Sprintf("%s/openai/v1/videos/%s?api-version=%s", baseUrl, taskID, apiVersion)
	} else {
		// 原生 OpenAI Sora API
		uri = fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)
	}

	common.SysLog(fmt.Sprintf("[Sora2] 查询任务状态 - TaskID: %s, URL: %s", taskID, uri))

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("[Sora] 创建请求失败 - TaskID: %s, Error: %v", taskID, err))
		return nil, err
	}

	// 设置认证 header
	if a.ChannelType == constant.ChannelTypeAzure {
		req.Header.Set("Api-key", key)
		common.SysLog(fmt.Sprintf("[Sora] Azure认证 - Api-key: %s", key))
	} else {
		req.Header.Set("Authorization", "Bearer "+key)
		common.SysLog(fmt.Sprintf("[Sora] OpenAI认证 - Bearer token: %s", key))
	}

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("[Sora] 调用厂商接口失败 - TaskID: %s, Error: %v", taskID, err))
		return nil, err
	}

	common.SysLog(fmt.Sprintf("[Sora] 厂商接口响应 - TaskID: %s, StatusCode: %d", taskID, resp.StatusCode))
	return resp, nil
}

// FetchVideoContent 获取生成的视频内容（Azure 需要额外的 API 调用）
func (a *TaskAdaptor) FetchVideoContent(baseUrl, key, jobID, genID string) (*http.Response, error) {
	var uri string

	if a.ChannelType == constant.ChannelTypeAzure {
		// 使用模型特定的 API 版本，如果没有配置则使用默认版本
		apiVersion := a.getAPIVersion("")
		uri = fmt.Sprintf("%s/openai/v1/videos/%s/content?variant=video&api-version=%s", baseUrl, jobID, apiVersion)
	} else {
		// 原生 OpenAI: /v1/videos/{video_id}/content?variant=video
		uri = fmt.Sprintf("%s/v1/videos/%s/content?variant=video", baseUrl, jobID)
	}

	common.SysLog(fmt.Sprintf("[Sora2] 获取视频内容URL: %s", uri))

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

	// 打印解析任务结果（Sora 2 不再返回 Generations 数组）
	common.SysLog(fmt.Sprintf("[Sora2] 解析任务结果 - VideoID: %s, Status: %s, Seconds: %s, Size: %s",
		resTask.ID, resTask.Status, resTask.Seconds, resTask.Size))

	// 打印关键参数用于调试
	common.SysLog(fmt.Sprintf("[Sora2] 任务详细信息 - Model: %s, Prompt: %.50s...",
		resTask.Model, resTask.Prompt))

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Sora 2 官方文档规定的4种状态：queued, in_progress, completed, failed
	switch resTask.Status {
	case "queued":
		// 任务在队列中等待
		taskResult.Status = string(model.TaskStatusQueued)
		taskResult.Progress = "0%"
		common.SysLog(fmt.Sprintf("[Sora2] 📋 任务排队中 - VideoID: %s", resTask.ID))

	case "in_progress":
		// 任务正在处理中
		taskResult.Status = string(model.TaskStatusInProgress)
		if resTask.Progress > 0 {
			taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
		} else {
			taskResult.Progress = "50%"
		}
		common.SysLog(fmt.Sprintf("[Sora2] ⚙️ 任务处理中 - VideoID: %s, Progress: %d%%", resTask.ID, resTask.Progress))

	case "completed":
		// 任务成功完成
		taskResult.Status = string(model.TaskStatusSuccess)
		taskResult.Progress = "100%"

		// 打印成功时的完整信息
		common.SysLog(fmt.Sprintf("[Sora2] ✅ 任务成功 - VideoID: %s, Seconds: %s, Size: %s, Model: %s",
			resTask.ID, resTask.Seconds, resTask.Size, resTask.Model))

		// Sora 2: 直接使用 video_id，不再需要 genID
		// 返回系统代理 URL，由系统处理实际的视频下载
		taskResult.Url = fmt.Sprintf("%s/v1/videos/%s/content?variant=video",
			system_setting.ServerAddress, resTask.ID)

		common.SysLog(fmt.Sprintf("[Sora2] 生成代理URL: %s", taskResult.Url))

	case "failed":
		// 任务失败
		taskResult.Status = string(model.TaskStatusFailure)
		taskResult.Progress = "100%"
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else if resTask.FailureReason != "" {
			taskResult.Reason = resTask.FailureReason
		} else {
			taskResult.Reason = "task failed"
		}
		common.SysLog(fmt.Sprintf("[Sora2] ❌ 任务失败 - VideoID: %s, 原因: %s", resTask.ID, taskResult.Reason))

	default:
		// 未知状态（不应该出现，记录错误）
		common.SysError(fmt.Sprintf("[Sora2] ⚠️ 检测到非官方状态: %s (官方仅支持: queued/in_progress/completed/failed)", resTask.Status))
		// 根据具体情况判断
		if resTask.Progress == 100 {
			taskResult.Status = string(model.TaskStatusSuccess)
			taskResult.Progress = "100%"
		} else {
			taskResult.Status = string(model.TaskStatusInProgress)
			taskResult.Progress = "50%"
		}
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

// ============================
// 图片处理辅助函数
// ============================

// convertImageToBytes 将图片URL或base64转换为字节数组
// 支持三种格式：
// 1. data:image/xxx;base64,... (base64格式)
// 2. http://... 或 https://... (URL格式)
// 3. 直接的base64字符串
func convertImageToBytes(imageInput string) ([]byte, string, error) {
	// 1. 检查是否是 base64 格式 (data:image/jpeg;base64,...)
	if strings.HasPrefix(imageInput, "data:image/") {
		common.SysLog("[Sora] 检测到base64格式图片")

		// 提取MIME类型
		mimeType := "image/jpeg" // 默认
		if strings.Contains(imageInput, "image/png") {
			mimeType = "image/png"
		} else if strings.Contains(imageInput, "image/webp") {
			mimeType = "image/webp"
		}

		// 提取base64数据部分
		parts := strings.Split(imageInput, ",")
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("invalid base64 format")
		}

		// 解码base64
		imageBytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			common.SysError(fmt.Sprintf("[Sora] base64解码失败: %v", err))
			return nil, "", err
		}

		common.SysLog(fmt.Sprintf("[Sora] base64解码成功，图片大小: %d bytes, MIME: %s", len(imageBytes), mimeType))
		return imageBytes, mimeType, nil
	}

	// 2. 检查是否是 URL 格式
	if strings.HasPrefix(imageInput, "http://") || strings.HasPrefix(imageInput, "https://") {
		common.SysLog(fmt.Sprintf("[Sora] 检测到URL格式图片: %s", imageInput))

		// 下载图片
		resp, err := http.Get(imageInput)
		if err != nil {
			common.SysError(fmt.Sprintf("[Sora] 下载图片失败: %v", err))
			return nil, "", err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("download image failed, status code: %d", resp.StatusCode)
		}

		// 读取图片数据
		imageBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			common.SysError(fmt.Sprintf("[Sora] 读取图片数据失败: %v", err))
			return nil, "", err
		}

		// 从Content-Type获取MIME类型
		mimeType := resp.Header.Get("Content-Type")
		if mimeType == "" {
			mimeType = "image/jpeg" // 默认
		}

		common.SysLog(fmt.Sprintf("[Sora] URL图片下载成功，大小: %d bytes, MIME: %s", len(imageBytes), mimeType))
		return imageBytes, mimeType, nil
	}

	// 3. 尝试直接作为base64解码（无data:前缀）
	common.SysLog("[Sora] 尝试作为纯base64字符串解码")
	imageBytes, err := base64.StdEncoding.DecodeString(imageInput)
	if err != nil {
		return nil, "", fmt.Errorf("无效的图片格式，仅支持base64或URL")
	}

	common.SysLog(fmt.Sprintf("[Sora] 纯base64解码成功，图片大小: %d bytes", len(imageBytes)))
	return imageBytes, "image/jpeg", nil
}
