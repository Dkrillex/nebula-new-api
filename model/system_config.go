package model

import (
	"fmt"
	"one-api/common"
	"strings"
	"sync"
	"time"
)

type SystemConfig struct {
	Id                  int64     `json:"id" gorm:"primaryKey"`
	SystemCode          string    `json:"system_code" gorm:"column:system_code;type:varchar(32);uniqueIndex"`
	SystemName          string    `json:"system_name" gorm:"column:system_name;type:varchar(64)"`
	ApiPrefix           string    `json:"api_prefix" gorm:"column:api_prefix;type:varchar(64);uniqueIndex"`
	ApiPrefixAlias      *string   `json:"api_prefix_alias" gorm:"column:api_prefix_alias;type:varchar(255)"`
	Domain              *string   `json:"domain" gorm:"column:domain;type:varchar(255)"`
	LogoUrl             *string   `json:"logo_url" gorm:"column:logo_url;type:varchar(255)"`
	CompanyName         *string   `json:"company_name" gorm:"column:company_name;type:varchar(128)"`
	ContactEmail        *string   `json:"contact_email" gorm:"column:contact_email;type:varchar(128)"`
	SystemAccountUserId *int64    `json:"system_account_user_id" gorm:"column:system_account_user_id"`
	FeatureFlags        *string   `json:"feature_flags" gorm:"column:feature_flags;type:json"`
	Enabled             int       `json:"enabled" gorm:"column:enabled"`
	Remark              *string   `json:"remark" gorm:"column:remark;type:varchar(500)"`
	CreateTime          time.Time `json:"create_time" gorm:"column:create_time"`
	UpdateTime          time.Time `json:"update_time" gorm:"column:update_time"`
}

func (SystemConfig) TableName() string {
	return "system_config"
}

var (
	systemConfigCache     map[string]*SystemConfig // key: system_code
	systemConfigCacheLock sync.RWMutex
	systemConfigCacheTime time.Time
)

// InitSystemConfig 初始化系统配置缓存
func InitSystemConfig() {
	RefreshSystemConfigCache()
}

// RefreshSystemConfigCache 刷新系统配置缓存
func RefreshSystemConfigCache() {
	var configs []*SystemConfig
	err := DB.Where("enabled = ?", 1).Find(&configs).Error
	if err != nil {
		common.SysLog("刷新系统配置缓存失败: " + err.Error())
		return
	}

	newCache := make(map[string]*SystemConfig)
	for _, config := range configs {
		newCache[config.SystemCode] = config
	}

	systemConfigCacheLock.Lock()
	systemConfigCache = newCache
	systemConfigCacheTime = time.Now()
	systemConfigCacheLock.Unlock()

	common.SysLog(fmt.Sprintf("系统配置缓存已刷新，共 %d 条", len(configs)))
}

// GetSystemConfigByCode 根据系统代码获取系统配置
func GetSystemConfigByCode(systemCode string) *SystemConfig {
	if systemCode == "" {
		systemCode = "nebula" // 默认系统
	}

	systemConfigCacheLock.RLock()
	defer systemConfigCacheLock.RUnlock()

	// 检查缓存是否过期
	if time.Since(systemConfigCacheTime) > cacheExpireDuration {
		// 异步刷新缓存
		go RefreshSystemConfigCache()
	}

	return systemConfigCache[systemCode]
}

// GetSystemConfigByApiPrefix 根据API前缀获取系统配置
func GetSystemConfigByApiPrefix(apiPrefix string) *SystemConfig {
	systemConfigCacheLock.RLock()
	defer systemConfigCacheLock.RUnlock()

	// 检查缓存是否过期
	if time.Since(systemConfigCacheTime) > cacheExpireDuration {
		// 异步刷新缓存
		go RefreshSystemConfigCache()
	}

	for _, config := range systemConfigCache {
		if config.ApiPrefix == apiPrefix {
			return config
		}
	}

	return nil
}

// GetSystemConfigByApiPrefixAlias 根据API前缀别名获取系统配置
func GetSystemConfigByApiPrefixAlias(apiPrefixAlias string) *SystemConfig {
	systemConfigCacheLock.RLock()
	defer systemConfigCacheLock.RUnlock()

	// 检查缓存是否过期
	if time.Since(systemConfigCacheTime) > cacheExpireDuration {
		// 异步刷新缓存
		go RefreshSystemConfigCache()
	}

	for _, config := range systemConfigCache {
		if config.ApiPrefixAlias != nil {
			// 检查别名是否匹配（支持逗号分隔的多个别名）
			aliases := strings.Split(*config.ApiPrefixAlias, ",")
			for _, alias := range aliases {
				if strings.TrimSpace(alias) == apiPrefixAlias {
					return config
				}
			}
		}
	}

	return nil
}
