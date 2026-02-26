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
	"one-api/model"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/setting/operation_setting"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

func TextHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	textReq, ok := info.Request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.GeneralOpenAIRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(textReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to GeneralOpenAIRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	if request.WebSearchOptions != nil {
		c.Set("chat_completion_web_search_context_size", request.WebSearchOptions.SearchContextSize)
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	includeUsage := true
	// 判断用户是否需要返回使用情况
	if request.StreamOptions != nil {
		includeUsage = request.StreamOptions.IncludeUsage
	}

	// 如果不支持StreamOptions，将StreamOptions设置为nil
	if !info.SupportStreamOptions || !request.Stream {
		request.StreamOptions = nil
	} else {
		// 如果支持StreamOptions，且请求中没有设置StreamOptions，根据配置文件设置StreamOptions
		if constant.ForceStreamOption {
			request.StreamOptions = &dto.StreamOptions{
				IncludeUsage: true,
			}
		}
	}

	info.ShouldIncludeUsage = includeUsage

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		if common.DebugEnabled {
			truncatedBody := common.TruncateJsonValues(string(body))
			common.SysLog(fmt.Sprintf("requestBody: %s", truncatedBody))
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		// 先调用 GetRequestURL，以便 Vertex/Gemini 设置 info.UseGeminiOpenAICompatibleEndpoint；
		// ConvertOpenAIRequest 依赖该标志决定透传 OpenAI 格式还是转为原生格式。
		if _, err := adaptor.GetRequestURL(info); err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		convertedRequest, err := adaptor.ConvertOpenAIRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		if info.ChannelSetting.SystemPrompt != "" {
			// 如果有系统提示，则将其添加到请求中
			request, ok := convertedRequest.(*dto.GeneralOpenAIRequest)
			if ok {
				containSystemPrompt := false
				for _, message := range request.Messages {
					if message.Role == request.GetSystemRoleName() {
						containSystemPrompt = true
						break
					}
				}
				if !containSystemPrompt {
					// 如果没有系统提示，则添加系统提示
					systemMessage := dto.Message{
						Role:    request.GetSystemRoleName(),
						Content: info.ChannelSetting.SystemPrompt,
					}
					request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
				} else if info.ChannelSetting.SystemPromptOverride {
					common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
					// 如果有系统提示，且允许覆盖，则拼接到前面
					for i, message := range request.Messages {
						if message.Role == request.GetSystemRoleName() {
							if message.IsStringContent() {
								request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
							} else {
								contents := message.ParseContent()
								contents = append([]dto.MediaContent{
									{
										Type: dto.ContentTypeText,
										Text: info.ChannelSetting.SystemPrompt,
									},
								}, contents...)
								request.Messages[i].Content = contents
							}
							break
						}
					}
				}
			}
		}

		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for OpenAI API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
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
		logger.LogDebug(c, fmt.Sprintf("text request body: %s", truncatedBody))

		requestBody = bytes.NewBuffer(jsonData)
	}

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return newApiErr
		}
	}

	usage, newApiErr := adaptor.DoResponse(c, httpResp, info)
	if newApiErr != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return newApiErr
	}

	if strings.HasPrefix(info.OriginModelName, "gpt-4o-audio") {
		service.PostAudioConsumeQuota(c, info, usage.(*dto.Usage), "")
	} else {
		postConsumeQuota(c, info, usage.(*dto.Usage), "")
	}
	return nil
}

func postConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {
	if usage == nil {
		usage = &dto.Usage{
			PromptTokens:     relayInfo.PromptTokens,
			CompletionTokens: 0,
			TotalTokens:      relayInfo.PromptTokens,
		}
		extraContent += "（可能是请求出错）"
	}

	modelName := relayInfo.OriginModelName
	tokenName := ctx.GetString("token_name")
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio

	// ========== 特殊计费：图像Token表定价（gpt-image-1）==========
	if relayInfo.PriceData.UseImageTokenPricing {
		quota := calculateImageTokenPricingQuota(ctx, relayInfo, usage, extraContent)
		recordImageTokenPricingConsume(ctx, relayInfo, usage, quota, tokenName)
		return
	}

	// ========== 常规计费流程 ==========
	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	cacheTokens := usage.PromptTokensDetails.CachedTokens
	imageTokens := usage.PromptTokensDetails.ImageTokens
	audioTokens := usage.PromptTokensDetails.AudioTokens
	completionTokens := usage.CompletionTokens
	cachedCreationTokens := usage.PromptTokensDetails.CachedCreationTokens

	// 注意：上游返回的 prompt_tokens 包含了 cache_tokens、image_tokens、audio_tokens 等所有输入tokens
	// 为了正确记录日志和计费，需要从 promptTokens 中减去这些详细分类的tokens，避免重复计算
	// 对于 Azure OpenAI 和 OpenAI，prompt_tokens 已经包含了 cache_tokens，需要减去以便分开记录
	// 注意：OpenRouter 在 PostClaudeConsumeQuota 中已经处理了这个问题
	// 注意：Gemini API 的 promptTokenCount 不包含 cachedContentTokenCount，它们是分开返回的，所以不需要减去
	if relayInfo.ChannelType == constant.ChannelTypeAzure ||
		relayInfo.ChannelType == constant.ChannelTypeOpenAI {
		if cacheTokens > 0 && promptTokens >= cacheTokens {
			promptTokens -= cacheTokens
		}
		if cachedCreationTokens > 0 && promptTokens >= cachedCreationTokens {
			promptTokens -= cachedCreationTokens
		}
	}
	// gpt-image-1 的图片输入单独计费，baseTokens 只保留文本，避免图片 token 被重复计费（不依赖 ChannelType，有 image 明细即扣减）
	if strings.HasPrefix(modelName, "gpt-image-1") && imageTokens > 0 && promptTokens >= imageTokens {
		promptTokens -= imageTokens
	}

	completionRatio := relayInfo.PriceData.CompletionRatio
	cacheRatio := relayInfo.PriceData.CacheRatio
	imageRatio := relayInfo.PriceData.ImageRatio
	imageCompletionRatio := relayInfo.PriceData.ImageCompletionRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	cachedCreationRatio := relayInfo.PriceData.CacheCreationRatio

	// 获取 Gemini 图片和文本输出 tokens（用于计费和日志记录）
	geminiImageOutputTokens := ctx.GetInt("gemini_image_output_tokens")
	geminiTextOutputTokens := ctx.GetInt("gemini_text_output_tokens")

	// Convert values to decimal for precise calculation
	dPromptTokens := decimal.NewFromInt(int64(promptTokens))
	dCacheTokens := decimal.NewFromInt(int64(cacheTokens))
	dImageTokens := decimal.NewFromInt(int64(imageTokens))
	dAudioTokens := decimal.NewFromInt(int64(audioTokens))
	dCompletionTokens := decimal.NewFromInt(int64(completionTokens))
	dCachedCreationTokens := decimal.NewFromInt(int64(cachedCreationTokens))
	dCompletionRatio := decimal.NewFromFloat(completionRatio)
	dCacheRatio := decimal.NewFromFloat(cacheRatio)
	dImageRatio := decimal.NewFromFloat(imageRatio)
	dModelRatio := decimal.NewFromFloat(modelRatio)
	dGroupRatio := decimal.NewFromFloat(groupRatio)
	dModelPrice := decimal.NewFromFloat(modelPrice)
	dCachedCreationRatio := decimal.NewFromFloat(cachedCreationRatio)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)

	ratio := dModelRatio.Mul(dGroupRatio)

	// 获取OEM用户折扣（用于按张计费等特殊计费场景）
	oemUserDiscount := service.GetOemUserDiscountForQuota(ctx, modelName)

	// openai web search 工具计费
	var dWebSearchQuota decimal.Decimal
	var webSearchPrice float64
	// response api 格式工具计费
	if relayInfo.ResponsesUsageInfo != nil {
		if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool.CallCount > 0 {
			// 计算 web search 调用的配额 (配额 = 价格 * 调用次数 / 1000 * 分组倍率)
			webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, webSearchTool.SearchContextSize)
			dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
				Mul(decimal.NewFromInt(int64(webSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("Web Search 调用 %d 次，上下文大小 %s，调用花费 %s",
				webSearchTool.CallCount, webSearchTool.SearchContextSize, dWebSearchQuota.String())
		}
	} else if strings.HasSuffix(modelName, "search-preview") {
		// search-preview 模型不支持 response api
		searchContextSize := ctx.GetString("chat_completion_web_search_context_size")
		if searchContextSize == "" {
			searchContextSize = "medium"
		}
		webSearchPrice = operation_setting.GetWebSearchPricePerThousand(modelName, searchContextSize)
		dWebSearchQuota = decimal.NewFromFloat(webSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Web Search 调用 1 次，上下文大小 %s，调用花费 %s",
			searchContextSize, dWebSearchQuota.String())
	}
	// claude web search tool 计费
	var dClaudeWebSearchQuota decimal.Decimal
	var claudeWebSearchPrice float64
	claudeWebSearchCallCount := ctx.GetInt("claude_web_search_requests")
	if claudeWebSearchCallCount > 0 {
		claudeWebSearchPrice = operation_setting.GetClaudeWebSearchPricePerThousand()
		dClaudeWebSearchQuota = decimal.NewFromFloat(claudeWebSearchPrice).
			Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit).Mul(decimal.NewFromInt(int64(claudeWebSearchCallCount)))
		extraContent += fmt.Sprintf("Claude Web Search 调用 %d 次，调用花费 %s",
			claudeWebSearchCallCount, dClaudeWebSearchQuota.String())
	}
	// file search tool 计费
	var dFileSearchQuota decimal.Decimal
	var fileSearchPrice float64
	if relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists && fileSearchTool.CallCount > 0 {
			fileSearchPrice = operation_setting.GetFileSearchPricePerThousand()
			dFileSearchQuota = decimal.NewFromFloat(fileSearchPrice).
				Mul(decimal.NewFromInt(int64(fileSearchTool.CallCount))).
				Div(decimal.NewFromInt(1000)).Mul(dGroupRatio).Mul(dQuotaPerUnit)
			extraContent += fmt.Sprintf("File Search 调用 %d 次，调用花费 %s",
				fileSearchTool.CallCount, dFileSearchQuota.String())
		}
	}
	var dImageGenerationCallQuota decimal.Decimal
	var imageGenerationCallPrice float64
	if ctx.GetBool("image_generation_call") {
		imageGenerationCallPrice = operation_setting.GetGPTImage1PriceOnceCall(ctx.GetString("image_generation_call_quality"), ctx.GetString("image_generation_call_size"))
		dImageGenerationCallQuota = decimal.NewFromFloat(imageGenerationCallPrice).Mul(dGroupRatio).Mul(dQuotaPerUnit)
		extraContent += fmt.Sprintf("Image Generation Call 花费 %s", dImageGenerationCallQuota.String())
	}

	var quotaCalculateDecimal decimal.Decimal

	var audioInputQuota decimal.Decimal
	var audioInputPrice float64
	if !relayInfo.PriceData.UsePrice {
		baseTokens := dPromptTokens
		// 注意：对于 Azure/OpenAI，promptTokens 经过上面处理后已经减去了 cache/image/audio tokens
		// 对于 Gemini，promptTokens 本身就不包含 cachedTokens（它们是分开返回的）
		// 所以这里的 baseTokens 对于不同提供商有不同的含义：
		// - Azure/OpenAI: 是减去了缓存后的非缓存部分
		// - Gemini: 是原始的非缓存输入 tokens
		// 下面需要把各种特殊类型的tokens按倍率加回来
		var cachedTokensWithRatio decimal.Decimal
		if !dCacheTokens.IsZero() {
			cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
		}
		var dCachedCreationTokensWithRatio decimal.Decimal
		if !dCachedCreationTokens.IsZero() {
			dCachedCreationTokensWithRatio = dCachedCreationTokens.Mul(dCachedCreationRatio)
		}

		// image tokens 按倍率计算
		var imageTokensWithRatio decimal.Decimal
		if !dImageTokens.IsZero() {
			imageTokensWithRatio = dImageTokens.Mul(dImageRatio)
		}

		// gpt-image-1 使用「图片与基础倍率2一致、输出=图片输入价格×补全倍率」的单独计费，不乘 modelRatio
		isGptImage1 := strings.HasPrefix(modelName, "gpt-image-1")
		var imageQuotaGptImage1 decimal.Decimal
		var imageCompletionQuotaGptImage1 decimal.Decimal
		if isGptImage1 {
			imageQuotaGptImage1 = dImageTokens.Mul(dImageRatio).Mul(dGroupRatio)
		}

		// Gemini audio tokens 特殊价格计算
		if !dAudioTokens.IsZero() {
			audioInputPrice = operation_setting.GetGeminiInputAudioPricePerMillionTokens(modelName)
			if audioInputPrice > 0 {
				audioInputQuota = decimal.NewFromFloat(audioInputPrice).Div(decimal.NewFromInt(1000000)).Mul(dAudioTokens).Mul(dGroupRatio).Mul(dQuotaPerUnit)
				extraContent += fmt.Sprintf("Audio Input 花费 %s", audioInputQuota.String())
			}
		}
		// 计算输入配额（详细日志）；gpt-image-1 的图片输入单独计费，不并入 promptQuota
		var promptQuota decimal.Decimal
		if isGptImage1 {
			promptQuota = baseTokens.Add(cachedTokensWithRatio).Add(dCachedCreationTokensWithRatio)
		} else {
			promptQuota = baseTokens.Add(cachedTokensWithRatio).
				Add(imageTokensWithRatio).
				Add(dCachedCreationTokensWithRatio)
		}

		// 检查是否有图片输出（包括 gpt-image-1 和 Gemini）
		var completionQuota decimal.Decimal
		gptImageOutputTokens := ctx.GetInt("gpt_image_output_tokens")

		if gptImageOutputTokens > 0 && imageCompletionRatio > 0 {
			imageTokens := decimal.NewFromInt(int64(gptImageOutputTokens))
			dImageCompletionRatio := decimal.NewFromFloat(imageCompletionRatio)
			imageCompletionQuota := imageTokens.Mul(dImageCompletionRatio)
			completionQuota = imageCompletionQuota
			if isGptImage1 {
				// gpt-image-1：输出 = 图片输入单价×补全倍率×groupRatio，不再乘 modelRatio
				imageCompletionQuotaGptImage1 = imageTokens.Mul(dImageRatio).Mul(dImageCompletionRatio).Mul(dGroupRatio)
				completionQuota = decimal.Zero
			}
			extraContent += fmt.Sprintf("，图片输出 %d tokens × %.2f",
				gptImageOutputTokens, imageCompletionRatio)

		} else if geminiImageOutputTokens > 0 && imageCompletionRatio > 0 {
			// 原有的 Gemini 逻辑
			// 分别计算文本输出和图片输出费用
			textOutputTokens := int64(geminiTextOutputTokens)
			if textOutputTokens == 0 {
				// 如果没有单独统计文本 tokens，从 completionTokens 中减去图片 tokens
				imageTokens := int64(geminiImageOutputTokens)
				if dCompletionTokens.GreaterThanOrEqual(decimal.NewFromInt(imageTokens)) {
					textOutputTokens = dCompletionTokens.Sub(decimal.NewFromInt(imageTokens)).IntPart()
				}
				if textOutputTokens < 0 {
					textOutputTokens = 0
				}
			}

			imageTokens := decimal.NewFromInt(int64(geminiImageOutputTokens))
			dImageCompletionRatio := decimal.NewFromFloat(imageCompletionRatio)

			textCompletionQuota := decimal.NewFromInt(textOutputTokens).Mul(dCompletionRatio)
			imageCompletionQuota := imageTokens.Mul(dImageCompletionRatio)

			completionQuota = textCompletionQuota.Add(imageCompletionQuota)

			extraContent += fmt.Sprintf("，文本输出 %d tokens × %.2f + 图片输出 %d tokens × %.2f",
				textOutputTokens, completionRatio, geminiImageOutputTokens, imageCompletionRatio)
		} else {
			// 常规计费：所有 completion tokens 使用 CompletionRatio
			if gptImageOutputTokens > 0 {

			}
			completionQuota = dCompletionTokens.Mul(dCompletionRatio)
		}

		if isGptImage1 {
			quotaCalculateDecimal = promptQuota.Mul(ratio).Add(imageQuotaGptImage1).Add(imageCompletionQuotaGptImage1)
		} else {
			quotaCalculateDecimal = promptQuota.Add(completionQuota).Mul(ratio)
		}
		// 注意：oemUserDiscount 已经在 price.go 的 ModelPriceHelper 中应用到 modelRatio 了
		// 所以这里不需要再乘以 oemUserDiscount，否则会重复应用折扣

		if !ratio.IsZero() && quotaCalculateDecimal.LessThanOrEqual(decimal.Zero) {
			quotaCalculateDecimal = decimal.NewFromInt(1)
		}
	} else {
		// 按次计费（例如图片生成）——支持按图片数量乘以单价
		multiplier := 1
		if relayInfo.RelayMode == relayconstant.RelayModeImagesGenerations {
			// 获取图片数量
			if v, exists := ctx.Get("generated_images_count"); exists {
				if n, ok := v.(int); ok && n > 0 {
					multiplier = n
				}
			} else if usage != nil && usage.TotalTokens > 0 {
				// 兜底：没有透传图片数量时，使用 usage.TotalTokens（在 ImageHandler 中默认为请求 N）
				multiplier = usage.TotalTokens
			}

			// 优先检查是否有按张计费配置
			imagePrice, hasImagePrice := ratio_setting.GetImageModelPricePerImage(modelName)

			if hasImagePrice && imagePrice > 0 {
				// 获取OEM代码和厂商名称（用于OEM价格链条计算）
				// 默认系统
				if code, exists := ctx.Get(string(constant.ContextKeyOemCode)); exists {
					if codeStr, ok := code.(string); ok && codeStr != "" {

					}
				}
				vendorName := service.GetVendorNameFromModel(modelName)

				// 应用OEM用户折扣（用于用户实际支付价）
				// 注意：这里使用 oemUserDiscount 而不是 systemDiscount
				// systemDiscount 是平台给OEM的折扣，用于计算系统销售价和平台利润
				// oemUserDiscount 是OEM给用户的折扣，用于计算用户实际支付价
				discountedImagePrice := imagePrice * oemUserDiscount
				// 更新 relayInfo.PriceData.ModelPrice 为折扣后的价格，用于后续日志记录
				relayInfo.PriceData.ModelPrice = discountedImagePrice

				// 使用按张价格计费（单位：美元/张），已应用OEM用户折扣和分组倍率
				quotaCalculateDecimal = decimal.NewFromFloat(discountedImagePrice).
					Mul(decimal.NewFromInt(int64(multiplier))).
					Mul(dQuotaPerUnit).
					Mul(dGroupRatio)

				// 计算人民币价格用于日志显示
				priceInCNY := discountedImagePrice * 7.3 // USD to CNY
				totalPriceInCNY := priceInCNY * float64(multiplier)
				_ = vendorName // 用于价格链条计算
				extraContent += fmt.Sprintf("图片生成：%d张 × ¥%.2f = ¥%.2f (OEM用户折扣: %.2f%%, 分组倍率: %.2f)",
					multiplier, priceInCNY, totalPriceInCNY, oemUserDiscount*100, groupRatio)
			} else {
				// 使用原有的按次计费逻辑（ModelPrice），modelPrice 已在 price.go 中应用了 oemUserDiscount
				quotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio).Mul(decimal.NewFromInt(int64(multiplier)))
				if multiplier > 1 {
					extraContent += fmt.Sprintf("，按次计费×图片数：单价 %v，数量 %d", modelPrice, multiplier)
				}
			}
		} else {
			// 非图片生成的按次计费，modelPrice 已在 price.go 中应用了 oemUserDiscount
			quotaCalculateDecimal = dModelPrice.Mul(dQuotaPerUnit).Mul(dGroupRatio)
		}
	}

	quotaCalculateDecimal = quotaCalculateDecimal.Add(dWebSearchQuota)
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dFileSearchQuota)
	// 添加 audio input 独立计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(audioInputQuota)
	// 添加 image generation call 计费
	quotaCalculateDecimal = quotaCalculateDecimal.Add(dImageGenerationCallQuota)

	quota := int(quotaCalculateDecimal.Round(0).IntPart())
	totalTokens := promptTokens + completionTokens

	var logContent string

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += "（可能是上游超时）"
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		if !ratio.IsZero() && quota == 0 {
			quota = 1
		}
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

	//logger.LogInfo(ctx, fmt.Sprintf("request quota delta: %s", logger.FormatQuota(quotaDelta)))

	if quotaDelta > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	} else if quotaDelta < 0 {
		logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(-quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	}

	if quotaDelta != 0 {
		err := service.PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	logModel := modelName
	if strings.HasPrefix(logModel, "gpt-4-gizmo") {
		logModel = "gpt-4-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if strings.HasPrefix(logModel, "gpt-4o-gizmo") {
		logModel = "gpt-4o-gizmo-*"
		logContent += fmt.Sprintf("，模型 %s", modelName)
	}
	if extraContent != "" {
		if logContent != "" {
			logContent += ", " + extraContent
		} else {
			logContent = extraContent
		}
	}
	other := service.GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if imageTokens != 0 {
		other["image"] = true
		other["image_ratio"] = imageRatio
		other["image_output"] = imageTokens
		// 供前端计费过程展示：图片输入 token 数与单价（美元/1M），与 controller 展示一致 imageRatio×$2/M
		other["input_image_tokens"] = imageTokens
		other["input_image_price"] = imageRatio * 2.0
	}
	if cachedCreationTokens != 0 {
		other["cache_creation_tokens"] = cachedCreationTokens
		other["cache_creation_ratio"] = cachedCreationRatio
	}
	// 记录按次×图片数量的信息
	if relayInfo.PriceData.UsePrice && relayInfo.RelayMode == relayconstant.RelayModeImagesGenerations {
		if v, exists := ctx.Get("generated_images_count"); exists {
			if n, ok := v.(int); ok && n > 0 {
				other["per_call_image_multiplier"] = n
				other["per_call_price"] = modelPrice
			}
		} else if usage != nil && usage.TotalTokens > 0 {
			other["per_call_image_multiplier"] = usage.TotalTokens
			other["per_call_price"] = modelPrice
		}
	}
	if !dWebSearchQuota.IsZero() {
		if relayInfo.ResponsesUsageInfo != nil {
			if webSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists {
				other["web_search"] = true
				other["web_search_call_count"] = webSearchTool.CallCount
				other["web_search_price"] = webSearchPrice
			}
		} else if strings.HasSuffix(modelName, "search-preview") {
			other["web_search"] = true
			other["web_search_call_count"] = 1
			other["web_search_price"] = webSearchPrice
		}
	} else if !dClaudeWebSearchQuota.IsZero() {
		other["web_search"] = true
		other["web_search_call_count"] = claudeWebSearchCallCount
		other["web_search_price"] = claudeWebSearchPrice
	}
	if !dFileSearchQuota.IsZero() && relayInfo.ResponsesUsageInfo != nil {
		if fileSearchTool, exists := relayInfo.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolFileSearch]; exists {
			other["file_search"] = true
			other["file_search_call_count"] = fileSearchTool.CallCount
			other["file_search_price"] = fileSearchPrice
		}
	}
	if !audioInputQuota.IsZero() {
		other["audio_input_seperate_price"] = true
		other["audio_input_token_count"] = audioTokens
		other["audio_input_price"] = audioInputPrice
	}
	if !dImageGenerationCallQuota.IsZero() {
		other["image_generation_call"] = true
		other["image_generation_call_price"] = imageGenerationCallPrice
	}
	// 记录 gpt-image-1 图片输出 tokens 详情
	gptImageOutputTokens := ctx.GetInt("gpt_image_output_tokens")
	if gptImageOutputTokens > 0 {
		other["gpt_image_output_tokens"] = gptImageOutputTokens
		// 记录图片补全倍率
		if imageCompletionRatio > 0 {
			other["image_completion_ratio"] = imageCompletionRatio
		}
	}

	// 记录 Gemini 图片和文本输出 tokens 详情
	if geminiImageOutputTokens > 0 {
		other["image_output_tokens"] = geminiImageOutputTokens
		calculatedTextOutputTokens := geminiTextOutputTokens
		if calculatedTextOutputTokens == 0 {
			// 如果没有单独统计文本tokens，从总completionTokens中减去图片tokens
			calculatedTextOutputTokens = completionTokens - geminiImageOutputTokens
			if calculatedTextOutputTokens < 0 {
				calculatedTextOutputTokens = 0
			}
		}
		if calculatedTextOutputTokens > 0 {
			other["text_output_tokens"] = calculatedTextOutputTokens
		}
		// 记录图片补全倍率（如果gpt-image-1没有设置）
		if imageCompletionRatio > 0 && gptImageOutputTokens == 0 {
			other["image_completion_ratio"] = imageCompletionRatio
		}
	} else if geminiTextOutputTokens > 0 {
		// 只有文本输出时也记录
		other["text_output_tokens"] = geminiTextOutputTokens
	}

	// 计算价格链条
	var priceChain *model.PriceChainParams
	if relayInfo.PriceData.UsePrice && relayInfo.RelayMode == relayconstant.RelayModeImagesGenerations {
		// 图片生成：使用专门的价格链条计算函数
		imagePrice, hasImagePrice := ratio_setting.GetImageModelPricePerImage(modelName)
		if hasImagePrice && imagePrice > 0 {
			// 获取图片数量
			imageCount := 1
			if v, exists := ctx.Get("generated_images_count"); exists {
				if n, ok := v.(int); ok && n > 0 {
					imageCount = n
				}
			} else if usage != nil && usage.TotalTokens > 0 {
				imageCount = usage.TotalTokens
			}
			// 使用原始imagePrice（未应用系统折扣），因为价格链条计算中会应用
			priceChain = service.CalculatePriceChainForImageGeneration(ctx, logModel, imagePrice, imageCount, quota)
		} else {
			// 没有按张计费配置，使用常规价格链条计算
			priceChain = service.CalculatePriceChainForLog(ctx, logModel, promptTokens, completionTokens, quota)
		}
	} else {
		// 常规计费：使用tokens计算价格链条
		priceChain = service.CalculatePriceChainForLog(ctx, logModel, promptTokens, completionTokens, quota)
	}

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		PriceChain:       priceChain,
	})
}

// calculateImageTokenPricingQuota 计算图像Token表定价的配额
// 使用厂商返回的真实 tokens，价格从配置查表获取
func calculateImageTokenPricingQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) int {
	pricing := relayInfo.PriceData.ImageTokenPricing
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio

	// 获取OEM用户折扣
	oemUserDiscount := service.GetOemUserDiscountForQuota(ctx, relayInfo.OriginModelName)

	// 从 usage 和 context 获取真实的 tokens（不再查表）
	inputTextTokens := 0
	inputImageTokens := 0
	outputTokens := 0

	if usage != nil {
		// 输出 tokens（使用厂商返回的真实值）
		outputTokens = usage.CompletionTokens

		// 从 context 中获取 input_tokens_details
		if inputDetails, ok := ctx.Get("input_tokens_details"); ok {
			if details, ok := inputDetails.(map[string]interface{}); ok {
				if imageTokens, ok := details["image_tokens"].(int); ok {
					inputImageTokens = imageTokens
				}
				if textTokens, ok := details["text_tokens"].(int); ok {
					inputTextTokens = textTokens
				}
			}
		}

		// 备用：从 InputTokensDetails 获取
		if inputImageTokens == 0 && inputTextTokens == 0 && usage.InputTokensDetails != nil {
			inputImageTokens = usage.InputTokensDetails.ImageTokens
			inputTextTokens = usage.InputTokensDetails.TextTokens
		}

		// 最后备用：估算
		if inputImageTokens == 0 && inputTextTokens == 0 && usage.PromptTokens > 0 {
			inputTextTokens = 80
			inputImageTokens = usage.PromptTokens - inputTextTokens
		}
	}

	// 计算费用（使用真实tokens，价格从配置查表）
	dInputTextTokens := decimal.NewFromInt(int64(inputTextTokens))
	dInputImageTokens := decimal.NewFromInt(int64(inputImageTokens))
	dOutputTokens := decimal.NewFromInt(int64(outputTokens))

	dInputTextPrice := decimal.NewFromFloat(pricing.InputTextPrice)
	dInputImagePrice := decimal.NewFromFloat(pricing.InputImagePrice)
	dOutputImagePrice := decimal.NewFromFloat(pricing.OutputImagePrice)
	dOneMillion := decimal.NewFromInt(1000000)

	// 计算各部分费用
	inputTextCost := dInputTextTokens.Mul(dInputTextPrice).Div(dOneMillion)
	inputImageCost := dInputImageTokens.Mul(dInputImagePrice).Div(dOneMillion)
	// outputTokens 已经是所有输出图片的总tokens，不需要再乘以图片数量
	outputCost := dOutputTokens.Mul(dOutputImagePrice).Div(dOneMillion)

	totalCost := inputTextCost.Add(inputImageCost).Add(outputCost)
	totalCost = totalCost.Mul(decimal.NewFromFloat(groupRatio))
	// 应用OEM用户折扣
	totalCost = totalCost.Mul(decimal.NewFromFloat(oemUserDiscount))
	quota := totalCost.Mul(decimal.NewFromFloat(common.QuotaPerUnit))

	// 简化日志
	quality := ctx.GetString("image_quality")
	size := ctx.GetString("image_size")
	if common.DebugEnabled {
		logger.LogDebug(ctx, fmt.Sprintf("[ImageTokenPricing计费] %s %s | 文本=%d 图片输入=%d 输出=%d | $%.6f × oem_user_discount %.4f = quota=%d",
			quality, size, inputTextTokens, inputImageTokens, outputTokens, totalCost.InexactFloat64(), oemUserDiscount, int(quota.Round(0).IntPart())))
	}

	return int(quota.Round(0).IntPart())
}

// recordImageTokenPricingConsume 记录图像Token表定价的消费
func recordImageTokenPricingConsume(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, quota int, tokenName string) {
	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	modelName := relayInfo.OriginModelName

	// 获取图像相关信息
	quality := ctx.GetString("image_quality")
	if quality == "" {
		quality = "medium"
	}
	size := ctx.GetString("image_size")
	if size == "" {
		size = "1024x1024"
	}
	imageCount := 1
	if v, exists := ctx.Get("generated_images_count"); exists {
		if n, ok := v.(int); ok && n > 0 {
			imageCount = n
		}
	}

	logContent := fmt.Sprintf("图像Token表计费: 质量=%s, 尺寸=%s, 数量=%d张", quality, size, imageCount)

	// 更新用户和渠道配额
	if quota > 0 {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	// 计算差额
	quotaDelta := quota - relayInfo.FinalPreConsumedQuota

	if quotaDelta > 0 {
		logger.LogInfo(ctx, fmt.Sprintf("预扣费后补扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	} else if quotaDelta < 0 {
		logger.LogInfo(ctx, fmt.Sprintf("预扣费后返还扣费：%s（实际消耗：%s，预扣费：%s）",
			logger.FormatQuota(-quotaDelta),
			logger.FormatQuota(quota),
			logger.FormatQuota(relayInfo.FinalPreConsumedQuota),
		))
	}

	if quotaDelta != 0 {
		err := service.PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	// 记录消费日志
	logModel := modelName
	if relayInfo.UpstreamModelName != "" && relayInfo.UpstreamModelName != modelName {
		logModel = fmt.Sprintf("%s->%s", modelName, relayInfo.UpstreamModelName)
	}

	// 从 usage 中获取真实的 tokens
	pricing := relayInfo.PriceData.ImageTokenPricing
	inputTextTokens := 0
	inputImageTokens := 0
	outputTokens := 0

	if usage != nil {
		// 输出 tokens（使用厂商返回的真实值）
		outputTokens = usage.CompletionTokens

		// 从 context 中获取 input_tokens_details
		if inputDetails, ok := ctx.Get("input_tokens_details"); ok {
			if details, ok := inputDetails.(map[string]interface{}); ok {
				if imageTokens, ok := details["image_tokens"].(int); ok {
					inputImageTokens = imageTokens
				}
				if textTokens, ok := details["text_tokens"].(int); ok {
					inputTextTokens = textTokens
				}
			}
		}

		// 如果没有 details，尝试从 InputTokensDetails 获取（备用方案）
		if inputImageTokens == 0 && inputTextTokens == 0 && usage.InputTokensDetails != nil {
			inputImageTokens = usage.InputTokensDetails.ImageTokens
			inputTextTokens = usage.InputTokensDetails.TextTokens
		}

		// 如果还是没有，用总输入减去估算的文本tokens
		if inputImageTokens == 0 && inputTextTokens == 0 && usage.PromptTokens > 0 {
			inputTextTokens = 80 // 估算
			inputImageTokens = usage.PromptTokens - inputTextTokens
		}
	}

	// 获取输入图片数量
	inputImageCount := 0
	if v, exists := ctx.Get("input_images_count"); exists {
		if n, ok := v.(int); ok && n > 0 {
			inputImageCount = n
		}
	}

	// 计算总费用（用于 content 字段）
	inputTextCost := float64(inputTextTokens) * pricing.InputTextPrice / 1000000
	inputImageCost := float64(inputImageTokens) * pricing.InputImagePrice / 1000000
	outputCost := float64(outputTokens) * pricing.OutputImagePrice / 1000000
	totalCost := (inputTextCost + inputImageCost + outputCost) * relayInfo.PriceData.GroupRatioInfo.GroupRatio

	// 更新 logContent，包含详细的计费信息
	logContent = fmt.Sprintf("图像Token计费: 质量=%s, 尺寸=%s, 输出=%d张 | 输入文本=%dtokens 输入图片=%dtokens 输出=%dtokens | 费用=$%.6f",
		quality, size, imageCount, inputTextTokens, inputImageTokens, outputTokens, totalCost)

	// 构建详细的 other 信息
	other := make(map[string]any)
	other["image_token_pricing"] = true
	other["image_quality"] = quality
	other["image_size"] = size
	other["input_images_count"] = inputImageCount
	other["output_images_count"] = imageCount
	other["input_text_tokens"] = inputTextTokens
	other["input_image_tokens"] = inputImageTokens
	other["output_tokens"] = outputTokens
	other["input_text_price"] = pricing.InputTextPrice
	other["input_image_price"] = pricing.InputImagePrice
	other["output_image_price"] = pricing.OutputImagePrice
	other["group_ratio"] = relayInfo.PriceData.GroupRatioInfo.GroupRatio

	// 记录OEM用户折扣信息（用于溯源）
	oemUserDiscount := service.GetOemUserDiscountForQuota(ctx, modelName)
	if oemUserDiscount != 1.0 && oemUserDiscount > 0 {
		other["oem_user_discount"] = oemUserDiscount
		oemCode := "nebula"
		if code, exists := ctx.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		other["oem_code"] = oemCode
		vendorName := service.GetVendorNameFromModel(modelName)
		if vendorName != "" {
			other["vendor_name"] = vendorName
		}
	}

	// 计算价格链条（使用请求头X-Oem-Code中的OEM信息）
	priceChain := service.CalculatePriceChainForLog(ctx, logModel, usage.PromptTokens, outputTokens, quota)

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens, // 总输入tokens
		CompletionTokens: outputTokens,       // 真实输出tokens
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		PriceChain:       priceChain,
	})
}
