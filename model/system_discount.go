package model

import (
	"fmt"
	"one-api/common"
	"sort"
	"sync"
	"time"
)

type SystemDiscount struct {
	Id             int64     `json:"id" gorm:"primaryKey"`
	SystemCode     string    `json:"system_code" gorm:"column:system_code;index"`
	VendorID       *int64    `json:"vendor_id" gorm:"column:vendor_id;index"` // NULL表示通配符
	ModelName      *string   `json:"model_name" gorm:"column:model_name;type:varchar(128)"`
	SystemDiscount float64   `json:"system_discount" gorm:"column:system_discount;type:decimal(10,4)"`
	Priority       int       `json:"priority" gorm:"column:priority"`
	Enabled        int       `json:"enabled" gorm:"column:enabled"`
	Remark         *string   `json:"remark" gorm:"column:remark;type:varchar(500)"`
	CreateTime     time.Time `json:"create_time" gorm:"column:create_time"`
	UpdateTime     time.Time `json:"update_time" gorm:"column:update_time"`
}

func (SystemDiscount) TableName() string {
	return "system_discount"
}

var (
	systemDiscountCache     map[string][]*SystemDiscount // key: system_code
	systemDiscountCacheLock sync.RWMutex
	systemDiscountCacheTime time.Time
)

// InitSystemDiscount 初始化系统折扣配置缓存
func InitSystemDiscount() {
	RefreshSystemDiscountCache()
}

// RefreshSystemDiscountCache 刷新系统折扣配置缓存
func RefreshSystemDiscountCache() {
	var discounts []*SystemDiscount
	err := DB.Where("enabled = ?", 1).Order("priority DESC").Find(&discounts).Error
	if err != nil {
		common.SysLog("刷新系统折扣配置缓存失败: " + err.Error())
		return
	}

	newCache := make(map[string][]*SystemDiscount)
	for _, discount := range discounts {
		if newCache[discount.SystemCode] == nil {
			newCache[discount.SystemCode] = make([]*SystemDiscount, 0)
		}
		newCache[discount.SystemCode] = append(newCache[discount.SystemCode], discount)
	}

	systemDiscountCacheLock.Lock()
	systemDiscountCache = newCache
	systemDiscountCacheTime = time.Now()
	systemDiscountCacheLock.Unlock()

	common.SysLog(fmt.Sprintf("系统折扣配置缓存已刷新，共 %d 条", len(discounts)))
}

// GetSystemDiscounts 获取系统所有折扣配置
func GetSystemDiscounts(systemCode string) []*SystemDiscount {
	if systemCode == "" {
		systemCode = "nebula" // 默认系统
	}

	systemDiscountCacheLock.RLock()
	defer systemDiscountCacheLock.RUnlock()

	// 检查缓存是否过期
	if time.Since(systemDiscountCacheTime) > cacheExpireDuration {
		// 异步刷新缓存
		go RefreshSystemDiscountCache()
	}

	return systemDiscountCache[systemCode]
}

// GetSystemDiscount 获取系统销售折扣
// 优先级：模型级 > 厂商级 > 通配符 > 默认值(1.0)
func GetSystemDiscount(systemCode, modelName, vendorName string) float64 {
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

	discounts := GetSystemDiscounts(systemCode)
	if len(discounts) == 0 {
		return 1.0 // 无配置，返回原价
	}

	// 按优先级排序（数字越大优先级越高）
	sortedDiscounts := make([]*SystemDiscount, len(discounts))
	copy(sortedDiscounts, discounts)
	sort.Slice(sortedDiscounts, func(i, j int) bool {
		return sortedDiscounts[i].Priority > sortedDiscounts[j].Priority
	})

	// 1. 模型级匹配（vendor_id和model_name都不为空，且都匹配）
	if modelName != "" && vendorID != nil {
		for _, discount := range sortedDiscounts {
			if discount.VendorID != nil && discount.ModelName != nil &&
				*discount.VendorID == *vendorID && *discount.ModelName == modelName {
				return discount.SystemDiscount
			}
		}
	}

	// 2. 厂商级匹配（vendor_id不为空，model_name为空）
	if vendorID != nil {
		for _, discount := range sortedDiscounts {
			if discount.VendorID != nil &&
				(discount.ModelName == nil || *discount.ModelName == "") &&
				*discount.VendorID == *vendorID {
				return discount.SystemDiscount
			}
		}
	}

	// 3. 通配符匹配（vendor_id为NULL，model_name为空）
	for _, discount := range sortedDiscounts {
		if discount.VendorID == nil &&
			(discount.ModelName == nil || *discount.ModelName == "") {
			return discount.SystemDiscount
		}
	}

	// 4. 无配置，返回原价（1.0）
	return 1.0
}
