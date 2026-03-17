package model

import (
	"fmt"
	"one-api/common"
	"sort"
	"sync"
	"time"
)

type PlatformCost struct {
	Id           int64     `json:"id" gorm:"primaryKey"`
	VendorID     *int64    `json:"vendor_id" gorm:"column:vendor_id;index"` // NULL表示通配符
	ModelName    *string   `json:"model_name" gorm:"column:model_name;type:varchar(128)"`
	CostDiscount float64   `json:"cost_discount" gorm:"column:cost_discount;type:decimal(10,4)"`
	Priority     int       `json:"priority" gorm:"column:priority"`
	Enabled      int       `json:"enabled" gorm:"column:enabled"`
	Remark       string    `json:"remark" gorm:"column:remark;type:varchar(500)"`
	CreateTime   time.Time `json:"create_time" gorm:"column:create_time"`
	UpdateTime   time.Time `json:"update_time" gorm:"column:update_time"`
}

func (PlatformCost) TableName() string {
	return "platform_cost"
}

var (
	platformCostCache     []*PlatformCost
	platformCostCacheLock sync.RWMutex
	platformCostCacheTime time.Time
	// 注意：cacheExpireDuration 已在 oem_config.go 中统一定义为 1 分钟
)

// InitPlatformCost 初始化平台成本配置缓存
func InitPlatformCost() {
	RefreshPlatformCostCache()
}

// RefreshPlatformCostCache 刷新平台成本配置缓存
func RefreshPlatformCostCache() {
	var costs []*PlatformCost
	err := DB.Where("enabled = ?", 1).Order("priority DESC").Find(&costs).Error
	if err != nil {
		common.SysLog("刷新平台成本配置缓存失败: " + err.Error())
		return
	}

	platformCostCacheLock.Lock()
	platformCostCache = costs
	platformCostCacheTime = time.Now()
	platformCostCacheLock.Unlock()

	common.SysLog(fmt.Sprintf("平台成本配置缓存已刷新，共 %d 条", len(costs)))
}

// GetAllPlatformCosts 获取所有平台成本配置
func GetAllPlatformCosts() []*PlatformCost {
	platformCostCacheLock.RLock()
	defer platformCostCacheLock.RUnlock()

	// 检查缓存是否过期
	if time.Since(platformCostCacheTime) > cacheExpireDuration {
		// 异步刷新缓存
		go RefreshPlatformCostCache()
	}

	return platformCostCache
}

// GetPlatformCostDiscount 获取成本折扣
// 优先级：模型级 > 厂商级 > 通配符 > 默认值(0.5)
func GetPlatformCostDiscount(modelName, vendorName string) float64 {
	// 先通过vendorName获取vendorId
	var vendorID *int64
	if vendorName != "" {
		var vendor Vendor
		err := DB.Where("name = ? AND deleted_at IS NULL", vendorName).First(&vendor).Error
		if err == nil {
			vendorIDInt64 := int64(vendor.Id)
			vendorID = &vendorIDInt64
		}
	}

	costs := GetAllPlatformCosts()
	if len(costs) == 0 {
		return 0.5 // 默认50%
	}

	// 按优先级排序（数字越大优先级越高）
	sortedCosts := make([]*PlatformCost, len(costs))
	copy(sortedCosts, costs)
	sort.Slice(sortedCosts, func(i, j int) bool {
		return sortedCosts[i].Priority > sortedCosts[j].Priority
	})

	// 1. 模型级匹配（vendor_id和model_name都不为空，且都匹配）
	if modelName != "" && vendorID != nil {
		for _, cost := range sortedCosts {
			if cost.VendorID != nil && cost.ModelName != nil && *cost.ModelName != "" &&
				*cost.VendorID == *vendorID && *cost.ModelName == modelName {
				return cost.CostDiscount
			}
		}
	}

	// 2. 厂商级匹配（vendor_id不为空，model_name为空）
	if vendorID != nil {
		for _, cost := range sortedCosts {
			if cost.VendorID != nil && (cost.ModelName == nil || *cost.ModelName == "") &&
				*cost.VendorID == *vendorID {
				return cost.CostDiscount
			}
		}
	}

	// 3. 通配符匹配（vendor_id为NULL，model_name为空）
	for _, cost := range sortedCosts {
		if cost.VendorID == nil && (cost.ModelName == nil || *cost.ModelName == "") {
			return cost.CostDiscount
		}
	}

	// 4. 默认成本50%
	return 0.5
}
