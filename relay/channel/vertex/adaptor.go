package vertex

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/relay/channel"
	"one-api/relay/channel/claude"
	"one-api/relay/channel/gemini"
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

const (
	RequestModeClaude = 1
	RequestModeGemini = 2
	RequestModeLlama  = 3
)

var claudeModelMap = map[string]string{
	"claude-3-sonnet-20240229":   "claude-3-sonnet@20240229",
	"claude-3-opus-20240229":     "claude-3-opus@20240229",
	"claude-3-haiku-20240307":    "claude-3-haiku@20240307",
	"claude-3-5-sonnet-20240620": "claude-3-5-sonnet@20240620",
	"claude-3-5-sonnet-20241022": "claude-3-5-sonnet-v2@20241022",
	"claude-3-7-sonnet-20250219": "claude-3-7-sonnet@20250219",
	"claude-sonnet-4-20250514":   "claude-sonnet-4@20250514",
	"claude-opus-4-20250514":     "claude-opus-4@20250514",
	"claude-opus-4-1-20250805":   "claude-opus-4-1@20250805",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-5@20250929",
	"claude-haiku-4-5-20251001":  "claude-haiku-4-5@20251001",
}

const anthropicVersion = "vertex-2023-10-16"

type Adaptor struct {
	RequestMode        int
	AccountCredentials Credentials
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	geminiAdaptor := gemini.Adaptor{}
	return geminiAdaptor.ConvertGeminiRequest(c, info, request)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error) {
	if v, ok := claudeModelMap[info.UpstreamModelName]; ok {
		c.Set("request_model", v)
	} else {
		c.Set("request_model", request.Model)
	}
	vertexClaudeReq := copyRequest(request, anthropicVersion)
	return vertexClaudeReq, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	geminiAdaptor := gemini.Adaptor{}
	return geminiAdaptor.ConvertImageRequest(c, info, request)
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
	if strings.HasPrefix(info.UpstreamModelName, "claude") {
		a.RequestMode = RequestModeClaude
	} else if strings.Contains(info.UpstreamModelName, "llama") {
		a.RequestMode = RequestModeLlama
	} else {
		a.RequestMode = RequestModeGemini
	}
}

func (a *Adaptor) getRequestUrl(info *relaycommon.RelayInfo, modelName, suffix string) (string, error) {
	region := GetModelRegion(info.ApiVersion, info.OriginModelName)
	if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
		adc := &Credentials{}
		if err := common.Unmarshal([]byte(info.ApiKey), adc); err != nil {
			return "", fmt.Errorf("failed to decode credentials file: %w", err)
		}
		a.AccountCredentials = *adc

		if a.RequestMode == RequestModeGemini {
			if region == "global" {
				return fmt.Sprintf(
					"https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/google/models/%s:%s",
					adc.ProjectID,
					modelName,
					suffix,
				), nil
			} else {
				return fmt.Sprintf(
					"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models/%s:%s",
					region,
					adc.ProjectID,
					region,
					modelName,
					suffix,
				), nil
			}
		} else if a.RequestMode == RequestModeClaude {
			if region == "global" {
				return fmt.Sprintf(
					"https://aiplatform.googleapis.com/v1/projects/%s/locations/global/publishers/anthropic/models/%s:%s",
					adc.ProjectID,
					modelName,
					suffix,
				), nil
			} else {
				return fmt.Sprintf(
					"https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/anthropic/models/%s:%s",
					region,
					adc.ProjectID,
					region,
					modelName,
					suffix,
				), nil
			}
		} else if a.RequestMode == RequestModeLlama {
			return fmt.Sprintf(
				"https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/endpoints/openapi/chat/completions",
				region,
				adc.ProjectID,
				region,
			), nil
		}
	} else {
		var keyPrefix string
		if strings.HasSuffix(suffix, "?alt=sse") {
			keyPrefix = "&"
		} else {
			keyPrefix = "?"
		}
		if region == "global" {
			return fmt.Sprintf(
				"https://aiplatform.googleapis.com/v1/publishers/google/models/%s:%s%skey=%s",
				modelName,
				suffix,
				keyPrefix,
				info.ApiKey,
			), nil
		} else {
			return fmt.Sprintf(
				"https://%s-aiplatform.googleapis.com/v1/publishers/google/models/%s:%s%skey=%s",
				region,
				modelName,
				suffix,
				keyPrefix,
				info.ApiKey,
			), nil
		}
	}
	return "", errors.New("unsupported request mode")
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	suffix := ""
	if a.RequestMode == RequestModeGemini {
		// 检查是否需要使用 Google AI Studio 端点以支持思考内容返回
		// Vertex AI 端点可能不支持返回思考内容，所以当需要思考内容时，切换到 Google AI Studio 端点
		// 检查方式：
		// 1. 如果 ChannelBaseUrl 已经指向 Google AI Studio，直接使用
		// 2. 如果 ChannelBaseUrl 为空或指向 Vertex AI，且模型是 Gemini 3.1 系列，切换到 Google AI Studio 端点
		baseURL := strings.TrimSpace(info.ChannelBaseUrl)
		useGoogleAIStudio := false

		// 如果已经指向 Google AI Studio，直接使用（但需要检查是否支持）
		if strings.Contains(baseURL, "generativelanguage.googleapis.com") {
			// Google AI Studio 端点只支持 API Key，不支持 Service Account
			if info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey {
				useGoogleAIStudio = true
				common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s ChannelBaseUrl 已指向 Google AI Studio 端点，继续使用", info.UpstreamModelName))
			} else {
				common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s ChannelBaseUrl 指向 Google AI Studio 端点，但使用 Service Account，不支持。将回退到 Vertex AI 端点", info.UpstreamModelName))
			}
		} else if baseURL == "" || baseURL == "/" || strings.Contains(baseURL, "aiplatform.googleapis.com") {
			// 检查模型是否为 Gemini 3.1 系列（支持 thinking_level）
			// 如果模型是 Gemini 3.1 系列，且 ChannelBaseUrl 为空或指向 Vertex AI，且使用 API Key，使用 Google AI Studio 端点
			// 这样可以确保思考内容能够正确返回
			// 注意：Google AI Studio 端点只支持 API Key，不支持 Service Account
			if strings.HasPrefix(info.UpstreamModelName, "gemini-3.1-") && info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey {
				useGoogleAIStudio = true
				common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s 是 Gemini 3.1 系列，且 ChannelBaseUrl 为空或指向 Vertex AI，且使用 API Key，切换到 Google AI Studio 端点以支持思考内容返回", info.UpstreamModelName))
			} else if strings.HasPrefix(info.UpstreamModelName, "gemini-3.1-") && info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
				common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s 是 Gemini 3.1 系列，但使用 Service Account，无法使用 Google AI Studio 端点。将使用 Vertex AI 端点（可能不支持返回思考内容）", info.UpstreamModelName))
			}
		}

		if useGoogleAIStudio {
			// Google AI Studio 端点只支持 API Key，不支持 Service Account
			if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
				return "", fmt.Errorf("Google AI Studio 端点不支持 Service Account，只支持 API Key。请将 Channel 的 Key Type 设置为 API Key，或使用 Vertex AI 端点（但可能不支持返回思考内容）")
			}
			// 使用 Google AI Studio 端点格式
			version := model_setting.GetGeminiVersionSetting(info.UpstreamModelName)
			if info.IsStream {
				suffix = "streamGenerateContent?alt=sse"
			} else {
				suffix = "generateContent"
			}
			url := fmt.Sprintf("https://generativelanguage.googleapis.com/%s/models/%s:%s", version, info.UpstreamModelName, suffix)
			// 设置 ChannelBaseUrl 为 Google AI Studio 端点，以便 SetupRequestHeader 使用正确的认证方式
			info.ChannelBaseUrl = "https://generativelanguage.googleapis.com"
			common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s 使用 Google AI Studio 端点 URL: %s", info.UpstreamModelName, url))
			return url, nil
		}
		// 使用 Google 官方 OpenAI 兼容端点（仅 Service Account 有 project，可构建 URL）
		// 图片生成（imagen、*-image、*-image-preview）走原生端点，请求体为 contents/generationConfig，与 chat/completions 的 messages 格式不兼容
		useCompat := model_setting.GetGeminiSettings().UseOpenAICompatibleEndpoint
		isImageModel := strings.HasPrefix(info.UpstreamModelName, "imagen") || strings.Contains(info.UpstreamModelName, "-image")
		common.SysLog(fmt.Sprintf("[Vertex][Gemini] GetRequestURL: model=%s, UseOpenAICompatibleEndpoint=%v, isImageModel=%v", info.UpstreamModelName, useCompat, isImageModel))
		if useCompat && info.RelayMode == constant.RelayModeChatCompletions && !gemini.IsGeminiLiveModel(info.UpstreamModelName) && !isImageModel &&
			info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
			adc := &Credentials{}
			if err := common.Unmarshal([]byte(info.ApiKey), adc); err == nil && adc.ProjectID != "" {
				region := GetModelRegion(info.ApiVersion, info.OriginModelName)
				var baseURL string
				if region == "global" {
					baseURL = fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/global/endpoints/openapi/chat/completions", adc.ProjectID)
				} else {
					baseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/endpoints/openapi/chat/completions", region, adc.ProjectID, region)
				}
				info.UseGeminiOpenAICompatibleEndpoint = true
				common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s 使用 OpenAI 兼容端点 URL: %s", info.UpstreamModelName, baseURL))
				return baseURL, nil
			}
			if useCompat && info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey {
				common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s: API Key 模式无 project，无法使用 OpenAI 兼容端点，回退到原生端点", info.UpstreamModelName))
			}
		}

		// Check if this is a Gemini Live API request (WebSocket)
		if gemini.IsGeminiLiveModel(info.UpstreamModelName) {
			// H15: WebSocket URL 应该包含项目和区域信息，类似于 REST API
			// 从 Service Account JSON 中提取 project_id
			if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
				adc := &Credentials{}
				if err := common.Unmarshal([]byte(info.ApiKey), adc); err != nil {
					return "", fmt.Errorf("failed to decode credentials file: %w", err)
				}
				a.AccountCredentials = *adc
			}

			projectID := a.AccountCredentials.ProjectID
			if projectID == "" {
				return "", fmt.Errorf("project_id not found in service account credentials")
			}

			// 使用 us-central1 作为默认区域
			location := "us-central1"

			baseURL := strings.TrimSpace(info.ChannelBaseUrl)

			// If baseURL is empty, use default Vertex AI endpoint
			if baseURL == "" || baseURL == "/" {
				baseURL = "https://aiplatform.googleapis.com"
			}

			// Ensure baseURL has a scheme
			if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") && !strings.HasPrefix(baseURL, "ws://") && !strings.HasPrefix(baseURL, "wss://") {
				// Default to https if no scheme
				baseURL = "https://" + baseURL
			}

			// Convert https:// to wss:// or http:// to ws://
			if strings.HasPrefix(baseURL, "https://") {
				baseURL = "wss://" + strings.TrimPrefix(baseURL, "https://")
			} else if strings.HasPrefix(baseURL, "http://") {
				baseURL = "ws://" + strings.TrimPrefix(baseURL, "http://")
			}

			// H16: 尝试不同的 URL 路径格式
			// 可能的格式：
			// 1. /v1/projects/{project}/locations/{location}/publishers/google/models/{model}:streamGenerateContent
			// 2. /ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent (不带项目路径)
			// 3. 使用区域特定的域名: {region}-aiplatform.googleapis.com

			// 尝试使用区域特定的域名
			regionHost := fmt.Sprintf("%s-aiplatform.googleapis.com", location)
			baseURL = "wss://" + regionHost

			// 使用标准的 gRPC-Web 路径格式
			wsURL := fmt.Sprintf("%s/ws/google.cloud.aiplatform.v1beta1.LlmBidiService/BidiGenerateContent", baseURL)

			// #region agent log
			common.SysLog(fmt.Sprintf("[Vertex][H16][DEBUG] Using region-specific host: %s", regionHost))
			common.SysLog(fmt.Sprintf("[Vertex][H16][DEBUG] Built WebSocket URL: %s", wsURL))
			common.SysLog(fmt.Sprintf("[Vertex][H16][INFO] Project info will be sent in setup message, not in URL"))
			// #endregion
			return wsURL, nil
		}

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

		if info.IsStream {
			suffix = "streamGenerateContent?alt=sse"
		} else {
			suffix = "generateContent"
		}

		if strings.HasPrefix(info.UpstreamModelName, "imagen") {
			suffix = "predict"
		}

		// Check if this is an embedding model
		// Vertex AI uses :predict endpoint for embeddings (not embedContent/batchEmbedContents)
		if strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
			strings.HasPrefix(info.UpstreamModelName, "embedding") ||
			strings.HasPrefix(info.UpstreamModelName, "gemini-embedding") {
			suffix = "predict"
		}

		url, err := a.getRequestUrl(info, info.UpstreamModelName, suffix)
		if err == nil {
			common.SysLog(fmt.Sprintf("[Vertex][Gemini] 模型 %s 使用原生端点 URL: %s", info.UpstreamModelName, url))
		}
		return url, err
	} else if a.RequestMode == RequestModeClaude {
		if info.IsStream {
			suffix = "streamRawPredict?alt=sse"
		} else {
			suffix = "rawPredict"
		}
		model := info.UpstreamModelName
		if v, ok := claudeModelMap[info.UpstreamModelName]; ok {
			model = v
		}
		return a.getRequestUrl(info, model, suffix)
	} else if a.RequestMode == RequestModeLlama {
		return a.getRequestUrl(info, "", "")
	}
	return "", errors.New("unsupported request mode")
}

// vertexEmbeddingResponseHandler reads Vertex :predict embedding response and writes Gemini-format response to client.
func vertexEmbeddingResponseHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	var vertexResp VertexPredictEmbeddingResponse
	if jsonErr := json.Unmarshal(responseBody, &vertexResp); jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if len(vertexResp.Predictions) == 0 {
		return nil, types.NewOpenAIError(errors.New("no predictions in Vertex embedding response"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	usage := &dto.Usage{
		PromptTokens:     info.PromptTokens,
		CompletionTokens: 0,
		TotalTokens:      info.PromptTokens,
	}
	// 内部计费 + 返回用户：从 Vertex statistics 汇总 token
	if info.PromptTokens == 0 {
		var totalCount int
		for i := range vertexResp.Predictions {
			totalCount += vertexResp.Predictions[i].Embeddings.Statistics.TokenCount
		}
		if totalCount > 0 {
			usage.PromptTokens = totalCount
			usage.TotalTokens = totalCount
		}
	}

	usageMeta := map[string]interface{}{
		"prompt_tokens": usage.PromptTokens,
		"total_tokens":  usage.TotalTokens,
	}

	// /v1/embeddings 走 OpenAI 格式并带 usage
	if info.RelayMode == constant.RelayModeEmbeddings {
		openAIResp := dto.OpenAIEmbeddingResponse{
			Object: "list",
			Data:   make([]dto.OpenAIEmbeddingResponseItem, 0, len(vertexResp.Predictions)),
			Model:  info.UpstreamModelName,
			Usage: dto.Usage{
				PromptTokens:     usage.PromptTokens,
				CompletionTokens: 0,
				TotalTokens:      usage.TotalTokens,
			},
		}
		for i := range vertexResp.Predictions {
			openAIResp.Data = append(openAIResp.Data, dto.OpenAIEmbeddingResponseItem{
				Object:    "embedding",
				Index:     i,
				Embedding: vertexResp.Predictions[i].Embeddings.Values,
			})
		}
		jsonResponse, jsonErr := common.Marshal(openAIResp)
		if jsonErr != nil {
			return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		service.IOCopyBytesGracefully(c, resp, jsonResponse)
		return usage, nil
	}

	// Gemini 原生：返回 embedding/embeddings，用量放在 metadata.usage
	if !info.IsGeminiBatchEmbedding {
		geminiResp := dto.GeminiEmbeddingResponse{
			Embedding: dto.ContentEmbedding{Values: vertexResp.Predictions[0].Embeddings.Values},
			Metadata:  map[string]interface{}{"usage": usageMeta},
		}
		jsonResponse, jsonErr := common.Marshal(geminiResp)
		if jsonErr != nil {
			return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		service.IOCopyBytesGracefully(c, resp, jsonResponse)
		return usage, nil
	}
	embeddings := make([]*dto.ContentEmbedding, 0, len(vertexResp.Predictions))
	for i := range vertexResp.Predictions {
		embeddings = append(embeddings, &dto.ContentEmbedding{Values: vertexResp.Predictions[i].Embeddings.Values})
	}
	geminiBatchResp := dto.GeminiBatchEmbeddingResponse{
		Embeddings: embeddings,
		Metadata:   map[string]interface{}{"usage": usageMeta},
	}
	jsonResponse, jsonErr := common.Marshal(geminiBatchResp)
	if jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)

	// #region agent log
	common.SysLog(fmt.Sprintf("[Vertex][DEBUG] SetupRequestHeader - VertexKeyType: %s, IsGeminiLiveModel: %v, UpstreamModelName: %s, ChannelBaseUrl: %s", info.ChannelOtherSettings.VertexKeyType, gemini.IsGeminiLiveModel(info.UpstreamModelName), info.UpstreamModelName, info.ChannelBaseUrl))
	// #endregion

	// 检查是否使用了 Google AI Studio 端点
	// 对于 Gemini 3.1 系列模型，如果 ChannelBaseUrl 包含 generativelanguage.googleapis.com，使用 Google AI Studio 认证
	// 注意：Google AI Studio 端点只支持 API Key，不支持 Service Account
	useGoogleAIStudio := false
	if a.RequestMode == RequestModeGemini && strings.HasPrefix(info.UpstreamModelName, "gemini-3.1-") {
		if strings.Contains(info.ChannelBaseUrl, "generativelanguage.googleapis.com") {
			useGoogleAIStudio = true
			common.SysLog(fmt.Sprintf("[Vertex][Gemini] 检测到 Gemini 3.1 系列模型且使用 Google AI Studio 端点，将使用 x-goog-api-key 认证"))
		}
	}

	// 如果使用 Service Account 但需要 Google AI Studio 端点，这是不支持的
	if useGoogleAIStudio && info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
		return fmt.Errorf("Google AI Studio 端点不支持 Service Account，只支持 API Key。请使用 API Key 类型的 Channel")
	}

	// Initialize AccountCredentials if using Service Account mode
	if !useGoogleAIStudio && info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey && a.AccountCredentials.ClientEmail == "" {
		adc := &Credentials{}
		if err := common.Unmarshal([]byte(info.ApiKey), adc); err != nil {
			return fmt.Errorf("failed to decode credentials file: %w", err)
		}
		a.AccountCredentials = *adc
		// #region agent log
		common.SysLog(fmt.Sprintf("[Vertex][DEBUG] Initialized AccountCredentials - ProjectID: %s, ClientEmail: %s", a.AccountCredentials.ProjectID, a.AccountCredentials.ClientEmail))
		// #endregion
	}

	// Handle Gemini Live API (WebSocket) authentication
	if gemini.IsGeminiLiveModel(info.UpstreamModelName) {
		if info.ChannelOtherSettings.VertexKeyType == dto.VertexKeyTypeAPIKey {
			// For API Key mode with Gemini Live, use API Key directly
			req.Set("Authorization", "Bearer "+info.ApiKey)
		} else {
			// For Service Account mode, get OAuth2 token
			accessToken, err := getAccessToken(a, info)
			if err != nil {
				return err
			}
			req.Set("Authorization", "Bearer "+accessToken)
		}
	} else if useGoogleAIStudio {
		// Google AI Studio 端点使用 x-goog-api-key header（仅支持 API Key）
		// 注意：Google AI Studio 不支持 Service Account，只支持 API Key
		req.Set("x-goog-api-key", info.ApiKey)
		common.SysLog(fmt.Sprintf("[Vertex][Gemini] 使用 Google AI Studio 端点，设置 x-goog-api-key header"))
	} else {
		// Vertex AI 端点使用 OAuth2 token（Service Account）或 API Key
		if info.ChannelOtherSettings.VertexKeyType != dto.VertexKeyTypeAPIKey {
			// Vertex AI 端点使用 OAuth2 token
			accessToken, err := getAccessToken(a, info)
			if err != nil {
				return err
			}
			req.Set("Authorization", "Bearer "+accessToken)
		}
	}

	// x-goog-user-project 仅用于 Vertex AI 端点，不用于 Google AI Studio
	if !useGoogleAIStudio && a.AccountCredentials.ProjectID != "" {
		req.Set("x-goog-user-project", a.AccountCredentials.ProjectID)
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if a.RequestMode == RequestModeGemini && strings.HasPrefix(info.UpstreamModelName, "imagen") {
		prompt := ""
		for _, m := range request.Messages {
			if m.Role == "user" {
				prompt = m.StringContent()
				if prompt != "" {
					break
				}
			}
		}
		if prompt == "" {
			if p, ok := request.Prompt.(string); ok {
				prompt = p
			}
		}
		if prompt == "" {
			return nil, errors.New("prompt is required for image generation")
		}

		imgReq := dto.ImageRequest{
			Model:  request.Model,
			Prompt: prompt,
			N:      1,
			Size:   "1024x1024",
		}
		if request.N > 0 {
			imgReq.N = uint(request.N)
		}
		if request.Size != "" {
			imgReq.Size = request.Size
		}
		if len(request.ExtraBody) > 0 {
			var extra map[string]any
			if err := json.Unmarshal(request.ExtraBody, &extra); err == nil {
				if n, ok := extra["n"].(float64); ok && n > 0 {
					imgReq.N = uint(n)
				}
				if size, ok := extra["size"].(string); ok {
					imgReq.Size = size
				}
				// accept aspectRatio in extra body (top-level or under parameters)
				if ar, ok := extra["aspectRatio"].(string); ok && ar != "" {
					imgReq.Size = ar
				}
				if params, ok := extra["parameters"].(map[string]any); ok {
					if ar, ok := params["aspectRatio"].(string); ok && ar != "" {
						imgReq.Size = ar
					}
				}
			}
		}
		c.Set("request_model", request.Model)
		return a.ConvertImageRequest(c, info, imgReq)
	}
	if a.RequestMode == RequestModeClaude {
		claudeReq, err := claude.RequestOpenAI2ClaudeMessage(c, *request, info)
		if err != nil {
			return nil, err
		}
		vertexClaudeReq := copyRequest(claudeReq, anthropicVersion)
		c.Set("request_model", claudeReq.Model)
		info.UpstreamModelName = claudeReq.Model
		return vertexClaudeReq, nil
	} else if a.RequestMode == RequestModeGemini {
		if info.UseGeminiOpenAICompatibleEndpoint {
			pass := *request
			pass.Model = "google/" + info.UpstreamModelName
			helper.EnsureGeminiThoughtSignaturesForOpenAIRequest(&pass)
			common.SysLog(fmt.Sprintf("[Vertex][Gemini] ConvertOpenAIRequest: 使用兼容端点，透传 OpenAI 格式，model=%s", pass.Model))
			c.Set("request_model", request.Model)
			return &pass, nil
		}
		geminiRequest, err := gemini.CovertGemini2OpenAI(c, *request, info)
		if err != nil {
			return nil, err
		}
		c.Set("request_model", request.Model)
		return geminiRequest, nil
	} else if a.RequestMode == RequestModeLlama {
		return request, nil
	}
	return nil, errors.New("unsupported request mode")
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	// Vertex :predict 需要 instances，不能使用 Gemini 的 requests 格式
	if request.Input == nil {
		return nil, errors.New("input is required")
	}
	inputs := request.ParseInput()
	if len(inputs) == 0 {
		return nil, errors.New("input is empty")
	}
	info.IsGeminiBatchEmbedding = len(inputs) > 1
	instances := make([]VertexPredictEmbeddingInstance, 0, len(inputs))
	for _, text := range inputs {
		instances = append(instances, VertexPredictEmbeddingInstance{Content: text})
	}
	return &VertexPredictEmbeddingRequest{Instances: instances}, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	// Check if this is a Gemini Live API request (WebSocket)
	if gemini.IsGeminiLiveModel(info.UpstreamModelName) {
		return channel.DoWssRequest(a, c, info, requestBody)
	}
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// Handle Gemini Live API (WebSocket)
	if gemini.IsGeminiLiveModel(info.UpstreamModelName) && info.TargetWs != nil {
		newErr, realtimeUsage := gemini.GeminiLiveHandler(c, info)
		return realtimeUsage, newErr
	}

	if info.IsStream {
		switch a.RequestMode {
		case RequestModeClaude:
			return claude.ClaudeStreamHandler(c, resp, info, claude.RequestModeMessage)
		case RequestModeGemini:
			if info.UseGeminiOpenAICompatibleEndpoint {
				return openai.OaiStreamHandler(c, info, resp)
			}
			if info.RelayMode == constant.RelayModeGemini {
				return gemini.GeminiTextGenerationStreamHandler(c, info, resp)
			} else {
				return gemini.GeminiChatStreamHandler(c, info, resp)
			}
		case RequestModeLlama:
			return openai.OaiStreamHandler(c, info, resp)
		}
	} else {
		switch a.RequestMode {
		case RequestModeClaude:
			return claude.ClaudeHandler(c, resp, info, claude.RequestModeMessage)
		case RequestModeGemini:
			if info.UseGeminiOpenAICompatibleEndpoint {
				return openai.OpenaiHandler(c, info, resp)
			}
			if info.RelayMode == constant.RelayModeGemini {
				// Check if it's an embedding request in Gemini native format
				if strings.Contains(info.RequestURLPath, ":embedContent") ||
					strings.Contains(info.RequestURLPath, ":batchEmbedContents") ||
					strings.Contains(info.RequestURLPath, ":predict") &&
						(strings.HasPrefix(info.UpstreamModelName, "text-embedding") ||
							strings.HasPrefix(info.UpstreamModelName, "embedding") ||
							strings.HasPrefix(info.UpstreamModelName, "gemini-embedding")) {
					// Vertex returns :predict format (predictions); convert to Gemini format for client
					return vertexEmbeddingResponseHandler(c, resp, info)
				}
				return gemini.GeminiTextGenerationHandler(c, info, resp)
			} else if info.RelayMode == constant.RelayModeEmbeddings {
				// Vertex returns :predict format (predictions); convert to Gemini format for client
				return vertexEmbeddingResponseHandler(c, resp, info)
			} else {
				if strings.HasPrefix(info.UpstreamModelName, "imagen") {
					return gemini.GeminiImageHandler(c, info, resp)
				}
				return gemini.GeminiChatHandler(c, info, resp)
			}
		case RequestModeLlama:
			return openai.OpenaiHandler(c, info, resp)
		}
	}
	return
}

func (a *Adaptor) GetModelList() []string {
	var modelList []string
	for i, s := range ModelList {
		modelList = append(modelList, s)
		ModelList[i] = s
	}
	for i, s := range claude.ModelList {
		modelList = append(modelList, s)
		claude.ModelList[i] = s
	}
	for i, s := range gemini.ModelList {
		modelList = append(modelList, s)
		gemini.ModelList[i] = s
	}
	return modelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
