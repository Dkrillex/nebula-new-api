package service

import (
	"fmt"
	"one-api/common"
	"one-api/constant"
	"one-api/model"
	"one-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// PriceChain 价格链条结果
type PriceChain struct {
	OemId          *int64  // OEM ID
	OemCode        string  // OEM代码（用于向后兼容和日志显示）
	OfficialQuota  int64   // 官方价格quota（基于ModelRatio）
	CostDiscount   float64 // 成本折扣率
	CostQuota      int64   // 成本价quota
	SystemDiscount float64 // 系统销售折扣率
	SystemQuota    int64   // 系统销售价quota
	PlatformProfit int64   // 平台利润quota
	UserDiscount   float64 // 用户折扣率（GroupRatio）
	UserQuota      int64   // 用户支付价quota
	OemSubsidy     int64   // OEM补贴quota（负数表示补贴，正数表示盈利）
}

// CalculatePriceChain 计算完整价格链条
// 价格链条：官方价格 → 平台成本 → 系统销售价 → 用户支付价
func CalculatePriceChain(c *gin.Context, modelName string, vendorName string, tokens int, userQuota int) *PriceChain {
	chain := &PriceChain{}

	// 1. 获取OEM代码和ID
	oemCode := "nebula" // 默认系统
	var oemId *int64
	if c != nil {
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		// 向后兼容：如果没有OemCode，尝试从SystemCode获取
		if oemCode == "nebula" {
			if code, exists := c.Get(string(constant.ContextKeySystemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
		}
		// 获取OEM ID
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
	}

	// 如果oemId为空，通过oemCode查询
	if oemId == nil {
		oemConfig := model.GetOemConfigByCode(oemCode)
		if oemConfig != nil {
			oemId = &oemConfig.Id
		}
	}

	chain.OemCode = oemCode
	chain.OemId = oemId

	// 2. 计算官方价格（基于ModelRatio）
	modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
	officialQuota := int64(float64(tokens) * modelRatio)
	chain.OfficialQuota = officialQuota

	// 3. 计算平台成本价
	costDiscount := model.GetPlatformCostDiscount(modelName, vendorName)
	costQuota := int64(float64(officialQuota) * costDiscount)
	chain.CostDiscount = costDiscount
	chain.CostQuota = costQuota

	// 4. 计算系统销售价
	var systemDiscount float64
	if oemId != nil {
		systemDiscount = model.GetOemDiscount(*oemId, modelName, vendorName)
	} else {
		// 向后兼容：使用oemCode查询
		systemDiscount = model.GetOemDiscountByCode(oemCode, modelName, vendorName)
	}
	systemQuota := int64(float64(officialQuota) * systemDiscount)
	chain.SystemDiscount = systemDiscount
	chain.SystemQuota = systemQuota

	// 5. 计算平台利润
	chain.PlatformProfit = systemQuota - costQuota

	// 6. 计算用户支付价（基于GroupRatio）
	// 获取用户分组
	userGroup := "default"
	if c != nil {
		if group, exists := c.Get(string(constant.ContextKeyUserGroup)); exists {
			if groupStr, ok := group.(string); ok {
				userGroup = groupStr
			}
		}
	}

	// 获取OEM特定的GroupRatio
	userDiscount := GetGroupRatioByOem(oemCode, userGroup)
	userQuotaValue := int64(float64(systemQuota) * userDiscount)
	chain.UserDiscount = userDiscount
	chain.UserQuota = userQuotaValue

	// 7. 计算OEM补贴（负数表示补贴，正数表示盈利）
	// 补贴 = 系统销售价 - 用户支付价
	// 如果用户支付价 < 系统销售价，说明OEM在补贴用户
	chain.OemSubsidy = systemQuota - userQuotaValue

	// 注意：实际扣费quota可能与计算出的userQuota不一致，这是正常的
	// 因为实际扣费可能受到其他因素影响（如缓存、特殊定价等）

	return chain
}

// GetGroupRatioByOem 根据OEM代码获取GroupRatio
// 优先级：OEM特定GroupRatio > 全局GroupRatio > 默认值(1.0)
func GetGroupRatioByOem(oemCode, group string) float64 {
	// 1. 尝试获取OEM特定的GroupRatio（如GroupRatio_nebula、GroupRatio_xiaomai）
	systemGroupRatioKey := fmt.Sprintf("GroupRatio_%s", oemCode)
	option, err := model.GetOption(systemGroupRatioKey)
	if err == nil && option != nil && option.Value != "" {
		// 解析JSON格式的GroupRatio配置
		groupRatioMap := make(map[string]float64)
		if err := common.Unmarshal([]byte(option.Value), &groupRatioMap); err == nil {
			if ratio, ok := groupRatioMap[group]; ok {
				return ratio
			}
			// 如果没有找到特定group，尝试default
			if ratio, ok := groupRatioMap["default"]; ok {
				return ratio
			}
		}
	}

	// 2. 使用全局GroupRatio
	return ratio_setting.GetGroupRatio(group)
}

// GetVendorNameFromModel 从模型名称获取厂商名称
// 通过查询models表获取vendor_id，再查询vendors表获取name
func GetVendorNameFromModel(modelName string) string {
	var m model.Model
	err := model.DB.Where("model_name = ?", modelName).First(&m).Error
	if err != nil {
		// 如果查询失败，返回空字符串
		return ""
	}
	if m.VendorID == 0 {
		return ""
	}
	vendor, err := model.GetVendorByID(m.VendorID)
	if err != nil {
		return ""
	}
	return vendor.Name
}

// CalculatePriceChainForLog 为日志记录计算价格链条
// 这是一个辅助函数，用于在RecordConsumeLog中自动计算价格链条
func CalculatePriceChainForLog(c *gin.Context, modelName string, promptTokens int, completionTokens int, quota int) *model.PriceChainParams {
	// 获取厂商名称
	vendorName := GetVendorNameFromModel(modelName)
	if vendorName == "" {
		// 如果无法获取厂商名称，使用空字符串（会使用通配符配置）
		vendorName = ""
	}

	// 计算总tokens
	totalTokens := promptTokens + completionTokens

	// 计算价格链条
	priceChain := CalculatePriceChain(c, modelName, vendorName, totalTokens, quota)

	// 转换为PriceChainParams
	return &model.PriceChainParams{
		OemId:          priceChain.OemId,
		OemCode:        priceChain.OemCode,
		OfficialQuota:  priceChain.OfficialQuota,
		CostQuota:      priceChain.CostQuota,
		SystemQuota:    priceChain.SystemQuota,
		UserQuota:      priceChain.UserQuota,
		PlatformProfit: priceChain.PlatformProfit,
		OemSubsidy:     priceChain.OemSubsidy,
	}
}

// CalculatePriceChainForImageGeneration 为图片生成计算价格链条
// 图片生成是按张计费，不是按tokens计费，需要特殊处理
func CalculatePriceChainForImageGeneration(c *gin.Context, modelName string, imagePrice float64, imageCount int, quota int) *model.PriceChainParams {
	// 获取OEM代码和ID
	oemCode := "nebula" // 默认系统
	var oemId *int64
	if c != nil {
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		// 向后兼容：如果没有OemCode，尝试从SystemCode获取
		if oemCode == "nebula" {
			if code, exists := c.Get(string(constant.ContextKeySystemCode)); exists {
				if codeStr, ok := code.(string); ok && codeStr != "" {
					oemCode = codeStr
				}
			}
		}
		// 获取OEM ID
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
	}

	// 如果oemId为空，通过oemCode查询
	if oemId == nil {
		oemConfig := model.GetOemConfigByCode(oemCode)
		if oemConfig != nil {
			oemId = &oemConfig.Id
		}
	}

	// 获取厂商名称
	vendorName := GetVendorNameFromModel(modelName)
	if vendorName == "" {
		vendorName = ""
	}

	// 1. 计算官方价格（基于imagePrice，单位：美元）
	// 对于图片生成，官方价格就是配置的imagePrice
	officialPriceUSD := imagePrice * float64(imageCount)
	// 转换为quota（官方价格）
	officialQuota := int64(officialPriceUSD * common.QuotaPerUnit)

	// 2. 计算平台成本价
	costDiscount := model.GetPlatformCostDiscount(modelName, vendorName)
	costPriceUSD := officialPriceUSD * costDiscount
	costQuota := int64(costPriceUSD * common.QuotaPerUnit)

	// 3. 计算系统销售价
	var systemDiscount float64
	if oemId != nil {
		systemDiscount = model.GetOemDiscount(*oemId, modelName, vendorName)
	} else {
		// 向后兼容：使用oemCode查询
		systemDiscount = model.GetOemDiscountByCode(oemCode, modelName, vendorName)
	}
	systemPriceUSD := officialPriceUSD * systemDiscount
	systemQuota := int64(systemPriceUSD * common.QuotaPerUnit)

	// 4. 计算平台利润
	platformProfit := systemQuota - costQuota

	// 5. 计算用户支付价（基于GroupRatio）
	// 获取用户分组
	userGroup := "default"
	if c != nil {
		if group, exists := c.Get(string(constant.ContextKeyUserGroup)); exists {
			if groupStr, ok := group.(string); ok {
				userGroup = groupStr
			}
		}
	}

	// 获取OEM特定的GroupRatio
	userDiscount := GetGroupRatioByOem(oemCode, userGroup)
	userPriceUSD := systemPriceUSD * userDiscount
	userQuotaValue := int64(userPriceUSD * common.QuotaPerUnit)

	// 6. 计算OEM补贴（负数表示补贴，正数表示盈利）
	oemSubsidy := systemQuota - userQuotaValue

	// 转换为PriceChainParams
	return &model.PriceChainParams{
		OemId:          oemId,
		OemCode:        oemCode,
		OfficialQuota:  officialQuota,
		CostQuota:      costQuota,
		SystemQuota:    systemQuota,
		UserQuota:      userQuotaValue,
		PlatformProfit: platformProfit,
		OemSubsidy:     oemSubsidy,
	}
}
