package helper

import (
	"fmt"
	"one-api/common"
	"one-api/constant"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/service"
	"one-api/setting/ratio_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
// 优先级：用户特殊分组倍率 > OEM特定GroupRatio > 全局GroupRatio > 默认值1.0
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group - handle nil context
	if ctx != nil {
		autoGroup, exists := ctx.Get("auto_group")
		if exists {
			if common.DebugEnabled {
				println(fmt.Sprintf("final group: %s", autoGroup))
			}
			relayInfo.UsingGroup = autoGroup.(string)
		}
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// 优先使用OEM特定的GroupRatio
		groupRatioInfo.GroupRatio = service.GetGroupRatioByOemFromContext(ctx, relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	// 优先级1：检查图像Token表定价（排除 gpt-image-1，让它使用倍率计费）
	if !strings.HasPrefix(info.OriginModelName, "gpt-image-1") {
		imageTokenPricing, hasImageTokenPricing := ratio_setting.GetImageTokenPricing(info.OriginModelName)
		if hasImageTokenPricing {
			groupRatioInfo := HandleGroupRatio(c, info)
			return types.PriceData{
				UseImageTokenPricing: true,
				ImageTokenPricing:    imageTokenPricing,
				GroupRatioInfo:       groupRatioInfo,
			}, nil
		}
	}

	// 优先级2：检查按张计费（ImageModelPricePerImage）
	imageModelPrice, hasImageModelPrice := ratio_setting.GetImageModelPricePerImage(info.OriginModelName)
	if hasImageModelPrice && imageModelPrice > 0 {
		officialImageModelPrice := imageModelPrice // 记录原始价格
		// 应用OEM用户折扣到 imageModelPrice（oem_user_discount）
		if c != nil {
			oemCode := "nebula" // 默认系统
			if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
			vendorName := service.GetVendorNameFromModel(info.OriginModelName)
			oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, info.OriginModelName, vendorName)
			if oemUserDiscount != 1.0 {
				imageModelPrice = imageModelPrice * oemUserDiscount
				if common.DebugEnabled {
					println(fmt.Sprintf("[ModelPriceHelper] 应用OEM用户折扣到imageModelPrice: oemCode=%s, modelName=%s, oemUserDiscount=%.4f, 原价=%.4f, 折后价=%.4f",
						oemCode, info.OriginModelName, oemUserDiscount, imageModelPrice/oemUserDiscount, imageModelPrice))
				}
			}
		}
		groupRatioInfo := HandleGroupRatio(c, info)
		preConsumedQuota := int(imageModelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)

		// 计算OEM平台价格（原厂价格 * OEM折扣）
		var oemImageModelPrice float64
		if c != nil {
			oemCode := "nebula" // 默认系统
			if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
			vendorName := service.GetVendorNameFromModel(info.OriginModelName)
			oemDiscount := model.GetOemDiscountByCode(oemCode, info.OriginModelName, vendorName)
			if oemDiscount <= 0 {
				oemDiscount = 1.0
			}
			oemImageModelPrice = officialImageModelPrice * oemDiscount
		} else {
			oemImageModelPrice = officialImageModelPrice
		}

		return types.PriceData{
			UsePrice:                   true,            // 按张计费需要设置 UsePrice = true
			ModelPrice:                 imageModelPrice, // 已应用OEM用户折扣
			OfficialModelPrice:         officialImageModelPrice,
			OemModelPrice:              oemImageModelPrice,
			ImagePricePerImage:         imageModelPrice, // 用户使用的图片每张价格
			OfficialImagePricePerImage: officialImageModelPrice,
			OemImagePricePerImage:      oemImageModelPrice,
			GroupRatioInfo:             groupRatioInfo,
			ShouldPreConsumedQuota:     preConsumedQuota,
		}, nil
	}

	// 优先级3：检查按次计费（ModelPrice）
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)
	officialModelPrice := modelPrice // 记录原始价格

	// 应用OEM用户折扣到 modelPrice（oem_user_discount）
	if c != nil && usePrice && modelPrice > 0 {
		oemCode := "nebula" // 默认系统
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		vendorName := service.GetVendorNameFromModel(info.OriginModelName)
		oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, info.OriginModelName, vendorName)
		if common.DebugEnabled {
			println(fmt.Sprintf("[ModelPriceHelper] 查询OEM用户折扣: oemCode=%s, modelName=%s, vendorName=%s, oemUserDiscount=%.4f",
				oemCode, info.OriginModelName, vendorName, oemUserDiscount))
		}
		if oemUserDiscount != 1.0 && oemUserDiscount > 0 {
			modelPrice = modelPrice * oemUserDiscount
			if common.DebugEnabled {
				println(fmt.Sprintf("[ModelPriceHelper] 应用OEM用户折扣到modelPrice: oemCode=%s, modelName=%s, oemUserDiscount=%.4f, 原价=%.4f, 折后价=%.4f",
					oemCode, info.OriginModelName, oemUserDiscount, officialModelPrice, modelPrice))
			}
		}
	}

	groupRatioInfo := HandleGroupRatio(c, info)

	var preConsumedQuota int
	var modelRatio float64
	var officialModelRatio float64 // 记录原始倍率（应用OEM折扣前）
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var officialImageRatio float64
	var cacheCreationRatio float64
	var audioRatio float64
	var officialAudioRatio float64
	var audioCompletionRatio float64
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
		officialModelRatio = modelRatio // 记录原始倍率
		if !success {
			// 检查是否配置了视频每秒价格（第三种定价方式）
			_, hasVideoPrice := ratio_setting.GetVideoModelPricePerSecond(info.OriginModelName)

			// 对于 wan2.5 系列模型（i2v 和 t2v），还需要检查按分辨率定价
			if !hasVideoPrice && (info.OriginModelName == "wan2.5-i2v-preview" || info.OriginModelName == "wan2.5-t2v-preview") {
				// 使用默认分辨率 720p 检查是否有价格配置
				_, hasVideoPrice = ratio_setting.GetVideoModelPriceByResolution(info.OriginModelName, "720p")
			}

			// 检查是否配置了按张计费价格（第五种定价方式：图片生成模型）
			_, hasImagePricePerImage := ratio_setting.GetImageModelPricePerImage(info.OriginModelName)

			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}

			// 只有当五种定价方式（ImageTokenPricing、VideoModelPricePerSecond、ImageModelPricePerImage、ModelPrice、ModelRatio）都没有配置时才报错
			if !acceptUnsetRatio && !hasVideoPrice && !hasImagePricePerImage {
				return types.PriceData{}, fmt.Errorf("模型 %s 倍率或价格未配置，请联系管理员设置或开始自用模式；Model %s ratio or price not set, please set or start self-use mode", matchName, matchName)
			}
		}

		// 应用OEM用户折扣到 modelRatio（oem_user_discount）
		if c != nil && success {
			oemCode := "nebula" // 默认系统
			if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
			vendorName := service.GetVendorNameFromModel(info.OriginModelName)
			oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, info.OriginModelName, vendorName)
			if oemUserDiscount != 1.0 {
				modelRatio = modelRatio * oemUserDiscount
				if common.DebugEnabled {
					println(fmt.Sprintf("[ModelPriceHelper] 应用OEM用户折扣: oemCode=%s, modelName=%s, oemUserDiscount=%.4f, 原倍率=%.4f, 折后倍率=%.4f",
						oemCode, info.OriginModelName, oemUserDiscount, modelRatio/oemUserDiscount, modelRatio))
				}
			}
		}

		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		// 获取原始图片倍率和音频倍率
		officialImageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		officialAudioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		imageRatio = officialImageRatio // 用户侧倍率（后续应用 oem_user_discount）
		audioRatio = officialAudioRatio // 用户侧倍率（后续应用 oem_user_discount）
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)

		// 应用OEM用户折扣到 imageRatio / audioRatio（逻辑与 modelRatio 一致）
		// 目的：用户实际扣费/账单展示使用折后倍率；原始倍率保留在 official_* 字段里
		if c != nil {
			oemCode := "nebula" // 默认系统
			if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
			vendorName := service.GetVendorNameFromModel(info.OriginModelName)
			oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, info.OriginModelName, vendorName)
			if oemUserDiscount != 1.0 && oemUserDiscount > 0 {
				imageRatio = imageRatio * oemUserDiscount
				audioRatio = audioRatio * oemUserDiscount
			}
		}

		ratio := modelRatio * groupRatioInfo.GroupRatio
		preConsumedQuota = int(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	}

	imageCompletionRatio := ratio_setting.GetImageCompletionRatio(info.OriginModelName)

	// 计算OEM平台价格和倍率（原厂价格/倍率 * OEM折扣）
	var oemModelPrice float64
	var oemModelRatio float64
	var oemImageRatio float64
	var oemAudioRatio float64
	if c != nil {
		oemCode := "nebula" // 默认系统
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		vendorName := service.GetVendorNameFromModel(info.OriginModelName)
		oemDiscount := model.GetOemDiscountByCode(oemCode, info.OriginModelName, vendorName)
		if oemDiscount <= 0 {
			oemDiscount = 1.0
		}
		// OEM平台价格 = 原厂价格 * OEM折扣
		oemModelPrice = officialModelPrice * oemDiscount
		// OEM平台倍率 = 原厂倍率 * OEM折扣
		oemModelRatio = officialModelRatio * oemDiscount
		// OEM平台图片倍率 = 原厂图片倍率 * OEM折扣
		oemImageRatio = officialImageRatio * oemDiscount
		// OEM平台音频倍率 = 原厂音频倍率 * OEM折扣
		oemAudioRatio = officialAudioRatio * oemDiscount
	} else {
		// 如果没有context，使用原厂价格和倍率
		oemModelPrice = officialModelPrice
		oemModelRatio = officialModelRatio
		oemImageRatio = officialImageRatio
		oemAudioRatio = officialAudioRatio
	}

	priceData := types.PriceData{
		ModelPrice:             modelPrice,
		ModelRatio:             modelRatio,
		OfficialModelPrice:     officialModelPrice,
		OfficialModelRatio:     officialModelRatio,
		OemModelPrice:          oemModelPrice,
		OemModelRatio:          oemModelRatio,
		CompletionRatio:        completionRatio,
		GroupRatioInfo:         groupRatioInfo,
		UsePrice:               usePrice,
		CacheRatio:             cacheRatio,
		ImageRatio:             imageRatio,
		OfficialImageRatio:     officialImageRatio,
		OemImageRatio:          oemImageRatio,
		AudioRatio:             audioRatio,
		OfficialAudioRatio:     officialAudioRatio,
		OemAudioRatio:          oemAudioRatio,
		AudioCompletionRatio:   audioCompletionRatio,
		ImageCompletionRatio:   imageCompletionRatio,
		CacheCreationRatio:     cacheCreationRatio,
		ShouldPreConsumedQuota: preConsumedQuota,
	}

	if common.DebugEnabled {
		println(fmt.Sprintf("model_price_helper result: %s", priceData.ToSetting()))
	}
	info.PriceData = priceData
	return priceData, nil
}

// ModelPriceHelperPerCall 按次计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) types.PerCallPriceData {
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(info.OriginModelName, true)
	// 如果没有配置价格，则使用默认价格
	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[info.OriginModelName]
		if !ok {
			modelPrice = 0.1
		} else {
			modelPrice = defaultPrice
		}
	}
	officialModelPrice := modelPrice // 记录原始价格

	// 计算OEM平台价格（原厂价格 * OEM折扣）
	var oemModelPrice float64
	oemCode := "nebula" // 默认系统
	if c != nil {
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		vendorName := service.GetVendorNameFromModel(info.OriginModelName)
		oemDiscount := model.GetOemDiscountByCode(oemCode, info.OriginModelName, vendorName)
		if oemDiscount <= 0 {
			oemDiscount = 1.0
		}
		oemModelPrice = officialModelPrice * oemDiscount

		// 应用OEM用户折扣到 modelPrice（oem_user_discount）
		oemUserDiscount := model.GetOemUserDiscountByCode(oemCode, info.OriginModelName, vendorName)
		if oemUserDiscount != 1.0 {
			modelPrice = modelPrice * oemUserDiscount
			if common.DebugEnabled {
				println(fmt.Sprintf("[ModelPriceHelperPerCall] 应用OEM用户折扣到modelPrice: oemCode=%s, modelName=%s, oemUserDiscount=%.4f, 原价=%.4f, 折后价=%.4f",
					oemCode, info.OriginModelName, oemUserDiscount, officialModelPrice, modelPrice))
			}
		}
	} else {
		oemModelPrice = officialModelPrice
	}

	quota := int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	priceData := types.PerCallPriceData{
		ModelPrice:                 modelPrice,
		OfficialModelPrice:         officialModelPrice,
		OemModelPrice:              oemModelPrice,
		OfficialImagePricePerImage: officialModelPrice, // 对于按次计费的图片任务，可以视为每次=每张
		OemImagePricePerImage:      oemModelPrice,
		Quota:                      quota,
		GroupRatioInfo:             groupRatioInfo,
	}
	return priceData
}

func ContainPriceOrRatio(modelName string) bool {
	// 检查 ModelPrice（按次计费）
	_, ok := ratio_setting.GetModelPrice(modelName, false)
	if ok {
		return true
	}
	// 检查 ModelRatio（按token计费）
	_, ok, _ = ratio_setting.GetModelRatio(modelName)
	if ok {
		return true
	}
	// 检查 VideoModelPricePerSecond（视频按秒计费）
	_, ok = ratio_setting.GetVideoModelPricePerSecond(modelName)
	if ok {
		return true
	}
	// 对于 wan2.5 系列模型（i2v 和 t2v），检查按分辨率定价
	if modelName == "wan2.5-i2v-preview" || modelName == "wan2.5-t2v-preview" {
		_, ok = ratio_setting.GetVideoModelPriceByResolution(modelName, "720p")
		if ok {
			return true
		}
	}
	return false
}
