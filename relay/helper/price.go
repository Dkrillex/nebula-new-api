package helper

import (
	"fmt"
	"one-api/common"
	relaycommon "one-api/relay/common"
	"one-api/setting/ratio_setting"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
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
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	// 优先级1：检查图像Token表定价（gpt-image-1等特殊图像模型）
	imageTokenPricing, hasImageTokenPricing := ratio_setting.GetImageTokenPricing(info.OriginModelName)
	if hasImageTokenPricing {
		groupRatioInfo := HandleGroupRatio(c, info)
		return types.PriceData{
			UseImageTokenPricing: true,
			ImageTokenPricing:    imageTokenPricing,
			GroupRatioInfo:       groupRatioInfo,
		}, nil
	}

	// 优先级2：检查按张计费（ImageModelPricePerImage）
	imageModelPrice, hasImageModelPrice := ratio_setting.GetImageModelPricePerImage(info.OriginModelName)
	if hasImageModelPrice && imageModelPrice > 0 {
		groupRatioInfo := HandleGroupRatio(c, info)
		preConsumedQuota := int(imageModelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
		return types.PriceData{
			UsePrice:               true, // 按张计费需要设置 UsePrice = true
			ModelPrice:             imageModelPrice,
			GroupRatioInfo:         groupRatioInfo,
			ShouldPreConsumedQuota: preConsumedQuota,
		}, nil
	}

	// 优先级3：检查按次计费（ModelPrice）
	modelPrice, usePrice := ratio_setting.GetModelPrice(info.OriginModelName, false)

	groupRatioInfo := HandleGroupRatio(c, info)

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var audioRatio float64
	var audioCompletionRatio float64
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(info.OriginModelName)
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
		completionRatio = ratio_setting.GetCompletionRatio(info.OriginModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(info.OriginModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(info.OriginModelName)
		imageRatio, _ = ratio_setting.GetImageRatio(info.OriginModelName)
		audioRatio = ratio_setting.GetAudioRatio(info.OriginModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(info.OriginModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		preConsumedQuota = int(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	}

	priceData := types.PriceData{
		ModelPrice:             modelPrice,
		ModelRatio:             modelRatio,
		CompletionRatio:        completionRatio,
		GroupRatioInfo:         groupRatioInfo,
		UsePrice:               usePrice,
		CacheRatio:             cacheRatio,
		ImageRatio:             imageRatio,
		AudioRatio:             audioRatio,
		AudioCompletionRatio:   audioCompletionRatio,
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
	quota := int(modelPrice * common.QuotaPerUnit * groupRatioInfo.GroupRatio)
	priceData := types.PerCallPriceData{
		ModelPrice:     modelPrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
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
