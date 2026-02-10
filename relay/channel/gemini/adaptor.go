package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/relay/channel"
	"one-api/relay/channel/openai"
	relaycommon "one-api/relay/common"
	"one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	if len(request.Contents) > 0 {
		for i, content := range request.Contents {
			if i == 0 {
				if request.Contents[0].Role == "" {
					request.Contents[0].Role = "user"
				}
			}
			for _, part := range content.Parts {
				if part.FileData != nil {
					if part.FileData.MimeType == "" && strings.Contains(part.FileData.FileUri, "www.youtube.com") {
						part.FileData.MimeType = "video/webm"
					}
				}
			}
		}
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	adaptor := openai.Adaptor{}
	oaiReq, err := adaptor.ConvertClaudeRequest(c, info, req)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, oaiReq.(*dto.GeneralOpenAIRequest))
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// convertPartToGeminiPart 将通用的part数据转换为GeminiPart
func (a *Adaptor) convertPartToGeminiPart(c *gin.Context, partData interface{}) *dto.GeminiPart {
	partMap, ok := partData.(map[string]interface{})
	if !ok {
		return nil
	}

	// 处理文本part
	if text, exists := partMap["text"]; exists {
		if textStr, ok := text.(string); ok {
			return &dto.GeminiPart{Text: textStr}
		}
	}

	// 处理图像part（base64或URL）
	if imageData, exists := partMap["image"]; exists {
		if imageStr, ok := imageData.(string); ok {
			return a.convertImageDataToGeminiPart(c, imageStr)
		}
	}

	return nil
}

// convertImageDataToGeminiPart 将图像数据转换为GeminiPart
func (a *Adaptor) convertImageDataToGeminiPart(c *gin.Context, imageData string) *dto.GeminiPart {
	// 处理base64图像
	if strings.HasPrefix(imageData, "data:image/") {
		// 提取base64数据和MIME类型
		imageParts := strings.Split(imageData, ",")
		if len(imageParts) == 2 {
			mimeType := "image/jpeg" // 默认
			if strings.Contains(imageParts[0], "image/png") {
				mimeType = "image/png"
			} else if strings.Contains(imageParts[0], "image/webp") {
				mimeType = "image/webp"
			}
			return &dto.GeminiPart{
				InlineData: &dto.GeminiInlineData{
					MimeType: mimeType,
					Data:     imageParts[1],
				},
			}
		}
	} else if strings.HasPrefix(imageData, "http") {
		// 处理URL图像（下载并转换为base64）
		fileData, err := service.GetFileBase64FromUrl(c, imageData, "formatting image for Gemini")
		if err != nil {
			return nil // 跳过无法下载的图像
		}
		return &dto.GeminiPart{
			InlineData: &dto.GeminiInlineData{
				MimeType: fileData.MimeType,
				Data:     fileData.Base64Data,
			},
		}
	}
	return nil
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	// 支持gemini-2.5-flash-image系列模型
	if strings.Contains(info.UpstreamModelName, "gemini-2.5-flash-image") || strings.Contains(info.UpstreamModelName, "-image") {
		var contents []dto.GeminiChatContent

		// 检查是否有完整的对话上下文（contents格式）
		if request.Extra != nil {
			if contentsData, exists := request.Extra["contents"]; exists {
				// 解析完整的对话上下文
				var contextContents []map[string]interface{}
				if err := json.Unmarshal(contentsData, &contextContents); err == nil {
					for _, content := range contextContents {
						role, _ := content["role"].(string)
						partsData, _ := content["parts"].([]interface{})

						var parts []dto.GeminiPart
						for _, partData := range partsData {
							part := a.convertPartToGeminiPart(c, partData)
							if part != nil {
								parts = append(parts, *part)
							}
						}

						if len(parts) > 0 {
							contents = append(contents, dto.GeminiChatContent{
								Role:  role,
								Parts: parts,
							})
						}
					}
				}
			}
		}

		// 如果没有完整的对话上下文，使用传统方式构建单个用户消息
		if len(contents) == 0 {
			parts := []dto.GeminiPart{}

			// 添加文本提示词
			if request.Prompt != "" {
				parts = append(parts, dto.GeminiPart{
					Text: request.Prompt,
				})
			}

			// 检查Extra中的图像数据
			if request.Extra != nil {
				for key, value := range request.Extra {
					if strings.Contains(strings.ToLower(key), "image") {
						var imageData string
						if err := json.Unmarshal(value, &imageData); err == nil {
							part := a.convertImageDataToGeminiPart(c, imageData)
							if part != nil {
								parts = append(parts, *part)
							}
						}
					}
				}
			}

			contents = []dto.GeminiChatContent{
				{
					Role:  "user",
					Parts: parts,
				},
			}
		}

		// 从Extra中获取各种参数，如果没有则使用默认值
		temperature := 1.0
		maxOutputTokens := uint(32768)
		responseModalities := []string{"IMAGE"} // 默认只返回图片
		topP := 0.95
		aspectRatio := "1:1" // 默认宽高比
		imageSize := "1K"    // 默认图片尺寸

		// 辅助函数：从json.RawMessage中读取值
		readStringValue := func(data json.RawMessage) (string, bool) {
			var value string
			if err := json.Unmarshal(data, &value); err == nil {
				return value, true
			}
			return "", false
		}

		readFloatValue := func(data json.RawMessage) (float64, bool) {
			var value float64
			if err := json.Unmarshal(data, &value); err == nil {
				return value, true
			}
			return 0, false
		}

		readUintValue := func(data json.RawMessage) (uint, bool) {
			var value uint
			if err := json.Unmarshal(data, &value); err == nil {
				return value, true
			}
			// 尝试作为float64解析，然后转换
			var floatValue float64
			if err := json.Unmarshal(data, &floatValue); err == nil {
				return uint(floatValue), true
			}
			return 0, false
		}

		readStringArrayValue := func(data json.RawMessage) ([]string, bool) {
			var value []string
			if err := json.Unmarshal(data, &value); err == nil {
				return value, true
			}
			return nil, false
		}

		// 先尝试从嵌套的extra对象中读取参数
		var extraMap map[string]json.RawMessage
		if request.Extra != nil {
			if extraData, exists := request.Extra["extra"]; exists {
				// 解析嵌套的extra对象
				if err := json.Unmarshal(extraData, &extraMap); err == nil {
					// 从嵌套的extra对象中读取参数
					if value, ok := readStringArrayValue(extraMap["response_modalities"]); ok {
						responseModalities = value
					}
					if value, ok := readFloatValue(extraMap["temperature"]); ok {
						temperature = value
					}
					if value, ok := readUintValue(extraMap["max_output_tokens"]); ok {
						maxOutputTokens = value
					}
					if value, ok := readFloatValue(extraMap["top_p"]); ok {
						topP = value
					}
					if value, ok := readStringValue(extraMap["aspect_ratio"]); ok {
						aspectRatio = value
					}
					if value, ok := readStringValue(extraMap["image_size"]); ok {
						if value == "1K" || value == "2K" || value == "4K" {
							imageSize = value
						}
					}
				}
			}
		}

		// 如果Extra不为空，也尝试直接从Extra中读取（向后兼容，优先级高于嵌套的extra对象）
		if request.Extra != nil {
			// 获取response_modalities参数
			if value, ok := readStringArrayValue(request.Extra["response_modalities"]); ok {
				responseModalities = value
			}

			// 获取temperature参数
			if value, ok := readFloatValue(request.Extra["temperature"]); ok {
				temperature = value
			}

			// 获取max_output_tokens参数
			if value, ok := readUintValue(request.Extra["max_output_tokens"]); ok {
				maxOutputTokens = value
			}

			// 获取top_p参数
			if value, ok := readFloatValue(request.Extra["top_p"]); ok {
				topP = value
			}

			// 获取aspect_ratio参数
			if value, ok := readStringValue(request.Extra["aspect_ratio"]); ok {
				aspectRatio = value
			}

			// 获取image_size参数
			if value, ok := readStringValue(request.Extra["image_size"]); ok {
				if value == "1K" || value == "2K" || value == "4K" {
					imageSize = value
				}
			}
		}

		// 如果Extra中没有aspect_ratio，尝试从Size字段转换
		if aspectRatio == "1:1" && request.Size != "" {
			size := strings.TrimSpace(request.Size)
			if strings.Contains(size, ":") {
				// 直接使用比例格式
				aspectRatio = size
			} else {
				// 从像素尺寸转换为比例
				switch size {
				case "256x256", "512x512", "1024x1024":
					aspectRatio = "1:1"
				case "1536x1024":
					aspectRatio = "3:2"
				case "1024x1536":
					aspectRatio = "2:3"
				case "1536x2048":
					aspectRatio = "3:4"
				case "2048x1536":
					aspectRatio = "4:3"
				case "1024x1280":
					aspectRatio = "4:5"
				case "1280x1024":
					aspectRatio = "5:4"
				case "1024x1792":
					aspectRatio = "9:16"
				case "1792x1024":
					aspectRatio = "16:9"
				case "1024x2176":
					aspectRatio = "21:9"
				}
			}
		}

		// 构建ImageConfig，包含aspectRatio、imageSize
		imageConfig := map[string]interface{}{
			"aspectRatio": aspectRatio,
			"imageSize":   imageSize,
		}
		imageConfigJSON, _ := json.Marshal(imageConfig)

		// 使用标准Gemini格式，支持多模态响应
		geminiRequest := dto.GeminiChatRequest{
			Contents: contents,
			GenerationConfig: dto.GeminiChatGenerationConfig{
				Temperature:        &temperature,
				MaxOutputTokens:    maxOutputTokens,
				ResponseModalities: responseModalities, // 关键：请求图像生成
				TopP:               topP,
				ImageConfig:        imageConfigJSON, // 设置图像配置，包含宽高比
			},
			SafetySettings: []dto.GeminiChatSafetySettings{
				{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
				//{Category: "HARM_CATEGORY_IMAGE_HATE", Threshold: "OFF"},
				//{Category: "HARM_CATEGORY_IMAGE_DANGEROUS_CONTENT", Threshold: "OFF"},
				//{Category: "HARM_CATEGORY_IMAGE_HARASSMENT", Threshold: "OFF"},
				//{Category: "HARM_CATEGORY_IMAGE_SEXUALLY_EXPLICIT", Threshold: "OFF"},
			},
		}
		return geminiRequest, nil
	}

	if !strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return nil, errors.New("not supported model for image generation")
	}

	// convert size to aspect ratio but allow user to specify aspect ratio
	aspectRatio := "1:1" // default aspect ratio
	size := strings.TrimSpace(request.Size)
	if size != "" {
		if strings.Contains(size, ":") {
			aspectRatio = size
		} else {
			switch size {
			case "256x256", "512x512", "1024x1024":
				aspectRatio = "1:1"
			case "1536x1024":
				aspectRatio = "3:2"
			case "1024x1536":
				aspectRatio = "2:3"
			case "1024x1792":
				aspectRatio = "9:16"
			case "1792x1024":
				aspectRatio = "16:9"
			}
		}
	}

	// build gemini imagen request
	geminiRequest := dto.GeminiImageRequest{
		Instances: []dto.GeminiImageInstance{
			{
				Prompt: request.Prompt,
			},
		},
		Parameters: dto.GeminiImageParameters{
			SampleCount:      int(request.N),
			AspectRatio:      aspectRatio,
			PersonGeneration: "allow_adult", // default allow adult
		},
	}

	// Set imageSize when quality parameter is specified
	// Map quality parameter to imageSize (only supported by Standard and Ultra models)
	// quality values: auto, high, medium, low (for gpt-image-1), hd, standard (for dall-e-3)
	// imageSize values: 1K (default), 2K
	// https://ai.google.dev/gemini-api/docs/imagen
	// https://platform.openai.com/docs/api-reference/images/create
	if request.Quality != "" {
		imageSize := "1K" // default
		switch request.Quality {
		case "hd", "high":
			imageSize = "2K"
		case "2K":
			imageSize = "2K"
		case "standard", "medium", "low", "auto", "1K":
			imageSize = "1K"
		default:
			// unknown quality value, default to 1K
			imageSize = "1K"
		}
		geminiRequest.Parameters.ImageSize = imageSize
	}

	return geminiRequest, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {

}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled {
		// 新增逻辑：处理 -thinking-<budget> 格式
		if strings.Contains(info.UpstreamModelName, "-thinking-") {
			parts := strings.Split(info.UpstreamModelName, "-thinking-")
			info.UpstreamModelName = parts[0]
		} else if strings.HasSuffix(info.UpstreamModelName, "-thinking") { // 旧的适配
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		} else if strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-nothinking")
		}
	}

	// 检查是否为 Gemini Live API 模型
	if IsGeminiLiveModel(info.UpstreamModelName) {
		// 转换为 WebSocket URL
		baseURL := strings.TrimSpace(info.ChannelBaseUrl)

		// If baseURL is empty, use default endpoint based on API type
		if baseURL == "" || baseURL == "/" {
			// Default to Google AI Studio endpoint
			baseURL = "https://generativelanguage.googleapis.com"
		}

		// Ensure baseURL has a scheme
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") && !strings.HasPrefix(baseURL, "ws://") && !strings.HasPrefix(baseURL, "wss://") {
			// Default to https if no scheme
			baseURL = "https://" + baseURL
		}

		if strings.HasPrefix(baseURL, "https://") {
			baseURL = "wss://" + strings.TrimPrefix(baseURL, "https://")
		} else if strings.HasPrefix(baseURL, "http://") {
			baseURL = "ws://" + strings.TrimPrefix(baseURL, "http://")
		}

		// Gemini Live API WebSocket 端点
		// 格式: wss://{region}-aiplatform.googleapis.com/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent
		// 或: wss://generativelanguage.googleapis.com/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent
		var wsURL string
		if strings.Contains(baseURL, "generativelanguage.googleapis.com") {
			// Google AI Studio: 使用 API Key 查询参数
			wsURL = fmt.Sprintf("%s/ws/google.ai.generativelanguage.v1beta.GenerativeService.BidiGenerateContent?key=%s", baseURL, info.ApiKey)
		} else {
			// Vertex AI 格式: 使用 OAuth2 token（在 header 中设置）
			wsURL = fmt.Sprintf("%s/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent", baseURL)
		}
		return wsURL, nil
	}

	version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return fmt.Sprintf("%s/%s/models/%s:predict", info.ChannelBaseUrl, version, info.UpstreamModelName), nil
	}

	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		action := "embedContent"
		if info.IsGeminiBatchEmbedding {
			action = "batchEmbedContents"
		}
		return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
	}

	// 使用 Google 官方 OpenAI 兼容端点（协议适配器）：仅当请求来自 OpenAI 风格路径时使用，请求体保持 OpenAI 格式
	useCompat := model_setting.GetGeminiSettings().UseOpenAICompatibleEndpoint
	common.SysLog(fmt.Sprintf("[Gemini] GetRequestURL: UseOpenAICompatibleEndpoint=%v, ChannelBaseUrl=%s", useCompat, info.ChannelBaseUrl))
	if useCompat && info.RelayMode == constant.RelayModeChatCompletions {
		info.UseGeminiOpenAICompatibleEndpoint = true
		baseURL := strings.TrimSuffix(strings.TrimSpace(info.ChannelBaseUrl), "/")
		if strings.Contains(baseURL, "aiplatform.googleapis.com") {
			// Vertex AI: .../v1/projects/{project}/locations/{location}/publishers/google -> .../locations/{location}/endpoints/openapi/chat/completions
			const publishersGoogle = "/publishers/google"
			if idx := strings.LastIndex(baseURL, publishersGoogle); idx != -1 {
				baseURL = baseURL[:idx] + "/endpoints/openapi/chat/completions"
				common.SysLog(fmt.Sprintf("[Gemini] 使用 OpenAI 兼容端点 URL: %s", baseURL))
				return baseURL, nil
			}
		}
		if strings.Contains(baseURL, "generativelanguage.googleapis.com") {
			// Gemini API (Google AI Studio)
			if !strings.HasPrefix(baseURL, "http") {
				baseURL = "https://" + baseURL
			}
			url := fmt.Sprintf("%s/v1beta/openai/chat/completions", strings.TrimSuffix(baseURL, "/"))
			common.SysLog(fmt.Sprintf("[Gemini] 使用 OpenAI 兼容端点 URL: %s", url))
			return url, nil
		}
		// 无法推导 openapi base 时不启用兼容端点
		common.SysLog("[Gemini] 无法从 ChannelBaseUrl 推导 OpenAI 兼容端点，回退到原生端点")
		info.UseGeminiOpenAICompatibleEndpoint = false
	}

	action := "generateContent"
	if info.IsStream {
		action = "streamGenerateContent?alt=sse"
		if info.RelayMode == constant.RelayModeGemini {
			info.DisablePing = true
		}
	}
	return fmt.Sprintf("%s/%s/models/%s:%s", info.ChannelBaseUrl, version, info.UpstreamModelName, action), nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)

	// Gemini Live API 使用 WebSocket，需要特殊处理
	if IsGeminiLiveModel(info.UpstreamModelName) {
		// Google AI Studio 使用 API Key 查询参数，Vertex AI 使用 OAuth2
		// 对于 WebSocket，我们可以在 URL 中添加 key 参数，或者在 header 中设置
		// 这里先设置 header，如果 URL 中已经有 key 参数则不需要
		if !strings.Contains(info.ChannelBaseUrl, "generativelanguage.googleapis.com") {
			// Vertex AI 可能需要 OAuth2 token
			req.Set("Authorization", "Bearer "+info.ApiKey)
		}
		// 注意：Google AI Studio 的 API Key 会在 URL 查询参数中传递（在 GetRequestURL 中处理）
	} else {
		// Vertex OpenAI 兼容端点仅支持 Bearer；Gemini API 兼容端点可用 x-goog-api-key 或 Bearer
		if info.UseGeminiOpenAICompatibleEndpoint && strings.Contains(info.ChannelBaseUrl, "aiplatform.googleapis.com") {
			req.Set("Authorization", "Bearer "+info.ApiKey)
		} else {
			req.Set("x-goog-api-key", info.ApiKey)
		}
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// 使用 Google 官方 OpenAI 兼容端点：透传 OpenAI 格式，仅做 model 映射为 google/xxx
	if model_setting.GetGeminiSettings().UseOpenAICompatibleEndpoint {
		pass := *request
		pass.Model = "google/" + info.UpstreamModelName
		helper.EnsureGeminiThoughtSignaturesForOpenAIRequest(&pass)
		common.SysLog(fmt.Sprintf("[Gemini] ConvertOpenAIRequest: 使用兼容端点，透传 OpenAI 格式，model=%s", pass.Model))
		return &pass, nil
	}

	common.SysLog(fmt.Sprintf("[Gemini] ConvertOpenAIRequest: 使用原生 Gemini 格式转换，model=%s", info.UpstreamModelName))
	geminiRequest, err := CovertGemini2OpenAI(c, *request, info)
	if err != nil {
		return nil, err
	}

	return geminiRequest, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	if request.Input == nil {
		return nil, errors.New("input is required")
	}

	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	// We always build a batch-style payload with `requests`, so ensure we call the
	// batch endpoint upstream to avoid payload/endpoint mismatches.
	info.IsGeminiBatchEmbedding = true
	// process all inputs
	geminiRequests := make([]map[string]interface{}, 0, len(inputs))
	for _, input := range inputs {
		geminiRequest := map[string]interface{}{
			"model": fmt.Sprintf("models/%s", info.UpstreamModelName),
			"content": dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{
						Text: input,
					},
				},
			},
		}

		// set specific parameters for different models
		// https://ai.google.dev/api/embeddings?hl=zh-cn#method:-models.embedcontent
		switch info.UpstreamModelName {
		case "text-embedding-004", "gemini-embedding-exp-03-07", "gemini-embedding-001":
			// Only newer models introduced after 2024 support OutputDimensionality
			if request.Dimensions > 0 {
				geminiRequest["outputDimensionality"] = request.Dimensions
			}
		}
		geminiRequests = append(geminiRequests, geminiRequest)
	}

	return map[string]interface{}{
		"requests": geminiRequests,
	}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// 检查是否为 Gemini Live API (WebSocket)
	if IsGeminiLiveModel(info.UpstreamModelName) {
		return channel.DoWssRequest(a, c, info, requestBody)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// 检查是否为 Gemini Live API (WebSocket)
	if IsGeminiLiveModel(info.UpstreamModelName) && info.TargetWs != nil {
		newErr, realtimeUsage := GeminiLiveHandler(c, info)
		return realtimeUsage, newErr
	}

	// 使用 Google OpenAI 兼容端点时，响应已是 OpenAI 格式，直接走 OpenAI 解析
	if info.UseGeminiOpenAICompatibleEndpoint {
		if info.IsStream {
			return openai.OaiStreamHandler(c, info, resp)
		}
		return openai.OpenaiHandler(c, info, resp)
	}

	if info.RelayMode == constant.RelayModeGemini {
		if strings.Contains(info.RequestURLPath, ":embedContent") ||
			strings.Contains(info.RequestURLPath, ":batchEmbedContents") {
			return NativeGeminiEmbeddingHandler(c, resp, info)
		}
		if info.IsStream {
			return GeminiTextGenerationStreamHandler(c, info, resp)
		} else {
			return GeminiTextGenerationHandler(c, info, resp)
		}
	}

	if strings.HasPrefix(info.UpstreamModelName, "imagen") {
		return GeminiImageHandler(c, info, resp)
	}

	// check if the model is an embedding model
	if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "embedding") ||
		strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
		return GeminiEmbeddingHandler(c, info, resp)
	}

	if info.IsStream {
		return GeminiChatStreamHandler(c, info, resp)
	} else {
		return GeminiChatHandler(c, info, resp)
	}

}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
