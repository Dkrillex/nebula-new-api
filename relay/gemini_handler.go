package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/relay/channel"
	"one-api/relay/channel/vertex"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func isNoThinkingRequest(req *dto.GeminiChatRequest) bool {
	if req.GenerationConfig.ThinkingConfig != nil && req.GenerationConfig.ThinkingConfig.ThinkingBudget != nil {
		configBudget := req.GenerationConfig.ThinkingConfig.ThinkingBudget
		if configBudget != nil && *configBudget == 0 {
			// 如果思考预算为 0，则认为是非思考请求
			return true
		}
	}
	return false
}

func trimModelThinking(modelName string) string {
	// 去除模型名称中的 -nothinking 后缀
	if strings.HasSuffix(modelName, "-nothinking") {
		return strings.TrimSuffix(modelName, "-nothinking")
	}
	// 去除模型名称中的 -thinking 后缀
	if strings.HasSuffix(modelName, "-thinking") {
		return strings.TrimSuffix(modelName, "-thinking")
	}

	// 去除模型名称中的 -thinking-number
	if strings.Contains(modelName, "-thinking-") {
		parts := strings.Split(modelName, "-thinking-")
		if len(parts) > 1 {
			return parts[0] + "-thinking"
		}
	}
	return modelName
}

func GeminiHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	geminiReq, ok := info.Request.(*dto.GeminiChatRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.GeminiChatRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(geminiReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeminiChatRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// model mapped 模型映射
	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled {
		if isNoThinkingRequest(request) {
			// check is thinking
			if !strings.Contains(info.OriginModelName, "-nothinking") {
				// try to get no thinking model price
				noThinkingModelName := info.OriginModelName + "-nothinking"
				containPrice := helper.ContainPriceOrRatio(noThinkingModelName)
				if containPrice {
					info.OriginModelName = noThinkingModelName
					info.UpstreamModelName = noThinkingModelName
				}
			}
		}
		// 重要：Gemini 原生请求（/v1beta/models/*）应“透传”为主。
		// 如果 SDK 没有传 thinkingConfig，这里不能擅自注入默认 thinking_level=HIGH，
		// 否则会导致像 gemini-3-pro-image-preview 这类不支持 thinking_level 的模型直接 400。
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}

	adaptor.Init(info)

	if info.ChannelSetting.SystemPrompt != "" {
		if request.SystemInstructions == nil {
			request.SystemInstructions = &dto.GeminiChatContent{
				Parts: []dto.GeminiPart{
					{Text: info.ChannelSetting.SystemPrompt},
				},
			}
		} else if len(request.SystemInstructions.Parts) == 0 {
			request.SystemInstructions.Parts = []dto.GeminiPart{{Text: info.ChannelSetting.SystemPrompt}}
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			merged := false
			for i := range request.SystemInstructions.Parts {
				if request.SystemInstructions.Parts[i].Text == "" {
					continue
				}
				request.SystemInstructions.Parts[i].Text = info.ChannelSetting.SystemPrompt + "\n" + request.SystemInstructions.Parts[i].Text
				merged = true
				break
			}
			if !merged {
				request.SystemInstructions.Parts = append([]dto.GeminiPart{{Text: info.ChannelSetting.SystemPrompt}}, request.SystemInstructions.Parts...)
			}
		}
	}

	// Clean up empty system instruction
	if request.SystemInstructions != nil {
		hasContent := false
		for _, part := range request.SystemInstructions.Parts {
			if part.Text != "" {
				hasContent = true
				break
			}
		}
		if !hasContent {
			request.SystemInstructions = nil
		}
	}

	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = bytes.NewReader(body)
	} else {
		// 使用 ConvertGeminiRequest 转换请求格式
		convertedRequest, err := adaptor.ConvertGeminiRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// apply param override
		if len(info.ParamOverride) > 0 {
			jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
			if err != nil {
				return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
			}
		}

		truncatedBody := common.TruncateJsonValues(string(jsonData))
		logger.LogDebug(c, "Gemini request body: "+truncatedBody)

		requestBody = bytes.NewReader(jsonData)
	}

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		logger.LogError(c, "Do gemini request failed: "+err.Error())
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	if openaiErr != nil {
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		return openaiErr
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), "")
	return nil
}

func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	isBatch := strings.HasSuffix(c.Request.URL.Path, "batchEmbedContents")
	info.IsGeminiBatchEmbedding = isBatch

	// First, peek at the request body to check if it's Vertex instances format
	bodyBytes, err := common.GetRequestBody(c)
	if err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// Check if request body contains "instances" (Vertex format)
	var bodyMap map[string]interface{}
	if err := common.Unmarshal(bodyBytes, &bodyMap); err == nil {
		if _, hasInstances := bodyMap["instances"]; hasInstances {
			// This is Vertex native format, handle it directly
			common.SysLog("[GeminiEmbedding] Detected Vertex native instances format, using as-is")
			return handleVertexNativeEmbedding(c, info, bodyBytes)
		}
	}

	// Otherwise, parse as Gemini format
	var req dto.Request
	var inputTexts []string

	if isBatch {
		batchRequest := &dto.GeminiBatchEmbeddingRequest{}
		err = common.Unmarshal(bodyBytes, batchRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req = batchRequest
		for _, r := range batchRequest.Requests {
			for _, part := range r.Content.Parts {
				if part.Text != "" {
					inputTexts = append(inputTexts, part.Text)
				}
			}
		}
	} else {
		singleRequest := &dto.GeminiEmbeddingRequest{}
		err = common.Unmarshal(bodyBytes, singleRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
		}
		req = singleRequest
		for _, part := range singleRequest.Content.Parts {
			if part.Text != "" {
				inputTexts = append(inputTexts, part.Text)
			}
		}
	}

	err = helper.ModelMappedHelper(c, info, req)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader
	var jsonData []byte

	// Check if we need to use Vertex predict format:
	// 1. API type is VertexAI (19), OR
	// 2. API type is Gemini (9) but base URL points to aiplatform.googleapis.com (Vertex endpoint)
	useVertexFormat := info.ApiType == constant.APITypeVertexAi ||
		(info.ApiType == constant.APITypeGemini && strings.Contains(info.ChannelBaseUrl, "aiplatform.googleapis.com"))

	// Log for debugging
	common.SysLog(fmt.Sprintf("[GeminiEmbedding] ApiType=%d, APITypeVertexAi=%d, APITypeGemini=%d, ChannelBaseUrl=%s, isBatch=%v, useVertexFormat=%v",
		info.ApiType, constant.APITypeVertexAi, constant.APITypeGemini, info.ChannelBaseUrl, isBatch, useVertexFormat))

	// Vertex AI uses :predict with "instances" body; Gemini uses embedContent/batchEmbedContents with model/content body
	if useVertexFormat {
		common.SysLog(fmt.Sprintf("[Vertex][Embedding] Converting Gemini format to Vertex predict format, isBatch=%v", isBatch))
		vertexReq, _ := vertex.GeminiEmbeddingToVertexPredictRequest(req, isBatch)
		if vertexReq == nil {
			return types.NewError(fmt.Errorf("convert to Vertex embedding request failed"), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err = common.Marshal(vertexReq)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		common.SysLog(fmt.Sprintf("[Vertex][Embedding] Converted request body: %s", string(jsonData)))
	} else {
		common.SysLog(fmt.Sprintf("[GeminiEmbedding] Using Gemini native format (not Vertex)"))
		jsonData, err = common.Marshal(req)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		common.SysLog(fmt.Sprintf("[GeminiEmbedding] Request body: %s", string(jsonData)))
	}

	// apply param override
	if len(info.ParamOverride) > 0 {
		reqMap := make(map[string]interface{})
		_ = common.Unmarshal(jsonData, &reqMap)
		for key, value := range info.ParamOverride {
			reqMap[key] = value
		}
		jsonData, err = common.Marshal(reqMap)
		if err != nil {
			return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
		}
	}
	requestBody = bytes.NewReader(jsonData)

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		logger.LogError(c, "Do gemini request failed: "+err.Error())
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, openaiErr := adaptor.DoResponse(c, resp.(*http.Response), info)
	if openaiErr != nil {
		service.ResetStatusCode(openaiErr, statusCodeMappingStr)
		return openaiErr
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), "")
	return nil
}

// handleVertexNativeEmbedding handles Vertex AI native instances format
func handleVertexNativeEmbedding(c *gin.Context, info *relaycommon.RelayInfo, bodyBytes []byte) *types.NewAPIError {
	// Parse Vertex instances request to count tokens
	var vertexReq vertex.VertexPredictEmbeddingRequest
	if err := common.Unmarshal(bodyBytes, &vertexReq); err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	// Count characters from all instances
	var totalChars int
	for _, instance := range vertexReq.Instances {
		totalChars += utf8.RuneCountInString(instance.Content)
		totalChars += utf8.RuneCountInString(instance.Title)
	}

	// Set prompt tokens (characters)
	common.SetContextKey(c, constant.ContextKeyPromptTokens, totalChars)
	info.PromptTokens = totalChars

	// Check if batch
	info.IsGeminiBatchEmbedding = len(vertexReq.Instances) > 1

	err := helper.ModelMappedHelper(c, info, nil)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	// For Vertex native format, use body as-is (no conversion needed)
	requestBody := bytes.NewReader(bodyBytes)

	common.SysLog(fmt.Sprintf("[VertexNative][Embedding] Using Vertex instances format as-is, instances count: %d", len(vertexReq.Instances)))

	resp, err := channel.DoApiRequest(adaptor, c, info, requestBody)
	if err != nil {
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
	}

	usage, newAPIError := adaptor.DoResponse(c, resp, info)
	if newAPIError != nil {
		return newAPIError
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), "")
	return nil
}
