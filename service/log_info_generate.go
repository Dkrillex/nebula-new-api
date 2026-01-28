package service

import (
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_ratio"] = modelRatio
	other["group_ratio"] = groupRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	other["user_group_ratio"] = userGroupRatio
	// 记录原始价格和倍率（应用OEM折扣前），用于原厂计费过程展示
	if relayInfo != nil && relayInfo.PriceData.OfficialModelPrice > 0 {
		other["official_model_price"] = relayInfo.PriceData.OfficialModelPrice
	}
	if relayInfo != nil && relayInfo.PriceData.OfficialModelRatio > 0 {
		other["official_model_ratio"] = relayInfo.PriceData.OfficialModelRatio
	}
	// 记录OEM平台价格和倍率（原厂价格/倍率 * OEM折扣），用于OEM平台计费过程展示
	if relayInfo != nil && relayInfo.PriceData.OemModelPrice > 0 {
		other["oem_model_price"] = relayInfo.PriceData.OemModelPrice
	}
	if relayInfo != nil && relayInfo.PriceData.OemModelRatio > 0 {
		other["oem_model_ratio"] = relayInfo.PriceData.OemModelRatio
	}
	// 记录图片倍率的三层价格体系
	if relayInfo != nil && relayInfo.PriceData.OfficialImageRatio > 0 {
		other["official_image_ratio"] = relayInfo.PriceData.OfficialImageRatio
	}
	if relayInfo != nil && relayInfo.PriceData.OemImageRatio > 0 {
		other["oem_image_ratio"] = relayInfo.PriceData.OemImageRatio
	}
	// 记录音频倍率的三层价格体系
	if relayInfo != nil && relayInfo.PriceData.OfficialAudioRatio > 0 {
		other["official_audio_ratio"] = relayInfo.PriceData.OfficialAudioRatio
	}
	if relayInfo != nil && relayInfo.PriceData.OemAudioRatio > 0 {
		other["oem_audio_ratio"] = relayInfo.PriceData.OemAudioRatio
	}
	// 记录视频每秒价格的三层价格体系
	if relayInfo != nil && relayInfo.PriceData.OfficialVideoPricePerSecond > 0 {
		other["official_video_price_per_second"] = relayInfo.PriceData.OfficialVideoPricePerSecond
	}
	if relayInfo != nil && relayInfo.PriceData.OemVideoPricePerSecond > 0 {
		other["oem_video_price_per_second"] = relayInfo.PriceData.OemVideoPricePerSecond
	}
	// 记录图片每张价格的三层价格体系
	if relayInfo != nil && relayInfo.PriceData.OfficialImagePricePerImage > 0 {
		other["official_image_price_per_image"] = relayInfo.PriceData.OfficialImagePricePerImage
	}
	if relayInfo != nil && relayInfo.PriceData.OemImagePricePerImage > 0 {
		other["oem_image_price_per_image"] = relayInfo.PriceData.OemImagePricePerImage
	}
	other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	if relayInfo.ReasoningEffort != "" {
		other["reasoning_effort"] = relayInfo.ReasoningEffort
	}
	if relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}

	// 记录OEM用户折扣信息和厂商名称（用于溯源和导出）
	if ctx != nil {
		oemCode := "nebula" // 默认系统
		if code, exists := ctx.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		vendorName := GetVendorNameFromModel(relayInfo.OriginModelName)
		// 总是记录厂商名称，用于导出
		if vendorName != "" {
			other["vendor_name"] = vendorName
		}
		oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, relayInfo.OriginModelName, vendorName)
		// 只有当折扣不是1.0时才记录折扣信息，避免日志冗余
		if oemUserDiscount != 1.0 && oemUserDiscount > 0 {
			other["oem_user_discount"] = oemUserDiscount
			other["oem_code"] = oemCode
		}
	}

	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}
	other["admin_info"] = adminInfo
	return other
}

func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["ws"] = true
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["audio"] = true
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, cacheCreationTokens int, cacheCreationRatio float64, modelPrice float64, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio)
	info["claude"] = true
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	return info
}

func GenerateMjOtherInfo(ctx *gin.Context, modelName string, priceData types.PerCallPriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	other["group_ratio"] = priceData.GroupRatioInfo.GroupRatio
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
	}
	// 记录原始价格（应用OEM折扣前），用于原厂计费过程展示
	if priceData.OfficialModelPrice > 0 {
		other["official_model_price"] = priceData.OfficialModelPrice
	}
	// 记录OEM平台价格（原厂价格 * OEM折扣），用于OEM平台计费过程展示
	if priceData.OemModelPrice > 0 {
		other["oem_model_price"] = priceData.OemModelPrice
	}
	// 记录图片每张价格的三层价格体系
	if priceData.OfficialImagePricePerImage > 0 {
		other["official_image_price_per_image"] = priceData.OfficialImagePricePerImage
	}
	if priceData.OemImagePricePerImage > 0 {
		other["oem_image_price_per_image"] = priceData.OemImagePricePerImage
	}

	// 记录OEM用户折扣信息和厂商名称（用于溯源和导出）
	if ctx != nil {
		oemCode := "nebula" // 默认系统
		if code, exists := ctx.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		vendorName := GetVendorNameFromModel(modelName)
		// 总是记录厂商名称，用于导出
		if vendorName != "" {
			other["vendor_name"] = vendorName
		}
		oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, modelName, vendorName)
		// 只有当折扣不是1.0时才记录折扣信息，避免日志冗余
		if oemUserDiscount != 1.0 && oemUserDiscount > 0 {
			other["oem_user_discount"] = oemUserDiscount
			other["oem_code"] = oemCode
		}
	}

	return other
}
