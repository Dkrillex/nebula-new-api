package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/dto"
	"one-api/relay/channel"
	"one-api/relay/channel/openai"
	relaycommon "one-api/relay/common"
	"one-api/relay/constant"
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
	if strings.Contains(info.UpstreamModelName, "gemini-2.5-flash-image") {
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
		responseModalities := []string{"TEXT", "IMAGE"}
		topP := 0.95

		if request.Extra != nil {
			// 获取temperature参数
			if tempData, exists := request.Extra["temperature"]; exists {
				var tempValue float64
				if err := json.Unmarshal(tempData, &tempValue); err == nil {
					temperature = tempValue
				}
			}

			// 获取max_output_tokens参数
			if maxTokensData, exists := request.Extra["max_output_tokens"]; exists {
				var maxTokensValue uint
				if err := json.Unmarshal(maxTokensData, &maxTokensValue); err == nil {
					maxOutputTokens = maxTokensValue
				}
			}

			// 获取response_modalities参数
			if modalitiesData, exists := request.Extra["response_modalities"]; exists {
				var modalitiesValue []string
				if err := json.Unmarshal(modalitiesData, &modalitiesValue); err == nil {
					responseModalities = modalitiesValue
				}
			}

			// 获取top_p参数
			if topPData, exists := request.Extra["top_p"]; exists {
				var topPValue float64
				if err := json.Unmarshal(topPData, &topPValue); err == nil {
					topP = topPValue
				}
			}
		}

		// 使用标准Gemini格式，支持多模态响应
		geminiRequest := dto.GeminiChatRequest{
			Contents: contents,
			GenerationConfig: dto.GeminiChatGenerationConfig{
				Temperature:        &temperature,
				MaxOutputTokens:    maxOutputTokens,
				ResponseModalities: responseModalities, // 关键：请求图像生成
				TopP:               topP,
			},
			SafetySettings: []dto.GeminiChatSafetySettings{
				{Category: "HARM_CATEGORY_HATE_SPEECH", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_DANGEROUS_CONTENT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_SEXUALLY_EXPLICIT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_HARASSMENT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_IMAGE_HATE", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_IMAGE_DANGEROUS_CONTENT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_IMAGE_HARASSMENT", Threshold: "OFF"},
				{Category: "HARM_CATEGORY_IMAGE_SEXUALLY_EXPLICIT", Threshold: "OFF"},
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
			case "1024x1024":
				aspectRatio = "1:1"
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
	req.Set("x-goog-api-key", info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

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
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	if info.RelayMode == constant.RelayModeGemini {
		if strings.HasSuffix(info.RequestURLPath, ":embedContent") ||
			strings.HasSuffix(info.RequestURLPath, ":batchEmbedContents") {
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
