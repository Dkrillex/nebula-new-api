package openai

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/relay/channel"
	"one-api/relay/channel/ai360"
	"one-api/relay/channel/lingyiwanwu"
	"one-api/relay/channel/minimax"
	"one-api/relay/channel/openrouter"
	"one-api/relay/channel/xinference"
	relaycommon "one-api/relay/common"
	"one-api/relay/common_handler"
	relayconstant "one-api/relay/constant"
	"one-api/service"
	"one-api/types"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	ChannelType    int
	ResponseFormat string
}

// parseReasoningEffortFromModelSuffix 从模型名称中解析推理级别
// support OAI models: o1-mini/o3-mini/o4-mini/o1/o3 etc...
// minimal effort only available in gpt-5
func parseReasoningEffortFromModelSuffix(model string) (string, string) {
	effortSuffixes := []string{"-high", "-minimal", "-low", "-medium"}
	for _, suffix := range effortSuffixes {
		if strings.HasSuffix(model, suffix) {
			effort := strings.TrimPrefix(suffix, "-")
			originModel := strings.TrimSuffix(model, suffix)
			return effort, originModel
		}
	}
	return "", model
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	// 使用 service.GeminiToOpenAIRequest 转换请求格式
	openaiRequest, err := service.GeminiToOpenAIRequest(request, info)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, openaiRequest)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	//if !strings.Contains(request.Model, "claude") {
	//	return nil, fmt.Errorf("you are using openai channel type with path /v1/messages, only claude model supported convert, but got %s", request.Model)
	//}
	//if common.DebugEnabled {
	//	bodyBytes := []byte(common.GetJsonString(request))
	//	err := os.WriteFile(fmt.Sprintf("claude_request_%s.txt", c.GetString(common.RequestIdKey)), bodyBytes, 0644)
	//	if err != nil {
	//		println(fmt.Sprintf("failed to save request body to file: %v", err))
	//	}
	//}
	aiRequest, err := service.ClaudeToOpenAIRequest(*request, info)
	if err != nil {
		return nil, err
	}
	//if common.DebugEnabled {
	//	println(fmt.Sprintf("convert claude to openai request result: %s", common.GetJsonString(aiRequest)))
	//	// Save request body to file for debugging
	//	bodyBytes := []byte(common.GetJsonString(aiRequest))
	//	err = os.WriteFile(fmt.Sprintf("claude_to_openai_request_%s.txt", c.GetString(common.RequestIdKey)), bodyBytes, 0644)
	//	if err != nil {
	//		println(fmt.Sprintf("failed to save request body to file: %v", err))
	//	}
	//}
	if info.SupportStreamOptions && info.IsStream {
		aiRequest.StreamOptions = &dto.StreamOptions{
			IncludeUsage: true,
		}
	}
	return a.ConvertOpenAIRequest(c, info, aiRequest)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType

	// initialize ThinkingContentInfo when thinking_to_content is enabled
	if info.ChannelSetting.ThinkingToContent {
		info.ThinkingContentInfo = relaycommon.ThinkingContentInfo{
			IsFirstThinkingContent:  true,
			SendLastThinkingContent: false,
			HasSentThinkingContent:  false,
		}
	}
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.RelayMode == relayconstant.RelayModeRealtime {
		if strings.HasPrefix(info.ChannelBaseUrl, "https://") {
			baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "https://")
			baseUrl = "wss://" + baseUrl
			info.ChannelBaseUrl = baseUrl
		} else if strings.HasPrefix(info.ChannelBaseUrl, "http://") {
			baseUrl := strings.TrimPrefix(info.ChannelBaseUrl, "http://")
			baseUrl = "ws://" + baseUrl
			info.ChannelBaseUrl = baseUrl
		}
	}
	switch info.ChannelType {
	case constant.ChannelTypeAzure:
		apiVersion := info.ApiVersion
		if apiVersion == "" {
			apiVersion = constant.AzureDefaultAPIVersion
		}
		// 如果配置了模型特定的 API 版本，优先使用模型特定的版本（适用于普通 API 和 Responses API）
		if len(info.ChannelOtherSettings.AzureModelApiVersions) > 0 {
			if modelApiVersion, exists := info.ChannelOtherSettings.AzureModelApiVersions[info.UpstreamModelName]; exists && modelApiVersion != "" {
				apiVersion = modelApiVersion
			}
		}
		// 文件上传走资源级别接口，不依赖 deployment
		if info.RelayMode == relayconstant.RelayModeFiles {
			requestURL := "/openai/v1/files"
			if apiVersion != "" {
				requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, apiVersion)
			}
			return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
		}
		// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/chatgpt-quickstart?pivots=rest-api&tabs=command-line#rest-api
		requestURL := strings.Split(info.RequestURLPath, "?")[0]
		requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, apiVersion)
		task := strings.TrimPrefix(requestURL, "/v1/")
		// 同时处理 /api/sync/system/ 前缀（外部系统接口）
		task = strings.TrimPrefix(task, "/api/sync/system/")

		if info.RelayFormat == types.RelayFormatClaude {
			task = strings.TrimPrefix(task, "messages")
			task = "chat/completions" + task
		}

		// 特殊处理 responses API
		if info.RelayMode == relayconstant.RelayModeResponses {
			responsesApiVersion := apiVersion // 已经应用了模型特定的版本（如果有）

			// 统一使用 /openai/responses（取消 /v1 前缀，兼容云策/官方 Azure）
			subUrl := "/openai/responses"
			// 官方 Azure 域名沿用默认 apiVersion；其他厂商若未设置，在下方用 AzureResponsesVersion 覆盖
			if strings.Contains(info.ChannelBaseUrl, "cognitiveservices.azure.com") {
				// 如果模型特定版本已设置，使用它；否则使用默认版本
				if apiVersion == constant.AzureDefaultAPIVersion {
					responsesApiVersion = apiVersion
				}
			}

			// 如果配置了默认 Responses API 版本且没有模型特定的版本，使用默认 Responses API 版本
			if info.ChannelOtherSettings.AzureResponsesVersion != "" {
				// 检查是否使用了模型特定的版本，如果没有则使用默认 Responses API 版本
				if len(info.ChannelOtherSettings.AzureModelApiVersions) == 0 {
					responsesApiVersion = info.ChannelOtherSettings.AzureResponsesVersion
				} else if _, exists := info.ChannelOtherSettings.AzureModelApiVersions[info.UpstreamModelName]; !exists {
					// 如果模型不在模型特定配置中，使用默认 Responses API 版本
					responsesApiVersion = info.ChannelOtherSettings.AzureResponsesVersion
				}
			}

			requestURL = fmt.Sprintf("%s?api-version=%s", subUrl, responsesApiVersion)
			return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
		}

		model_ := info.UpstreamModelName
		// 2025年5月10日后创建的渠道不移除.
		if info.ChannelCreateTime < constant.AzureNoRemoveDotTime {
			model_ = strings.Replace(model_, ".", "", -1)
		}
		// https://github.com/songquanpeng/one-api/issues/67
		requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)
		if info.RelayMode == relayconstant.RelayModeRealtime {
			requestURL = fmt.Sprintf("/openai/realtime?deployment=%s&api-version=%s", model_, apiVersion)
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, requestURL, info.ChannelType), nil
	case constant.ChannelTypeMiniMax:
		return minimax.GetRequestURL(info)
	case constant.ChannelTypeCustom:
		url := info.ChannelBaseUrl
		url = strings.Replace(url, "{model}", info.UpstreamModelName, -1)
		return url, nil
	default:
		if info.RelayFormat == types.RelayFormatClaude || info.RelayFormat == types.RelayFormatGemini {
			return fmt.Sprintf("%s/v1/chat/completions", info.ChannelBaseUrl), nil
		}
		return relaycommon.GetFullRequestURL(info.ChannelBaseUrl, info.RequestURLPath, info.ChannelType), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, header *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, header)
	if info.ChannelType == constant.ChannelTypeAzure {
		header.Set("api-key", info.ApiKey)
		return nil
	}
	if info.ChannelType == constant.ChannelTypeOpenAI && "" != info.Organization {
		header.Set("OpenAI-Organization", info.Organization)
	}
	if info.RelayMode == relayconstant.RelayModeRealtime {
		// 对接自建 Realtime 服务：统一使用 Authorization 头，Sec-WebSocket-Protocol 仅为 realtime
		header.Set("Sec-WebSocket-Protocol", "realtime")
		header.Set("Authorization", "Bearer "+info.ApiKey)
		header.Set("openai-beta", "realtime=v1")
	} else {
		header.Set("Authorization", "Bearer "+info.ApiKey)
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		header.Set("HTTP-Referer", "https://www.newapi.ai")
		header.Set("X-Title", "New API")
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if info.ChannelType != constant.ChannelTypeOpenAI && info.ChannelType != constant.ChannelTypeAzure {
		request.StreamOptions = nil
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter {
		if len(request.Usage) == 0 {
			request.Usage = json.RawMessage(`{"include":true}`)
		}
		// 适配 OpenRouter 的 thinking 后缀
		if strings.HasSuffix(info.UpstreamModelName, "-thinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
			request.Model = info.UpstreamModelName
			if len(request.Reasoning) == 0 {
				reasoning := map[string]any{
					"enabled": true,
				}
				if request.ReasoningEffort != "" && request.ReasoningEffort != "none" {
					reasoning["effort"] = request.ReasoningEffort
				}
				marshal, err := common.Marshal(reasoning)
				if err != nil {
					return nil, fmt.Errorf("error marshalling reasoning: %w", err)
				}
				request.Reasoning = marshal
			}
			// 清空多余的ReasoningEffort
			request.ReasoningEffort = ""
		} else {
			if len(request.Reasoning) == 0 {
				// 适配 OpenAI 的 ReasoningEffort 格式
				if request.ReasoningEffort != "" {
					reasoning := map[string]any{
						"enabled": true,
					}
					if request.ReasoningEffort != "none" {
						reasoning["effort"] = request.ReasoningEffort
						marshal, err := common.Marshal(reasoning)
						if err != nil {
							return nil, fmt.Errorf("error marshalling reasoning: %w", err)
						}
						request.Reasoning = marshal
					}
				}
			}
			request.ReasoningEffort = ""
		}

		// https://docs.anthropic.com/en/api/openai-sdk#extended-thinking-support
		// 没有做排除3.5Haiku等，要出问题再加吧，最佳兼容性（不是
		if request.THINKING != nil && strings.HasPrefix(info.UpstreamModelName, "anthropic") {
			var thinking dto.Thinking // Claude标准Thinking格式
			if err := json.Unmarshal(request.THINKING, &thinking); err != nil {
				return nil, fmt.Errorf("error Unmarshal thinking: %w", err)
			}

			// 只有当 thinking.Type 是 "enabled" 时才处理
			if thinking.Type == "enabled" {
				// 检查 BudgetTokens 是否为 nil
				if thinking.BudgetTokens == nil {
					return nil, fmt.Errorf("BudgetTokens is nil when thinking is enabled")
				}

				reasoning := openrouter.RequestReasoning{
					MaxTokens: *thinking.BudgetTokens,
				}

				marshal, err := common.Marshal(reasoning)
				if err != nil {
					return nil, fmt.Errorf("error marshalling reasoning: %w", err)
				}

				request.Reasoning = marshal
			}

			// 清空 THINKING
			request.THINKING = nil
		}

	}
	// 处理模型名，去掉 openai/ 前缀（如果存在）
	modelName := info.UpstreamModelName
	if strings.HasPrefix(modelName, "openai/") {
		modelName = strings.TrimPrefix(modelName, "openai/")
		// 更新 UpstreamModelName 和 request.Model，确保发送到上游的模型名不包含 openai/ 前缀
		info.UpstreamModelName = modelName
		request.Model = modelName
	}

	if strings.HasPrefix(modelName, "o") || strings.HasPrefix(modelName, "gpt-5") {
		// 部分上游（如 gpt-5.2）只支持 max_completion_tokens，不支持 max_tokens，需统一用 max_completion_tokens
		if request.MaxCompletionTokens == 0 && request.MaxTokens != 0 {
			request.MaxCompletionTokens = request.MaxTokens
			request.MaxTokens = 0
		}
		if request.MaxCompletionTokens == 0 {
			request.MaxCompletionTokens = 4096 // 两者都未传时给默认值，避免上游 400
		}

		// 转换模型推理力度后缀
		effort, originModel := parseReasoningEffortFromModelSuffix(modelName)
		if effort != "" {
			request.ReasoningEffort = effort
			modelName = originModel
			info.UpstreamModelName = originModel
			request.Model = originModel
		}

		info.ReasoningEffort = request.ReasoningEffort

		// o系列模型developer适配（o1-mini除外）
		if !strings.HasPrefix(modelName, "o1-mini") && !strings.HasPrefix(modelName, "o1-preview") {
			//修改第一个Message的内容，将system改为developer
			if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
				request.Messages[0].Role = "developer"
			}
		}
	}

	// 过滤掉 prompt_cache_retention 参数，因为上游不支持
	// 注意：设置为空字符串，但由于 JSON 的 omitempty 标签，空字符串可能仍会被序列化
	// 更好的方法是在序列化后从 JSON 中删除，但这需要在 responses_handler.go 中处理
	request.PromptCacheRetention = ""

	// 过滤掉 reasoning 参数（OpenRouter 除外，因为它需要这个字段）
	// 对于标准的 OpenAI Chat Completions API，不支持 reasoning 字段，只支持 reasoning_effort
	if info.ChannelType != constant.ChannelTypeOpenRouter {
		request.Reasoning = nil
	}

	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	a.ResponseFormat = request.ResponseFormat
	if info.RelayMode == relayconstant.RelayModeAudioSpeech {
		jsonData, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("error marshalling object: %w", err)
		}
		return bytes.NewReader(jsonData), nil
	} else {
		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)

		// 获取所有表单字段
		formData := c.Request.PostForm

		// 遍历表单字段并打印输出
		for key, values := range formData {
			if key == "model" {
				continue
			}
			for _, value := range values {
				writer.WriteField(key, value)
			}
		}

		// 添加文件字段
		file, header, err := c.Request.FormFile("file")
		if err != nil {
			return nil, errors.New("file is required")
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", header.Filename)
		if err != nil {
			return nil, errors.New("create form file failed")
		}
		if _, err := io.Copy(part, file); err != nil {
			return nil, errors.New("copy file failed")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &requestBody, nil
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// gpt-image-1 模型：使用白名单过滤，只保留支持的参数
	if strings.HasPrefix(request.Model, "gpt-image-1") {
		// 清空不支持的字段
		request.ResponseFormat = ""
		request.Style = nil
		request.ExtraFields = nil
		request.Background = nil
		request.Moderation = nil
		request.OutputFormat = nil
		request.OutputCompression = nil
		request.PartialImages = nil
		request.Stream = nil
		request.Watermark = nil
		request.User = nil

		// 只保留 gpt-image-1 支持的参数
		// 支持的参数：model, prompt, n, size, quality, input_fidelity(仅图生图), image/images
		if request.Extra != nil {
			allowedParams := map[string]bool{
				"model":          true,
				"prompt":         true,
				"n":              true,
				"size":           true,
				"quality":        true,
				"input_fidelity": true,
				"image":          true, // 图生图单图
				"images":         true, // 图生图多图
			}

			// 删除所有不在白名单中的参数
			for key := range request.Extra {
				if !allowedParams[key] {
					delete(request.Extra, key)
				}
			}
		}
	} else {
		// 非 gpt-image-1 模型：只删除内部参数
		if request.Extra != nil {
			delete(request.Extra, "user_id")
		}
	}

	switch info.RelayMode {
	case relayconstant.RelayModeImagesEdits:
		// 图生图必须使用 multipart/form-data 格式（Azure OpenAI 要求）

		// 调试日志：简化输出
		if common.DebugEnabled {
			logger.LogDebug(c, fmt.Sprintf("[ConvertImageRequest] Model=%s, Prompt=%s, Size=%s, Quality=%s, N=%d, InputFidelity=%s",
				request.Model, request.Prompt, request.Size, request.Quality, request.N, request.InputFidelity))
		}

		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)
		writer.WriteField("prompt", request.Prompt)

		// 添加可选参数
		if request.Size != "" {
			writer.WriteField("size", request.Size)
		}
		if request.Quality != "" {
			writer.WriteField("quality", request.Quality)
		}
		if request.N > 0 {
			writer.WriteField("n", fmt.Sprintf("%d", request.N))
		}
		if request.InputFidelity != "" {
			writer.WriteField("input_fidelity", request.InputFidelity)
		}

		// 检测请求格式：是 JSON 还是 multipart
		contentType := c.GetHeader("Content-Type")
		isJSON := strings.Contains(contentType, "application/json")

		if isJSON {
			// JSON 格式：从 Extra 中提取 image 或 images 字段
			if request.Extra != nil {
				var imageStrings []string
				var imageCount int

				// 检查是单图还是多图
				if imageData, ok := request.Extra["image"]; ok {
					var imageStr string
					if err := json.Unmarshal(imageData, &imageStr); err == nil {
						imageStrings = append(imageStrings, imageStr)
						imageCount = 1
					}
				} else if imagesData, ok := request.Extra["images"]; ok {
					var images []string
					if err := json.Unmarshal(imagesData, &images); err == nil {
						imageStrings = images
						imageCount = len(images)
					}
				}

				if common.DebugEnabled && imageCount > 0 {
					logger.LogDebug(c, fmt.Sprintf("[ConvertImageRequest] 输入图片: %d张", imageCount))
				}

				// 处理所有图片
				if len(imageStrings) > 0 {
					for i, imageStr := range imageStrings {
						// 下载或解码图片
						imageBytes, mimeType, err := downloadOrDecodeImage(imageStr)
						if err != nil {
							return nil, fmt.Errorf("failed to process image %d: %w", i, err)
						}

						// 根据 MIME 类型确定文件名
						filename := fmt.Sprintf("image%d.png", i)
						if mimeType == "image/jpeg" {
							filename = fmt.Sprintf("image%d.jpg", i)
						} else if mimeType == "image/webp" {
							filename = fmt.Sprintf("image%d.webp", i)
						}

						// 确定字段名：根据官方文档，GPT-image-1 系列统一使用 "image[]"（即使单图）
						// 官方文档示例：-F "image[]=@beach.png"（单图也使用 image[]）
						fieldName := "image[]"

						// 添加图片文件到 multipart（使用正确的 MIME 类型）
						h := make(textproto.MIMEHeader)
						h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, filename))
						h.Set("Content-Type", mimeType)

						part, err := writer.CreatePart(h)
						if err != nil {
							return nil, fmt.Errorf("failed to create form file %d: %w", i, err)
						}
						if _, err := part.Write(imageBytes); err != nil {
							return nil, fmt.Errorf("failed to write image data %d: %w", i, err)
						}
					}
				} else {
					// 没有找到图片数据
					return nil, errors.New("image or images field is required for edits endpoint")
				}
			}
		} else {
			// multipart/form-data 格式：使用已解析的 multipart 表单
			mf := c.Request.MultipartForm
			if mf == nil {
				if _, err := c.MultipartForm(); err != nil {
					return nil, errors.New("failed to parse multipart form")
				}
				mf = c.Request.MultipartForm
			}

			if mf != nil && mf.File != nil {
				// Check if "image" field exists in any form, including array notation
				var imageFiles []*multipart.FileHeader
				var exists bool

				// First check for standard "image" field
				if imageFiles, exists = mf.File["image"]; !exists || len(imageFiles) == 0 {
					// If not found, check for "image[]" field
					if imageFiles, exists = mf.File["image[]"]; !exists || len(imageFiles) == 0 {
						// If still not found, iterate through all fields to find any that start with "image["
						foundArrayImages := false
						for fieldName, files := range mf.File {
							if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
								foundArrayImages = true
								imageFiles = append(imageFiles, files...)
							}
						}

						// If no image fields found at all
						if !foundArrayImages && (len(imageFiles) == 0) {
							return nil, errors.New("image is required")
						}
					}
				}

				// Process all image files
				for i, fileHeader := range imageFiles {
					file, err := fileHeader.Open()
					if err != nil {
						return nil, fmt.Errorf("failed to open image file %d: %w", i, err)
					}

					// 根据官方文档，GPT-image-1 系列统一使用 "image[]"（即使单图）
					// 官方文档示例：-F "image[]=@beach.png"（单图也使用 image[]）
					fieldName := "image[]"

					// Determine MIME type based on file extension
					mimeType := detectImageMimeType(fileHeader.Filename)

					// Create a form file with the appropriate content type
					h := make(textproto.MIMEHeader)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
					h.Set("Content-Type", mimeType)

					part, err := writer.CreatePart(h)
					if err != nil {
						return nil, fmt.Errorf("create form part failed for image %d: %w", i, err)
					}

					if _, err := io.Copy(part, file); err != nil {
						return nil, fmt.Errorf("copy file failed for image %d: %w", i, err)
					}

					// 复制完立即关闭，避免在循环内使用 defer 占用资源
					_ = file.Close()
				}

				// Handle mask file if present
				if maskFiles, exists := mf.File["mask"]; exists && len(maskFiles) > 0 {
					maskFile, err := maskFiles[0].Open()
					if err != nil {
						return nil, errors.New("failed to open mask file")
					}
					// 复制完立即关闭，避免在循环内使用 defer 占用资源

					// Determine MIME type for mask file
					mimeType := detectImageMimeType(maskFiles[0].Filename)

					// Create a form file with the appropriate content type
					h := make(textproto.MIMEHeader)
					h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="mask"; filename="%s"`, maskFiles[0].Filename))
					h.Set("Content-Type", mimeType)

					maskPart, err := writer.CreatePart(h)
					if err != nil {
						return nil, errors.New("create form file failed for mask")
					}

					if _, err := io.Copy(maskPart, maskFile); err != nil {
						return nil, errors.New("copy mask file failed")
					}
					_ = maskFile.Close()
				}
			} else {
				return nil, errors.New("no multipart form data found")
			}
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())

		// 调试日志：简化输出
		if common.DebugEnabled {
			logger.LogDebug(c, fmt.Sprintf("[ConvertImageRequest] Multipart构建完成, Size=%d bytes", requestBody.Len()))
		}

		return &requestBody, nil

	default:
		return request, nil
	}
}

// detectImageMimeType determines the MIME type based on the file extension
func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		// Try to detect from extension if possible
		if strings.HasPrefix(ext, ".jp") {
			return "image/jpeg"
		}
		// Default to png as a fallback
		return "image/png"
	}
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	//  转换模型推理力度后缀
	effort, originModel := parseReasoningEffortFromModelSuffix(request.Model)
	if effort != "" {
		if request.Reasoning == nil {
			request.Reasoning = &dto.Reasoning{
				Effort: effort,
			}
		} else {
			request.Reasoning.Effort = effort
		}
		request.Model = originModel
	}

	// 过滤掉 prompt_cache_retention 参数，因为上游不支持
	// 使用 map 方式确保字段被完全移除，而不是设置为空字符串
	requestJSON, err := common.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var requestMap map[string]interface{}
	if err := common.Unmarshal(requestJSON, &requestMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	// 删除 prompt_cache_retention 字段
	delete(requestMap, "prompt_cache_retention")

	return requestMap, nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	if info.RelayMode == relayconstant.RelayModeAudioTranscription ||
		info.RelayMode == relayconstant.RelayModeAudioTranslation ||
		info.RelayMode == relayconstant.RelayModeImagesEdits {
		return channel.DoFormRequest(a, c, info, requestBody)
	} else if info.RelayMode == relayconstant.RelayModeRealtime {
		return channel.DoWssRequest(a, c, info, requestBody)
	} else {
		return channel.DoApiRequest(a, c, info, requestBody)
	}
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayMode {
	case relayconstant.RelayModeRealtime:
		err, usage = OpenaiRealtimeHandler(c, info)
	case relayconstant.RelayModeAudioSpeech:
		usage = OpenaiTTSHandler(c, resp, info)
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err, usage = OpenaiSTTHandler(c, resp, info, a.ResponseFormat)
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		usage, err = OpenaiHandlerWithUsage(c, info, resp)
	case relayconstant.RelayModeRerank:
		usage, err = common_handler.RerankHandler(c, info, resp)
	case relayconstant.RelayModeResponses:
		if info.IsStream {
			usage, err = OaiResponsesStreamHandler(c, info, resp)
		} else {
			usage, err = OaiResponsesHandler(c, info, resp)
		}
	default:
		if info.IsStream {
			usage, err = OaiStreamHandler(c, info, resp)
		} else {
			usage, err = OpenaiHandler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	switch a.ChannelType {
	case constant.ChannelType360:
		return ai360.ModelList
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ModelList
	case constant.ChannelTypeMiniMax:
		return minimax.ModelList
	case constant.ChannelTypeXinference:
		return xinference.ModelList
	case constant.ChannelTypeOpenRouter:
		return openrouter.ModelList
	default:
		return ModelList
	}
}

func (a *Adaptor) GetChannelName() string {
	switch a.ChannelType {
	case constant.ChannelType360:
		return ai360.ChannelName
	case constant.ChannelTypeLingYiWanWu:
		return lingyiwanwu.ChannelName
	case constant.ChannelTypeMiniMax:
		return minimax.ChannelName
	case constant.ChannelTypeXinference:
		return xinference.ChannelName
	case constant.ChannelTypeOpenRouter:
		return openrouter.ChannelName
	default:
		return ChannelName
	}
}

// downloadOrDecodeImage 下载图片URL或解码base64图片
// 返回：图片字节、MIME类型、错误
func downloadOrDecodeImage(imageData string) ([]byte, string, error) {
	if strings.HasPrefix(imageData, "data:image") {
		// Base64 解码
		parts := strings.SplitN(imageData, ",", 2)
		if len(parts) != 2 {
			return nil, "", errors.New("invalid base64 image format")
		}

		// 提取 MIME 类型（例如：data:image/png;base64,xxx）
		mimeType := "image/png" // 默认
		if strings.Contains(parts[0], "image/jpeg") || strings.Contains(parts[0], "image/jpg") {
			mimeType = "image/jpeg"
		} else if strings.Contains(parts[0], "image/png") {
			mimeType = "image/png"
		} else if strings.Contains(parts[0], "image/webp") {
			mimeType = "image/webp"
		}

		imageBytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, "", fmt.Errorf("failed to decode base64: %w", err)
		}

		return imageBytes, mimeType, nil
	} else if strings.HasPrefix(imageData, "http://") || strings.HasPrefix(imageData, "https://") {
		// URL 下载
		resp, err := http.Get(imageData)
		if err != nil {
			return nil, "", fmt.Errorf("failed to download image: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("failed to download image: status %d", resp.StatusCode)
		}

		// 从 Content-Type 获取 MIME 类型
		mimeType := resp.Header.Get("Content-Type")
		if mimeType == "" || mimeType == "application/octet-stream" {
			// 从 URL 推断类型
			mimeType = detectMimeTypeFromURL(imageData)
		}

		imageBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("failed to read image: %w", err)
		}

		return imageBytes, mimeType, nil
	}

	return nil, "", errors.New("image must be a URL or base64 data URI")
}

// detectMimeTypeFromURL 从 URL 推断 MIME 类型
func detectMimeTypeFromURL(url string) string {
	lower := strings.ToLower(url)
	if strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") {
		return "image/jpeg"
	} else if strings.Contains(lower, ".png") {
		return "image/png"
	} else if strings.Contains(lower, ".webp") {
		return "image/webp"
	}
	return "image/png" // 默认使用 PNG
}
