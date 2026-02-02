package controller

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/model"
	"one-api/relay"
	"one-api/relay/channel/ai360"
	"one-api/relay/channel/lingyiwanwu"
	"one-api/relay/channel/minimax"
	"one-api/relay/channel/moonshot"
	relaycommon "one-api/relay/common"
	"one-api/setting"
	"one-api/setting/ratio_setting"
	"strings"
	"time"
)

// https://platform.openai.com/docs/api-reference/models/list

var openAIModels []dto.OpenAIModels
var openAIModelsMap map[string]dto.OpenAIModels
var channelId2Models map[int][]string

func init() {
	// https://platform.openai.com/docs/models/model-endpoint-compatibility
	for i := 0; i < constant.APITypeDummy; i++ {
		if i == constant.APITypeAIProxyLibrary {
			continue
		}
		adaptor := relay.GetAdaptor(i)
		channelName := adaptor.GetChannelName()
		modelNames := adaptor.GetModelList()
		for _, modelName := range modelNames {
			openAIModels = append(openAIModels, dto.OpenAIModels{
				Id:      modelName,
				Object:  "model",
				Created: 1626777600,
				OwnedBy: channelName,
			})
		}
	}
	for _, modelName := range ai360.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: ai360.ChannelName,
		})
	}
	for _, modelName := range moonshot.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: moonshot.ChannelName,
		})
	}
	for _, modelName := range lingyiwanwu.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: lingyiwanwu.ChannelName,
		})
	}
	for _, modelName := range minimax.ModelList {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: minimax.ChannelName,
		})
	}
	for modelName, _ := range constant.MidjourneyModel2Action {
		openAIModels = append(openAIModels, dto.OpenAIModels{
			Id:      modelName,
			Object:  "model",
			Created: 1626777600,
			OwnedBy: "midjourney",
		})
	}
	openAIModelsMap = make(map[string]dto.OpenAIModels)
	for _, aiModel := range openAIModels {
		openAIModelsMap[aiModel.Id] = aiModel
	}
	channelId2Models = make(map[int][]string)
	for i := 1; i <= constant.ChannelTypeDummy; i++ {
		apiType, success := common.ChannelType2APIType(i)
		if !success || apiType == constant.APITypeAIProxyLibrary {
			continue
		}
		meta := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: i,
		}}
		adaptor := relay.GetAdaptor(apiType)
		adaptor.Init(meta)
		channelId2Models[i] = adaptor.GetModelList()
	}
	openAIModels = lo.UniqBy(openAIModels, func(m dto.OpenAIModels) string {
		return m.Id
	})
}

func ListModels(c *gin.Context, modelType int) {
	userOpenAiModels := make([]dto.OpenAIModels, 0)

	modelLimitEnable := common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled)
	if modelLimitEnable {
		s, ok := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
		var tokenModelLimit map[string]bool
		if ok {
			tokenModelLimit = s.(map[string]bool)
		} else {
			tokenModelLimit = map[string]bool{}
		}
		for allowModel, _ := range tokenModelLimit {
			if oaiModel, ok := openAIModelsMap[allowModel]; ok {
				oaiModel.SupportedEndpointTypes = model.GetModelSupportEndpointTypes(allowModel)
				userOpenAiModels = append(userOpenAiModels, oaiModel)
			} else {
				userOpenAiModels = append(userOpenAiModels, dto.OpenAIModels{
					Id:                     allowModel,
					Object:                 "model",
					Created:                1626777600,
					OwnedBy:                "custom",
					SupportedEndpointTypes: model.GetModelSupportEndpointTypes(allowModel),
				})
			}
		}
	} else {
		userId := c.GetInt("id")
		userGroup, err := model.GetUserGroup(userId, false)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "get user group failed",
			})
			return
		}
		group := userGroup
		tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
		if tokenGroup != "" {
			group = tokenGroup
		}
		var models []string
		if tokenGroup == "auto" {
			for _, autoGroup := range setting.AutoGroups {
				groupModels := model.GetGroupEnabledModels(autoGroup)
				for _, g := range groupModels {
					if !common.StringsContains(models, g) {
						models = append(models, g)
					}
				}
			}
		} else {
			models = model.GetGroupEnabledModels(group)
		}
		for _, modelName := range models {
			if oaiModel, ok := openAIModelsMap[modelName]; ok {
				oaiModel.SupportedEndpointTypes = model.GetModelSupportEndpointTypes(modelName)
				userOpenAiModels = append(userOpenAiModels, oaiModel)
			} else {
				userOpenAiModels = append(userOpenAiModels, dto.OpenAIModels{
					Id:                     modelName,
					Object:                 "model",
					Created:                1626777600,
					OwnedBy:                "custom",
					SupportedEndpointTypes: model.GetModelSupportEndpointTypes(modelName),
				})
			}
		}
	}
	switch modelType {
	case constant.ChannelTypeAnthropic:
		useranthropicModels := make([]dto.AnthropicModel, len(userOpenAiModels))
		for i, model := range userOpenAiModels {
			useranthropicModels[i] = dto.AnthropicModel{
				ID:          model.Id,
				CreatedAt:   time.Unix(int64(model.Created), 0).UTC().Format(time.RFC3339),
				DisplayName: model.Id,
				Type:        "model",
			}
		}
		c.JSON(200, gin.H{
			"data":     useranthropicModels,
			"first_id": useranthropicModels[0].ID,
			"has_more": false,
			"last_id":  useranthropicModels[len(useranthropicModels)-1].ID,
		})
	case constant.ChannelTypeGemini:
		userGeminiModels := make([]dto.GeminiModel, len(userOpenAiModels))
		for i, model := range userOpenAiModels {
			userGeminiModels[i] = dto.GeminiModel{
				Name:        model.Id,
				DisplayName: model.Id,
			}
		}
		c.JSON(200, gin.H{
			"models":        userGeminiModels,
			"nextPageToken": nil,
		})
	default:
		c.JSON(200, gin.H{
			"success": true,
			"data":    userOpenAiModels,
			"object":  "list",
		})
	}
}

func ChannelListModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    openAIModels,
	})
}

func DashboardListModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    channelId2Models,
	})
}

func EnabledListModels(c *gin.Context) {
	c.JSON(200, gin.H{
		"success": true,
		"data":    model.GetEnabledModels(),
	})
}

func RetrieveModel(c *gin.Context, modelType int) {
	modelId := c.Param("model")
	if aiModel, ok := openAIModelsMap[modelId]; ok {
		switch modelType {
		case constant.ChannelTypeAnthropic:
			c.JSON(200, dto.AnthropicModel{
				ID:          aiModel.Id,
				CreatedAt:   time.Unix(int64(aiModel.Created), 0).UTC().Format(time.RFC3339),
				DisplayName: aiModel.Id,
				Type:        "model",
			})
		default:
			c.JSON(200, aiModel)
		}
	} else {
		openAIError := dto.OpenAIError{
			Message: fmt.Sprintf("The model '%s' does not exist", modelId),
			Type:    "invalid_request_error",
			Param:   "model",
			Code:    "model_not_found",
		}
		c.JSON(200, gin.H{
			"error": openAIError,
		})
	}
}

// NebulaModelPricing 定价详情
type NebulaModelPricing struct {
	// 计费类型: "token" | "per_call" | "per_image" | "per_second"
	// token: 按量计费(文本/图片/音频token)
	// per_call: 按次计费
	// per_image: 按张计费(图片生成)
	// per_second: 按秒计费(视频生成)
	BillingType string `json:"billing_type"`

	// 按量计费 (token) - 单位: $/M
	InputTextPrice     *string `json:"input_text_price,omitempty"`     // 文本输入 ($/M)
	OutputTextPrice    *string `json:"output_text_price,omitempty"`    // 文本输出 ($/M)
	CachedInputPrice   *string `json:"cached_input_price,omitempty"`   // 缓存输入 ($/M)
	CacheCreationPrice *string `json:"cache_creation_price,omitempty"` // 创建缓存 ($/M)
	InputImagePrice    *string `json:"input_image_price,omitempty"`    // 图片输入 ($/M)
	OutputImagePrice   *string `json:"output_image_price,omitempty"`   // 图片输出 ($/M)
	InputAudioPrice    *string `json:"input_audio_price,omitempty"`    // 音频输入 ($/M)
	OutputAudioPrice   *string `json:"output_audio_price,omitempty"`   // 音频输出 ($/M)

	// 按次计费 - 单位: $/次
	PricePerCall *string `json:"price_per_call,omitempty"`

	// 按张计费 - 单位: $/张 (与per_call相同,为兼容性保留)
	PricePerImage *string `json:"price_per_image,omitempty"`

	// 按秒计费 - 单位: $/秒
	PricePerSecond *string `json:"price_per_second,omitempty"`
}

// NebulaModel 模型信息
type NebulaModel struct {
	ID                 string                  `json:"id"`
	Vendor             string                  `json:"vendor,omitempty"`
	Tags               string                  `json:"tags,omitempty"`
	Pricing            NebulaModelPricing      `json:"pricing"`
	SupportedEndpoints []constant.EndpointType `json:"supported_endpoints"`
}

// GetNebulaModels 获取用户可用的模型列表(含价格和厂商信息)
func GetNebulaModels(c *gin.Context) {
	// 1. 获取当前用户ID和分组
	userId := c.GetInt("id")
	common.SysLog(fmt.Sprintf("[GetNebulaModels] userId: %d", userId))

	// 获取用户分组
	userGroup, err := model.GetUserGroup(userId, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 获取用户分组失败: %v", err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户分组失败",
		})
		return
	}

	// 优先使用token中的分组信息
	group := userGroup
	tokenGroup := common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	if tokenGroup != "" {
		group = tokenGroup
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 使用token分组: %s", tokenGroup))
	} else {
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 使用用户分组: %s", userGroup))
	}

	// 获取用户信息以获取OEM ID
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 获取用户信息失败: %v", err))
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取用户信息失败",
		})
		return
	}

	// 获取用户的OEM ID用于价格折扣
	var oemId int64 = 0 // 默认为0表示无OEM
	if user.OemId != nil {
		oemId = *user.OemId
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 用户OEM ID: %d", oemId))
	} else {
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 用户无OEM配置，使用默认折扣"))
	}

	// 2. 获取完整的定价信息
	allPricing := model.GetPricing()
	common.SysLog(fmt.Sprintf("[GetNebulaModels] 获取到 %d 个模型定价信息", len(allPricing)))

	// 3. 根据用户分组过滤模型
	var userModels []model.Pricing

	// 处理 auto 分组特殊情况
	if tokenGroup == "auto" {
		// auto分组需要合并所有AutoGroups中的模型
		modelSet := make(map[string]model.Pricing)
		for _, autoGroup := range setting.AutoGroups {
			for _, p := range allPricing {
				if containsGroup(p.EnableGroup, autoGroup) {
					modelSet[p.ModelName] = p
				}
			}
		}
		for _, p := range modelSet {
			userModels = append(userModels, p)
		}
		common.SysLog(fmt.Sprintf("[GetNebulaModels] auto分组合并后模型数: %d", len(userModels)))
	} else {
		// 普通分组过滤
		for _, p := range allPricing {
			if containsGroup(p.EnableGroup, group) {
				userModels = append(userModels, p)
			}
		}
		common.SysLog(fmt.Sprintf("[GetNebulaModels] 过滤后用户可用模型数: %d", len(userModels)))
	}

	// 4. 转换为新的响应格式
	response := make([]NebulaModel, 0, len(userModels))
	for _, p := range userModels {
		// 过滤：只返回对话相关的模型
		if !isConversationModel(p.Tags) {
			continue
		}

		nebulaModel := NebulaModel{
			ID:                 p.ModelName,
			Tags:               p.Tags,
			Pricing:            convertToNebulaModelPricing(p, oemId),
			SupportedEndpoints: p.SupportedEndpointTypes,
		}

		// 关联厂商名称
		if p.VendorID > 0 {
			vendorName := getVendorName(p.VendorID)
			if vendorName != "" {
				nebulaModel.Vendor = vendorName
			}
		}

		response = append(response, nebulaModel)
	}

	common.SysLog(fmt.Sprintf("[GetNebulaModels] 返回 %d 个模型", len(response)))

	// 5. 返回JSON响应
	c.JSON(200, gin.H{
		"success": true,
		"data":    response,
	})
}

// containsGroup 检查分组列表中是否包含指定分组
func containsGroup(groups []string, target string) bool {
	for _, g := range groups {
		if g == target {
			return true
		}
	}
	return false
}

// isConversationModel 检查模型是否为对话模型
// 对话模型需要包含以下标签之一: 对话、文案生成、代码能力、思考
func isConversationModel(tags string) bool {
	if tags == "" {
		return false
	}

	conversationTags := []string{"对话", "文案生成", "代码能力", "思考"}
	tagsLower := strings.ToLower(tags)

	for _, tag := range conversationTags {
		if strings.Contains(tagsLower, strings.ToLower(tag)) {
			return true
		}
	}

	return false
}

// getVendorName 获取厂商名称
func getVendorName(vendorID int) string {
	if vendorID == 0 {
		return ""
	}

	vendor, err := model.GetVendorByID(vendorID)
	if err != nil {
		return ""
	}

	return vendor.Name
}

// formatPrice 格式化价格为字符串 ($/M 格式)
func formatPrice(price float64) string {
	return fmt.Sprintf("%.6f $/M", price)
}

// formatPricePerUnit 格式化按次/按秒价格
func formatPricePerUnit(price float64, unit string) string {
	return fmt.Sprintf("%.6f $/%s", price, unit)
}

// convertToNebulaModelPricing 转换价格信息为Nebula格式
func convertToNebulaModelPricing(pricing model.Pricing, oemId int64) NebulaModelPricing {
	result := NebulaModelPricing{}
	modelName := pricing.ModelName

	// 获取OEM折扣
	vendorName := ""
	if pricing.VendorID > 0 {
		if vendor, err := model.GetVendorByID(pricing.VendorID); err == nil {
			vendorName = vendor.Name
		}
	}

	// 使用OEM ID获取用户折扣
	oemDiscount := 1.0
	if oemId > 0 {
		oemDiscount = model.GetOemUserDiscount(oemId, modelName, vendorName)
		if oemDiscount <= 0 {
			oemDiscount = 1.0
		}
	}
	common.SysLog(fmt.Sprintf("[GetNebulaModels] 模型 %s (OEM ID: %d) 用户折扣: %.4f", modelName, oemId, oemDiscount))

	// 优先级1: 检查按秒计费(视频模型)
	if videoPrice, hasVideoPrice := ratio_setting.GetVideoModelPricePerSecondWithAudio(modelName, false); hasVideoPrice && videoPrice > 0 {
		result.BillingType = "per_second"
		// 应用OEM折扣
		finalPrice := videoPrice * oemDiscount
		priceStr := formatPricePerUnit(finalPrice, "秒")
		result.PricePerSecond = &priceStr
		return result
	}

	// 优先级2: 检查按张计费(图片模型)
	if imagePrice, hasImagePrice := ratio_setting.GetImageModelPricePerImage(modelName); hasImagePrice && imagePrice > 0 {
		result.BillingType = "per_image"
		// 应用OEM折扣
		finalPrice := imagePrice * oemDiscount
		priceStr := formatPricePerUnit(finalPrice, "张")
		result.PricePerImage = &priceStr
		return result
	}

	// 优先级3: 检查按次计费(ModelPrice)
	if pricing.QuotaType == 1 && pricing.ModelPrice > 0 {
		result.BillingType = "per_call"
		// 应用OEM折扣
		finalPrice := pricing.ModelPrice * oemDiscount
		priceStr := formatPricePerUnit(finalPrice, "次")
		result.PricePerCall = &priceStr
		return result
	}

	// 优先级4: 图像Token表计费(特殊)
	if pricing.QuotaType == 2 {
		result.BillingType = "token"
		// 从ImageTokenPricing获取详细价格 (已经是$/M tokens)
		if imgPricing, hasImgPricing := ratio_setting.GetImageTokenPricing(modelName); hasImgPricing {
			inputTextPriceStr := formatPrice(imgPricing.InputTextPrice)
			result.InputTextPrice = &inputTextPriceStr

			inputImagePriceStr := formatPrice(imgPricing.InputImagePrice)
			result.InputImagePrice = &inputImagePriceStr

			outputImagePriceStr := formatPrice(imgPricing.OutputImagePrice)
			result.OutputImagePrice = &outputImagePriceStr
		}
		return result
	}

	// 优先级5: 按量计费(token) - QuotaType=0 或默认
	result.BillingType = "token"

	// 基准价格: 1 ratio = $2/M tokens
	if pricing.ModelRatio > 0 {
		// 文本输入价格: ModelRatio × $2/M × OEM折扣
		inputTextPrice := pricing.ModelRatio * 2.0 * oemDiscount
		inputTextPriceStr := formatPrice(inputTextPrice)
		result.InputTextPrice = &inputTextPriceStr

		// 文本输出价格: 输入价格 × CompletionRatio
		if pricing.CompletionRatio > 0 {
			outputTextPrice := inputTextPrice * pricing.CompletionRatio
			outputTextPriceStr := formatPrice(outputTextPrice)
			result.OutputTextPrice = &outputTextPriceStr
		}

		// 缓存输入价格 (CacheRatio)
		cacheRatio, hasCacheRatio := ratio_setting.GetCacheRatio(modelName)
		if hasCacheRatio && cacheRatio > 0 && cacheRatio < 1 {
			cachedInputPrice := inputTextPrice * cacheRatio
			cachedInputPriceStr := formatPrice(cachedInputPrice)
			result.CachedInputPrice = &cachedInputPriceStr
		}

		// 创建缓存价格 (CacheCreationRatio 或 Claude默认1.25倍)
		cacheCreationRatio, hasCacheCreation := ratio_setting.GetCreateCacheRatio(modelName)
		if hasCacheCreation && cacheCreationRatio > 0 {
			cacheCreationPrice := inputTextPrice * cacheCreationRatio
			cacheCreationPriceStr := formatPrice(cacheCreationPrice)
			result.CacheCreationPrice = &cacheCreationPriceStr
		} else if strings.Contains(strings.ToLower(modelName), "claude") {
			// Claude模型默认创建缓存价格是输入价格的1.25倍
			cacheCreationPrice := inputTextPrice * 1.25
			cacheCreationPriceStr := formatPrice(cacheCreationPrice)
			result.CacheCreationPrice = &cacheCreationPriceStr
		}
	}

	// 图片价格 - 用于多模态模型(只有在配置中明确设置时才显示)
	imageRatio, hasImageRatio := ratio_setting.GetImageRatio(modelName)
	imageCompletionRatioMap := ratio_setting.GetImageCompletionRatioCopy()
	imageCompletionRatio, hasImageCompletion := imageCompletionRatioMap[modelName]

	// 如果有ImageRatio配置
	if hasImageRatio && imageRatio > 0 {
		// 图片输入价格: ImageRatio × $2/M × OEM折扣
		inputImagePrice := imageRatio * 2.0 * oemDiscount
		inputImagePriceStr := formatPrice(inputImagePrice)
		result.InputImagePrice = &inputImagePriceStr

		// 图片输出价格: 图片输入价格 × ImageCompletionRatio (只有明确配置了才显示)
		if hasImageCompletion && imageCompletionRatio > 0 {
			outputImagePrice := inputImagePrice * imageCompletionRatio
			outputImagePriceStr := formatPrice(outputImagePrice)
			result.OutputImagePrice = &outputImagePriceStr
		}
	} else if hasImageCompletion && imageCompletionRatio > 0 && pricing.ModelRatio > 0 {
		// 没有ImageRatio但明确配置了ImageCompletionRatio: 使用文本输入价格作为基准
		// 图片输出价格: 文本输入价格 × ImageCompletionRatio
		inputTextPrice := pricing.ModelRatio * 2.0 * oemDiscount
		outputImagePrice := inputTextPrice * imageCompletionRatio
		outputImagePriceStr := formatPrice(outputImagePrice)
		result.OutputImagePrice = &outputImagePriceStr
	}

	// 音频价格 - 用于多模态模型(只有在配置中明确设置时才显示)
	// 获取音频配置的副本，检查是否明确配置了该模型
	audioRatioMap := ratio_setting.GetAudioRatioCopy()
	audioCompletionRatioMap := ratio_setting.GetAudioCompletionRatioCopy()

	// 只有在map中明确存在该模型配置时才显示音频价格
	if audioRatio, hasAudio := audioRatioMap[modelName]; hasAudio && audioRatio > 0 {
		// 音频输入价格: AudioRatio × $2/M × OEM折扣
		inputAudioPrice := audioRatio * 2.0 * oemDiscount
		inputAudioPriceStr := formatPrice(inputAudioPrice)
		result.InputAudioPrice = &inputAudioPriceStr

		if audioCompletionRatio, hasCompletion := audioCompletionRatioMap[modelName]; hasCompletion && audioCompletionRatio > 0 {
			outputAudioPrice := inputAudioPrice * audioCompletionRatio
			outputAudioPriceStr := formatPrice(outputAudioPrice)
			result.OutputAudioPrice = &outputAudioPriceStr
		}
	}

	return result
}
