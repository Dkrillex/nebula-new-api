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
// 优先级：用户oem_id（从Context获取）> 请求头X-Oem-Code > 默认nebula
// 注意：用户已认证时优先使用用户的oem_id，未认证或oem_id为空时使用请求头
// userQuota 参数是实际扣费的 quota，用于确保价格链中的 user_quota 与实际扣费一致
//
// 重要：这个函数现在基于实际 quota 反推官方价格
func CalculatePriceChain(c *gin.Context, modelName string, vendorName string, tokens int, userQuota int) *PriceChain {
	chain := &PriceChain{}

	// 1. 优先从Context获取OEM信息
	// SystemIdentify中间件会优先使用用户oem_id（如果用户已认证且有oem_id），否则使用请求头X-Oem-Code
	// 所以这里从Context获取的已经是优先级最高的OEM信息了
	var oemId *int64
	var oemCode string

	if c != nil {
		// 优先获取OEM ID（可能来自用户oem_id或请求头）
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
		// 获取OEM Code（用于显示和fallback）
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		// 如果只有oemCode没有oemId，通过oemCode查询oemId
		if oemId == nil && oemCode != "" {
			oemConfig := model.GetOemConfigByCode(oemCode)
			if oemConfig != nil {
				oemId = &oemConfig.Id
			}
		}
	}

	if oemCode == "" {
		oemCode = "nebula"
	}
	if oemId == nil {
		oemConfig := model.GetOemConfigByCode("nebula")
		if oemConfig != nil {
			oemId = &oemConfig.Id
		}
	}

	chain.OemCode = oemCode
	chain.OemId = oemId

	// 2. 获取各种折扣率
	// oem_user_discount: OEM给用户的折扣
	var oemUserDiscount float64
	if oemId != nil {
		oemUserDiscount = model.GetOemUserDiscount(*oemId, modelName, vendorName)
	} else {
		oemUserDiscount = model.GetOemUserDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemUserDiscount <= 0 {
		oemUserDiscount = 1.0
	}

	// oem_discount: 平台给OEM的折扣（OEM的成本）
	var oemDiscount float64
	if oemId != nil {
		oemDiscount = model.GetOemDiscount(*oemId, modelName, vendorName)
	} else {
		oemDiscount = model.GetOemDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemDiscount <= 0 {
		oemDiscount = 1.0
	}

	// platform_cost: 平台成本折扣
	costDiscount := model.GetPlatformCostDiscount(modelName, vendorName)
	if costDiscount <= 0 {
		costDiscount = 1.0
	}
	chain.CostDiscount = costDiscount

	// group_ratio: 用户分组倍率
	groupRatio := GetGroupRatioByOemFromContext(c, "default")
	if c != nil {
		if group, exists := c.Get("group"); exists {
			if groupStr, ok := group.(string); ok && groupStr != "" {
				groupRatio = GetGroupRatioByOemFromContext(c, groupStr)
			}
		}
	}
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// 3. 基于实际 quota 反推官方价格
	// 实际扣费公式：user_quota = official_quota × oem_user_discount × group_ratio
	// 反推：official_quota = user_quota / (oem_user_discount × group_ratio)
	userQuotaValue := int64(userQuota)
	var officialQuota int64
	if oemUserDiscount > 0 && groupRatio > 0 {
		officialQuota = int64(float64(userQuotaValue) / (oemUserDiscount * groupRatio))
	} else {
		officialQuota = userQuotaValue
	}
	chain.OfficialQuota = officialQuota

	// 4. 计算平台成本价
	costQuota := int64(float64(officialQuota) * costDiscount)
	chain.CostQuota = costQuota

	// 5. 计算系统销售价（OEM的成本）
	systemQuota := int64(float64(officialQuota) * oemDiscount)
	chain.SystemDiscount = oemDiscount
	chain.SystemQuota = systemQuota

	// 6. 计算平台利润
	chain.PlatformProfit = systemQuota - costQuota

	// 7. 用户支付价
	chain.UserDiscount = oemUserDiscount
	chain.UserQuota = userQuotaValue

	// 8. 计算OEM盈亏（正数表示盈利，负数表示亏损/补贴）
	// OEM盈亏 = 用户支付价 - OEM成本（系统销售价）
	chain.OemSubsidy = userQuotaValue - systemQuota

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

// GetGroupRatioByOemFromContext 从Context中获取OEM代码，然后获取对应的GroupRatio
// 用于实际扣费时获取OEM特定的GroupRatio
func GetGroupRatioByOemFromContext(c *gin.Context, group string) float64 {
	oemCode := "nebula" // 默认系统
	if c != nil {
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
	}
	return GetGroupRatioByOem(oemCode, group)
}

// GetVendorNameFromModel 从模型名称获取厂商名称
// 通过查询models表获取vendor_id，再查询vendors表获取name
func GetVendorNameFromModel(modelName string) string {
	var m model.Model
	err := model.DB.Where("model_name = ?", modelName).First(&m).Error
	if err != nil {
		// 如果查询失败，返回空字符串
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("[GetVendorNameFromModel] 查询模型失败: modelName=%s, err=%v", modelName, err))
		}
		return ""
	}
	if m.VendorID == 0 {
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("[GetVendorNameFromModel] 模型没有关联厂商: modelName=%s, vendorId=0", modelName))
		}
		return ""
	}
	vendor, err := model.GetVendorByID(m.VendorID)
	if err != nil {
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("[GetVendorNameFromModel] 查询厂商失败: modelName=%s, vendorId=%d, err=%v", modelName, m.VendorID, err))
		}
		return ""
	}
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("[GetVendorNameFromModel] 成功: modelName=%s, vendorId=%d, vendorName=%s", modelName, m.VendorID, vendor.Name))
	}
	return vendor.Name
}

// CalculatePriceChainForLog 为日志记录计算价格链条
// 这是一个辅助函数，用于在RecordConsumeLog中自动计算价格链条
// 优先级：用户oem_id（从Context获取）> 请求头X-Oem-Code > 默认nebula
// 注意：用户已认证时优先使用用户的oem_id，未认证或oem_id为空时使用请求头
// promptTokens 和 completionTokens 是原始 tokens 数量
// quota 是实际扣费的 quota（已包含所有折扣）
//
// 重要：这个函数现在基于实际 quota 反推官方价格，而不是基于 tokens 计算
// 这样可以准确处理各种复杂场景（缓存、音频、图片等不同倍率的 tokens）
func CalculatePriceChainForLog(c *gin.Context, modelName string, promptTokens int, completionTokens int, quota int) *model.PriceChainParams {
	// 获取厂商名称
	vendorName := GetVendorNameFromModel(modelName)
	if vendorName == "" {
		vendorName = ""
	}

	// 1. 优先从Context获取OEM信息
	// SystemIdentify中间件会优先使用用户oem_id（如果用户已认证且有oem_id），否则使用请求头X-Oem-Code
	// 所以这里从Context获取的已经是优先级最高的OEM信息了
	var oemId *int64
	var oemCode string

	if c != nil {
		// 优先获取OEM ID（可能来自用户oem_id或请求头）
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
		// 获取OEM Code（用于显示和fallback）
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		// 如果只有oemCode没有oemId，通过oemCode查询oemId
		if oemId == nil && oemCode != "" {
			oemConfig := model.GetOemConfigByCode(oemCode)
			if oemConfig != nil {
				oemId = &oemConfig.Id
			}
		}
	}

	if oemCode == "" {
		oemCode = "nebula"
	}
	if oemId == nil {
		oemConfig := model.GetOemConfigByCode("nebula")
		if oemConfig != nil {
			oemId = &oemConfig.Id
		}
	}

	// 2. 获取各种折扣率
	// oem_user_discount: OEM给用户的折扣
	var oemUserDiscount float64
	if oemId != nil {
		oemUserDiscount = model.GetOemUserDiscount(*oemId, modelName, vendorName)
	} else {
		oemUserDiscount = model.GetOemUserDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemUserDiscount <= 0 {
		oemUserDiscount = 1.0
	}

	// oem_discount: 平台给OEM的折扣（OEM的成本）
	var oemDiscount float64
	if oemId != nil {
		oemDiscount = model.GetOemDiscount(*oemId, modelName, vendorName)
	} else {
		oemDiscount = model.GetOemDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemDiscount <= 0 {
		oemDiscount = 1.0
	}

	// platform_cost: 平台成本折扣
	costDiscount := model.GetPlatformCostDiscount(modelName, vendorName)
	if costDiscount <= 0 {
		costDiscount = 1.0
	}

	// group_ratio: 用户分组倍率
	groupRatio := GetGroupRatioByOemFromContext(c, "default")
	if c != nil {
		if group, exists := c.Get("group"); exists {
			if groupStr, ok := group.(string); ok && groupStr != "" {
				groupRatio = GetGroupRatioByOemFromContext(c, groupStr)
			}
		}
	}
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// 3. 基于实际 quota 反推官方价格
	// 实际扣费公式：user_quota = official_quota × oem_user_discount × group_ratio
	// 反推：official_quota = user_quota / (oem_user_discount × group_ratio)
	userQuotaValue := int64(quota)
	var officialQuota int64
	if oemUserDiscount > 0 && groupRatio > 0 {
		officialQuota = int64(float64(userQuotaValue) / (oemUserDiscount * groupRatio))
	} else {
		officialQuota = userQuotaValue
	}

	// 4. 计算平台成本价
	costQuota := int64(float64(officialQuota) * costDiscount)

	// 5. 计算系统销售价（OEM的成本）
	systemQuota := int64(float64(officialQuota) * oemDiscount)

	// 6. 计算平台利润
	platformProfit := systemQuota - costQuota

	// 7. 计算OEM盈亏（正数表示盈利，负数表示亏损/补贴）
	// OEM盈亏 = 用户支付价 - OEM成本（系统销售价）
	oemSubsidy := userQuotaValue - systemQuota

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

// CalculatePriceChainForImageGeneration 为图片生成计算价格链条
// 图片生成是按张计费，不是按tokens计费，需要特殊处理
// 优先级：用户oem_id（从Context获取）> 请求头X-Oem-Code > 默认nebula
// 注意：用户已认证时优先使用用户的oem_id，未认证或oem_id为空时使用请求头
//
// 重要：这个函数现在基于实际 quota 反推官方价格
func CalculatePriceChainForImageGeneration(c *gin.Context, modelName string, imagePrice float64, imageCount int, quota int) *model.PriceChainParams {
	// 1. 优先从Context获取OEM信息
	// SystemIdentify中间件会优先使用用户oem_id（如果用户已认证且有oem_id），否则使用请求头X-Oem-Code
	// 所以这里从Context获取的已经是优先级最高的OEM信息了
	var oemId *int64
	var oemCode string

	if c != nil {
		// 优先获取OEM ID（可能来自用户oem_id或请求头）
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
		// 获取OEM Code（用于显示和fallback）
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		// 如果只有oemCode没有oemId，通过oemCode查询oemId
		if oemId == nil && oemCode != "" {
			oemConfig := model.GetOemConfigByCode(oemCode)
			if oemConfig != nil {
				oemId = &oemConfig.Id
			}
		}
	}

	if oemCode == "" {
		oemCode = "nebula"
	}
	if oemId == nil {
		oemConfig := model.GetOemConfigByCode("nebula")
		if oemConfig != nil {
			oemId = &oemConfig.Id
		}
	}

	// 获取厂商名称
	vendorName := GetVendorNameFromModel(modelName)
	if vendorName == "" {
		vendorName = ""
	}

	// 2. 获取各种折扣率
	// oem_user_discount: OEM给用户的折扣
	var oemUserDiscount float64
	if oemId != nil {
		oemUserDiscount = model.GetOemUserDiscount(*oemId, modelName, vendorName)
	} else {
		oemUserDiscount = model.GetOemUserDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemUserDiscount <= 0 {
		oemUserDiscount = 1.0
	}

	// oem_discount: 平台给OEM的折扣（OEM的成本）
	var oemDiscount float64
	if oemId != nil {
		oemDiscount = model.GetOemDiscount(*oemId, modelName, vendorName)
	} else {
		oemDiscount = model.GetOemDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemDiscount <= 0 {
		oemDiscount = 1.0
	}

	// platform_cost: 平台成本折扣
	costDiscount := model.GetPlatformCostDiscount(modelName, vendorName)
	if costDiscount <= 0 {
		costDiscount = 1.0
	}

	// group_ratio: 用户分组倍率
	groupRatio := GetGroupRatioByOemFromContext(c, "default")
	if c != nil {
		if group, exists := c.Get("group"); exists {
			if groupStr, ok := group.(string); ok && groupStr != "" {
				groupRatio = GetGroupRatioByOemFromContext(c, groupStr)
			}
		}
	}
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// 3. 基于实际 quota 反推官方价格
	// 实际扣费公式：user_quota = official_quota × oem_user_discount × group_ratio
	// 反推：official_quota = user_quota / (oem_user_discount × group_ratio)
	userQuotaValue := int64(quota)
	var officialQuota int64
	if oemUserDiscount > 0 && groupRatio > 0 {
		officialQuota = int64(float64(userQuotaValue) / (oemUserDiscount * groupRatio))
	} else {
		officialQuota = userQuotaValue
	}

	// 4. 计算平台成本价
	costQuota := int64(float64(officialQuota) * costDiscount)

	// 5. 计算系统销售价（OEM的成本）
	systemQuota := int64(float64(officialQuota) * oemDiscount)

	// 6. 计算平台利润
	platformProfit := systemQuota - costQuota

	// 7. 计算OEM盈亏（正数表示盈利，负数表示亏损/补贴）
	oemSubsidy := userQuotaValue - systemQuota

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

// CalculatePriceChainForVideoTask 为视频任务计算价格链条
// 视频任务的轮询是在后台进行的，没有HTTP请求上下文，所以需要传入OEM信息
// 参数说明：
//   - oemCode: OEM代码（如 "nebula"）
//   - modelName: 模型名称
//   - vendorName: 厂商名称
//   - quota: 实际扣费的quota
//   - oemUserDiscount: OEM用户折扣（在任务创建时保存的）
//   - groupRatio: 用户分组倍率
func CalculatePriceChainForVideoTask(oemCode string, modelName string, vendorName string, quota int, oemUserDiscount float64, groupRatio float64) *model.PriceChainParams {
	// 获取 OEM 配置
	var oemId *int64
	if oemCode == "" {
		oemCode = "nebula"
	}
	oemConfig := model.GetOemConfigByCode(oemCode)
	if oemConfig != nil {
		oemId = &oemConfig.Id
	}

	// 获取厂商名称（如果未提供）
	if vendorName == "" {
		vendorName = GetVendorNameFromModel(modelName)
	}

	// 确保折扣值有效
	if oemUserDiscount <= 0 {
		oemUserDiscount = 1.0
	}
	if groupRatio <= 0 {
		groupRatio = 1.0
	}

	// oem_discount: 平台给OEM的折扣（OEM的成本）
	var oemDiscount float64
	if oemId != nil {
		oemDiscount = model.GetOemDiscount(*oemId, modelName, vendorName)
	} else {
		oemDiscount = model.GetOemDiscountByCode(oemCode, modelName, vendorName)
	}
	if oemDiscount <= 0 {
		oemDiscount = 1.0
	}

	// platform_cost: 平台成本折扣
	costDiscount := model.GetPlatformCostDiscount(modelName, vendorName)
	if costDiscount <= 0 {
		costDiscount = 1.0
	}

	// 基于实际 quota 反推官方价格
	// 实际扣费公式：user_quota = official_quota × oem_user_discount × group_ratio
	// 反推：official_quota = user_quota / (oem_user_discount × group_ratio)
	userQuotaValue := int64(quota)
	var officialQuota int64
	if oemUserDiscount > 0 && groupRatio > 0 {
		officialQuota = int64(float64(userQuotaValue) / (oemUserDiscount * groupRatio))
	} else {
		officialQuota = userQuotaValue
	}

	// 计算平台成本价
	costQuota := int64(float64(officialQuota) * costDiscount)

	// 计算系统销售价（OEM的成本）
	systemQuota := int64(float64(officialQuota) * oemDiscount)

	// 计算平台利润
	platformProfit := systemQuota - costQuota

	// 计算OEM盈亏（正数表示盈利，负数表示亏损/补贴）
	// OEM盈亏 = 用户支付价 - OEM成本（系统销售价）
	oemSubsidyValue := userQuotaValue - systemQuota

	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("[CalculatePriceChainForVideoTask] oemCode=%s, modelName=%s, vendorName=%s, quota=%d, oemUserDiscount=%.4f, groupRatio=%.4f",
			oemCode, modelName, vendorName, quota, oemUserDiscount, groupRatio))
		common.SysLog(fmt.Sprintf("[CalculatePriceChainForVideoTask] officialQuota=%d, costQuota=%d, systemQuota=%d, userQuota=%d, platformProfit=%d, oemSubsidy=%d",
			officialQuota, costQuota, systemQuota, userQuotaValue, platformProfit, oemSubsidyValue))
	}

	return &model.PriceChainParams{
		OemId:          oemId,
		OemCode:        oemCode,
		OfficialQuota:  officialQuota,
		CostQuota:      costQuota,
		SystemQuota:    systemQuota,
		UserQuota:      userQuotaValue,
		PlatformProfit: platformProfit,
		OemSubsidy:     oemSubsidyValue,
	}
}

// GetOemUserDiscountForQuota 获取用于quota计算的OEM用户折扣
// 用于在实际扣费时应用OEM给用户的折扣
// 优先级：用户oem_id（从Context获取）> 请求头X-Oem-Code > 默认nebula
// 注意：用户已认证时优先使用用户的oem_id，未认证或oem_id为空时使用请求头
func GetOemUserDiscountForQuota(c *gin.Context, modelName string) float64 {
	var oemId *int64
	var oemCode string

	if c != nil {
		// 获取OEM Code
		if code, exists := c.Get(string(constant.ContextKeyOemCode)); exists {
			if codeStr, ok := code.(string); ok && codeStr != "" {
				oemCode = codeStr
			}
		}
		// 获取OEM ID
		if id, exists := c.Get(string(constant.ContextKeyOemId)); exists {
			if idInt64, ok := id.(int64); ok {
				oemId = &idInt64
			}
		}
	}

	// 默认使用nebula系统
	if oemCode == "" {
		oemCode = "nebula"
	}
	if oemId == nil {
		oemConfig := model.GetOemConfigByCode(oemCode)
		if oemConfig != nil {
			oemId = &oemConfig.Id
		}
	}

	if oemId == nil {
		if common.DebugEnabled {
			common.SysLog(fmt.Sprintf("[GetOemUserDiscountForQuota] oemId is nil, returning 1.0"))
		}
		return 1.0
	}

	vendorName := GetVendorNameFromModel(modelName)
	discount := model.GetOemUserDiscount(*oemId, modelName, vendorName)
	if common.DebugEnabled {
		common.SysLog(fmt.Sprintf("[GetOemUserDiscountForQuota] oemId=%d, modelName=%s, vendorName=%s, discount=%.4f",
			*oemId, modelName, vendorName, discount))
	}
	return discount
}
