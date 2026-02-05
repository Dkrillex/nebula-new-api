package service

import (
	"errors"
	"fmt"
	"log"
	"math"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/setting/ratio_setting"
	"one-api/setting/system_setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails    TokenDetails
	OutputDetails   TokenDetails
	ModelName       string
	UsePrice        bool
	ModelPrice      float64
	ModelRatio      float64
	GroupRatio      float64
	OemUserDiscount float64 // OEM用户折扣
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) int {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		// 注意：modelPrice 应该已经在调用方应用了 oemUserDiscount
		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return int(quota.IntPart())
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)

	// 防止倍率为 0 导致计费失败，使用默认值
	if groupRatio.IsZero() {
		groupRatio = decimal.NewFromInt(1)
		common.SysLog(fmt.Sprintf("[calculateAudioQuota] groupRatio is zero, using default 1.0 for model %s", info.ModelName))
	}
	if modelRatio.IsZero() {
		// gpt-realtime 系列使用较高的默认倍率
		if strings.Contains(strings.ToLower(info.ModelName), "realtime") {
			modelRatio = decimal.NewFromFloat(15.0)
		} else {
			modelRatio = decimal.NewFromFloat(37.5)
		}
		common.SysLog(fmt.Sprintf("[calculateAudioQuota] modelRatio is zero, using default %.1f for model %s", modelRatio.InexactFloat64(), info.ModelName))
	}

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero

	// 文本输入和输出使用 modelRatio 作为基础
	// 注意：modelRatio 应该已经在调用方应用了 oemUserDiscount
	textRatio := groupRatio.Mul(modelRatio)
	quota = quota.Add(inputTextTokens.Mul(textRatio))
	quota = quota.Add(outputTextTokens.Mul(completionRatio).Mul(textRatio))

	// 音频输入和输出使用独立的绝对倍率（不乘以 modelRatio）
	// audioRatio 是音频输入的绝对倍率
	// audioCompletionRatio 是音频输出相对于音频输入的倍率
	// 注意：音频倍率需要应用 oemUserDiscount
	dOemUserDiscount := decimal.NewFromFloat(info.OemUserDiscount)
	if dOemUserDiscount.IsZero() {
		dOemUserDiscount = decimal.NewFromInt(1)
	}
	quota = quota.Add(inputAudioTokens.Mul(audioRatio).Mul(groupRatio).Mul(dOemUserDiscount))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio).Mul(groupRatio).Mul(dOemUserDiscount))

	// If quota is less than or equal to zero, set quota to 1
	if quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return int(quota.Round(0).IntPart())
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	userQuota, err := model.GetUserQuota(relayInfo.UserId, false)
	if err != nil {
		return err
	}

	token, err := model.GetTokenByKey(strings.TrimLeft(relayInfo.TokenKey, "sk-"), false)
	if err != nil {
		return err
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	groupRatio := ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)

	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		groupRatio = ratio_setting.GetGroupRatio(autoGroup.(string))
		log.Printf("final group ratio: %f", groupRatio)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	actualGroupRatio := groupRatio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		actualGroupRatio = userGroupRatio
	}

	// 获取OEM用户折扣并应用到 modelRatio
	oemUserDiscount := GetOemUserDiscountForQuota(ctx, modelName)
	discountedModelRatio := modelRatio * oemUserDiscount

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:       modelName,
		UsePrice:        relayInfo.UsePrice,
		ModelRatio:      discountedModelRatio, // 已应用 oemUserDiscount
		GroupRatio:      actualGroupRatio,
		OemUserDiscount: oemUserDiscount, // 用于音频倍率
	}

	quota := calculateAudioQuota(quotaInfo)

	if userQuota < quota {
		return fmt.Errorf("user quota is not enough, user quota: %s, need quota: %s", logger.FormatQuota(userQuota), logger.FormatQuota(quota))
	}

	if !token.UnlimitedQuota && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}

	err = PostConsumeQuota(relayInfo, quota, 0, false)
	if err != nil {
		return err
	}
	logger.LogInfo(ctx, "realtime streaming consume quota success, quota: "+fmt.Sprintf("%d", quota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	// 获取OEM用户折扣
	oemUserDiscount := GetOemUserDiscountForQuota(ctx, modelName)

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:       modelName,
		UsePrice:        usePrice,
		ModelRatio:      modelRatio,
		GroupRatio:      groupRatio,
		OemUserDiscount: oemUserDiscount,
	}

	quota := calculateAudioQuota(quotaInfo)

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
		// 如果没有使用量，不记录日志
		return
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)

	// 计算价格链条（优先使用用户oem_id，fallback到请求头X-Oem-Code）
	priceChain := CalculatePriceChainForLog(ctx, logModel, textInputTokens+audioInputTokens, textOutTokens+audioOutTokens, quota)

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     textInputTokens,
		CompletionTokens: textOutTokens,
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

func PostClaudeConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage) {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	promptTokens := usage.PromptTokens
	completionTokens := usage.CompletionTokens
	modelName := relayInfo.OriginModelName

	tokenName := ctx.GetString("token_name")
	completionRatio := relayInfo.PriceData.CompletionRatio
	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	cacheRatio := relayInfo.PriceData.CacheRatio
	cacheTokens := usage.PromptTokensDetails.CachedTokens

	cacheCreationRatio := relayInfo.PriceData.CacheCreationRatio
	cacheCreationTokens := usage.PromptTokensDetails.CachedCreationTokens

	// 上游返回的 input_tokens / prompt_tokens 已包含 cache_read、cache_creation，需先扣减再按缓存倍率计费，避免重复计费
	if relayInfo.ChannelType == constant.ChannelTypeOpenRouter {
		promptTokens -= cacheTokens
		isUsingCustomSettings := relayInfo.PriceData.UsePrice || hasCustomModelRatio(modelName, relayInfo.PriceData.ModelRatio)
		if cacheCreationTokens == 0 && relayInfo.PriceData.CacheCreationRatio != 1 && usage.Cost != 0 && !isUsingCustomSettings {
			maybeCacheCreationTokens := CalcOpenRouterCacheCreateTokens(*usage, relayInfo.PriceData)
			if maybeCacheCreationTokens >= 0 && promptTokens >= maybeCacheCreationTokens {
				cacheCreationTokens = maybeCacheCreationTokens
			}
		}
		promptTokens -= cacheCreationTokens
	} else if relayInfo.ChannelType == constant.ChannelTypeAnthropic || relayInfo.ChannelType == constant.ChannelTypeAws {
		if cacheTokens > 0 && promptTokens >= cacheTokens {
			promptTokens -= cacheTokens
		}
		if cacheCreationTokens > 0 && promptTokens >= cacheCreationTokens {
			promptTokens -= cacheCreationTokens
		}
	}

	calculateQuota := 0.0
	if !relayInfo.PriceData.UsePrice {
		calculateQuota = float64(promptTokens)
		calculateQuota += float64(cacheTokens) * cacheRatio
		calculateQuota += float64(cacheCreationTokens) * cacheCreationRatio
		calculateQuota += float64(completionTokens) * completionRatio
		// 注意：modelRatio 已在 price.go 的 ModelPriceHelper 中应用了 oemUserDiscount
		// 所以这里不需要再乘以 oemUserDiscount，否则会重复应用折扣
		calculateQuota = calculateQuota * groupRatio * modelRatio
	} else {
		// 注意：modelPrice 已在 price.go 的 ModelPriceHelper 中应用了 oemUserDiscount
		calculateQuota = modelPrice * common.QuotaPerUnit * groupRatio
	}

	if modelRatio != 0 && calculateQuota <= 0 {
		calculateQuota = 1
	}

	quota := int(calculateQuota)

	totalTokens := promptTokens + completionTokens

	var logContent string
	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游出错）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

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
		err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	other := GenerateClaudeOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio,
		cacheTokens, cacheRatio, cacheCreationTokens, cacheCreationRatio, modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)

	// 计算价格链条（使用请求头X-Oem-Code中的OEM信息）
	priceChain := CalculatePriceChainForLog(ctx, modelName, promptTokens, completionTokens, quota)

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		ModelName:        modelName,
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

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) {

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	// 获取OEM用户折扣
	oemUserDiscount := GetOemUserDiscountForQuota(ctx, relayInfo.OriginModelName)

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:       relayInfo.OriginModelName,
		UsePrice:        usePrice,
		ModelRatio:      modelRatio,
		GroupRatio:      groupRatio,
		OemUserDiscount: oemUserDiscount,
	}

	quota := calculateAudioQuota(quotaInfo)

	totalTokens := usage.TotalTokens
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	// record all the consume log even if quota is 0
	if totalTokens == 0 {
		// in this case, must be some error happened
		// we cannot just return, because we may have to return the pre-consumed quota
		quota = 0
		logContent += fmt.Sprintf("（可能是上游超时）")
		logger.LogError(ctx, fmt.Sprintf("total tokens is 0, cannot consume quota, userId %d, channelId %d, "+
			"tokenId %d, model %s， pre-consumed quota %d", relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	} else {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, quota)
	}

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
		err := PostConsumeQuota(relayInfo, quotaDelta, relayInfo.FinalPreConsumedQuota, true)
		if err != nil {
			logger.LogError(ctx, "error consuming token remain quota: "+err.Error())
		}
	}

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)

	// 计算价格链条（优先使用用户oem_id，fallback到请求头X-Oem-Code）
	priceChain := CalculatePriceChainForLog(ctx, logModel, textInputTokens+audioInputTokens, textOutTokens+audioOutTokens, quota)

	model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     textInputTokens,
		CompletionTokens: textOutTokens,
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

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	//if relayInfo.TokenUnlimited {
	//	return nil
	//}
	token, err := model.GetTokenByKey(relayInfo.TokenKey, false)
	if err != nil {
		return err
	}
	if !relayInfo.TokenUnlimited && token.RemainQuota < quota {
		return fmt.Errorf("token quota is not enough, token remain quota: %s, need quota: %s", logger.FormatQuota(token.RemainQuota), logger.FormatQuota(quota))
	}
	err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
	if err != nil {
		return err
	}
	return nil
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) (err error) {
	// 注意：价格链条计算和系统账户补贴处理在RecordConsumeLog中进行
	// 这里只处理基本的quota扣费

	if quota > 0 {
		err = model.DecreaseUserQuota(relayInfo.UserId, quota)
	} else {
		err = model.IncreaseUserQuota(relayInfo.UserId, -quota, false)
	}
	if err != nil {
		return err
	}

	if !relayInfo.IsPlayground {
		if quota > 0 {
			err = model.DecreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.IncreaseTokenQuota(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			return err
		}
	}

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return nil
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := fmt.Sprintf("%s/topup", system_setting.ServerAddress)

			// 根据通知方式生成不同的内容格式
			var content string
			var values []interface{}

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}

			if notifyType == dto.NotifyTypeBark {
				// Bark推送使用简短文本，不支持HTML
				content = "{{value}}，剩余额度：{{value}}，请及时充值"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else if notifyType == dto.NotifyTypeGotify {
				content = "{{value}}，当前剩余额度为 {{value}}，请及时充值。"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota)}
			} else {
				// 默认内容格式，适用于Email和Webhook（支持HTML）
				content = "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>"
				values = []interface{}{prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink, topUpLink}
			}

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
	})
}
