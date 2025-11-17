package model_setting

import (
	"net/http"
	"one-api/setting/config"
)

// AliSettings 定义阿里云通义千问模型的配置
type AliSettings struct {
	HeadersSettings         map[string]map[string][]string `json:"model_headers_settings"`
	DefaultMaxTokens        map[string]int                 `json:"default_max_tokens"`
	CacheEnabled            bool                           `json:"cache_enabled"`              // 是否启用缓存功能
	DefaultCachePolicy      string                         `json:"default_cache_policy"`       // 默认缓存策略：implicit/explicit
	DefaultCacheTtl         int                            `json:"default_cache_ttl"`          // 默认缓存过期时间(秒)
	CacheCreationTokenRatio float64                        `json:"cache_creation_token_ratio"` // 缓存创建token计费比例
	CacheHitTokenRatio      float64                        `json:"cache_hit_token_ratio"`      // 缓存命中token计费比例
}

// 默认配置
var defaultAliSettings = AliSettings{
	HeadersSettings:         map[string]map[string][]string{},
	CacheEnabled:            true,       // 默认启用缓存
	DefaultCachePolicy:      "implicit", // 默认隐式缓存
	DefaultCacheTtl:         3600,       // 默认缓存1小时
	CacheCreationTokenRatio: 1.0,        // 缓存创建按100%计费
	CacheHitTokenRatio:      0.2,        // 缓存命中按20%计费
	DefaultMaxTokens: map[string]int{
		"default": 8192,
	},
}

// 全局实例
var aliSettings = defaultAliSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("ali", &aliSettings)
}

// GetAliSettings 获取阿里云配置
func GetAliSettings() *AliSettings {
	// 检查默认最大token数必须包含default键
	if _, ok := aliSettings.DefaultMaxTokens["default"]; !ok {
		aliSettings.DefaultMaxTokens["default"] = 8192
	}
	return &aliSettings
}

func (a *AliSettings) WriteHeaders(originModel string, httpHeader *http.Header) {
	if headers, ok := a.HeadersSettings[originModel]; ok {
		for headerKey, headerValues := range headers {
			httpHeader.Del(headerKey)
			for _, headerValue := range headerValues {
				httpHeader.Add(headerKey, headerValue)
			}
		}
	}
}

func (a *AliSettings) GetDefaultMaxTokens(model string) int {
	if maxTokens, ok := a.DefaultMaxTokens[model]; ok {
		return maxTokens
	}
	return a.DefaultMaxTokens["default"]
}

// GetCacheTokenRatio 获取缓存token计费比例
// 根据缓存策略返回相应的计费比例
func (a *AliSettings) GetCacheTokenRatio(cachePolicy string, isCacheHit bool) float64 {
	if isCacheHit {
		// 缓存命中
		if cachePolicy == "explicit" {
			return 0.1 // 显式缓存命中按10%计费
		}
		return a.CacheHitTokenRatio // 隐式缓存命中按配置比例计费
	} else {
		// 缓存创建
		if cachePolicy == "explicit" {
			return 1.25 // 显式缓存创建按125%计费
		}
		return a.CacheCreationTokenRatio // 隐式缓存创建按配置比例计费
	}
}
