package ratio_setting

import (
	"encoding/json"
	"one-api/common"
	"strings"
	"sync"
)

var defaultCacheRatio = map[string]float64{
	"gpt-4":                               0.5,
	"o1":                                  0.5,
	"o1-2024-12-17":                       0.5,
	"o1-preview-2024-09-12":               0.5,
	"o1-preview":                          0.5,
	"o1-mini-2024-09-12":                  0.5,
	"o1-mini":                             0.5,
	"o3-mini":                             0.5,
	"o3-mini-2025-01-31":                  0.5,
	"gpt-4o-2024-11-20":                   0.5,
	"gpt-4o-2024-08-06":                   0.5,
	"gpt-4o":                              0.5,
	"gpt-4o-mini-2024-07-18":              0.5,
	"gpt-4o-mini":                         0.5,
	"gpt-4o-realtime-preview":             0.5,
	"gpt-4o-mini-realtime-preview":        0.5,
	"gpt-4.5-preview":                     0.5,
	"gpt-4.5-preview-2025-02-27":          0.5,
	"gpt-4.1":                             0.25,
	"gpt-4.1-mini":                        0.25,
	"gpt-4.1-nano":                        0.25,
	"gpt-5":                               0.1,
	"gpt-5-2025-08-07":                    0.1,
	"gpt-5-chat-latest":                   0.1,
	"gpt-5-mini":                          0.1,
	"gpt-5-mini-2025-08-07":               0.1,
	"gpt-5-nano":                          0.1,
	"gpt-5-nano-2025-08-07":               0.1,
	"deepseek-chat":                       0.25,
	"deepseek-reasoner":                   0.25,
	"deepseek-coder":                      0.25,
	"claude-3-sonnet-20240229":            0.1,
	"claude-3-opus-20240229":              0.1,
	"claude-3-haiku-20240307":             0.1,
	"claude-3-5-haiku-20241022":           0.1,
	"claude-3-5-sonnet-20240620":          0.1,
	"claude-3-5-sonnet-20241022":          0.1,
	"claude-3-7-sonnet-20250219":          0.1,
	"claude-3-7-sonnet-20250219-thinking": 0.1,
	"claude-sonnet-4-20250514":            0.1,
	"claude-sonnet-4-20250514-thinking":   0.1,
	"claude-opus-4-20250514":              0.1,
	"claude-opus-4-20250514-thinking":     0.1,
	"claude-opus-4-1-20250805":            0.1,
	"claude-opus-4-1-20250805-thinking":   0.1,
	"claude-haiku-4-5-20251001":           0.1,
	"claude-haiku-4-5-20251001-thinking":  0.1,
	// 阿里云通义千问模型缓存比例（隐式缓存命中20%，显式缓存命中10%）
	"qwen-turbo":           0.2,
	"qwen-plus":            0.2,
	"qwen-max":             0.2,
	"qwen-max-1201":        0.2,
	"qwen-max-0403":        0.2,
	"qwen-max-0107":        0.2,
	"qwen-max-longcontext": 0.2,
	"qwen-vl-max":          0.2,
	"qwen-vl-plus":         0.2,
	"qwen-audio-turbo":     0.2,
	"qwen-7b":              0.2,
	"qwen-14b":             0.2,
	"qwen-72b":             0.2,
	"qwen-1.8b":            0.2,
	"qwen-2.5b":            0.2,
}

var defaultCreateCacheRatio = map[string]float64{
	"claude-3-sonnet-20240229":            1.25,
	"claude-3-opus-20240229":              1.25,
	"claude-3-haiku-20240307":             1.25,
	"claude-3-5-haiku-20241022":           1.25,
	"claude-3-5-sonnet-20240620":          1.25,
	"claude-3-5-sonnet-20241022":          1.25,
	"claude-3-7-sonnet-20250219":          1.25,
	"claude-3-7-sonnet-20250219-thinking": 1.25,
	"claude-sonnet-4-20250514":            1.25,
	"claude-sonnet-4-20250514-thinking":   1.25,
	"claude-opus-4-20250514":              1.25,
	"claude-opus-4-20250514-thinking":     1.25,
	"claude-opus-4-1-20250805":            1.25,
	"claude-opus-4-1-20250805-thinking":   1.25,
	"claude-sonnet-4-5-20250929":          1.25,
	"claude-sonnet-4-5-20250929-thinking": 1.25,
	"claude-haiku-4-5-20251001":           1.25,
	"claude-haiku-4-5-20251001-thinking":  1.25,
	// 阿里云通义千问显式缓存创建比例（125%），隐式缓存创建使用默认值1.0
	"qwen-turbo":           1.25,
	"qwen-plus":            1.25,
	"qwen-max":             1.25,
	"qwen-max-1201":        1.25,
	"qwen-max-0403":        1.25,
	"qwen-max-0107":        1.25,
	"qwen-max-longcontext": 1.25,
	"qwen-vl-max":          1.25,
	"qwen-vl-plus":         1.25,
	"qwen-audio-turbo":     1.25,
	"qwen-7b":              1.25,
	"qwen-14b":             1.25,
	"qwen-72b":             1.25,
	"qwen-1.8b":            1.25,
	"qwen-2.5b":            1.25,
}

//var defaultCreateCacheRatio = map[string]float64{}

var cacheRatioMap map[string]float64
var cacheRatioMapMutex sync.RWMutex

// GetCacheRatioMap returns the cache ratio map
func GetCacheRatioMap() map[string]float64 {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	return cacheRatioMap
}

// CacheRatio2JSONString converts the cache ratio map to a JSON string
func CacheRatio2JSONString() string {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	jsonBytes, err := json.Marshal(cacheRatioMap)
	if err != nil {
		common.SysLog("error marshalling cache ratio: " + err.Error())
	}
	return string(jsonBytes)
}

// UpdateCacheRatioByJSONString updates the cache ratio map from a JSON string
func UpdateCacheRatioByJSONString(jsonStr string) error {
	cacheRatioMapMutex.Lock()
	defer cacheRatioMapMutex.Unlock()
	cacheRatioMap = make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &cacheRatioMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// GetCacheRatio returns the cache ratio for a model
func GetCacheRatio(name string) (float64, bool) {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	ratio, ok := cacheRatioMap[name]
	if !ok {
		// 动态兜底逻辑
		if strings.Contains(strings.ToLower(name), "claude") {
			return 0.1, false // Claude 系列默认 0.1
		} else if strings.Contains(strings.ToLower(name), "gpt") {
			return 0.12, false // GPT 系列默认 0.12
		} else if strings.Contains(strings.ToLower(name), "gemini") {
			return 0.1, false // Gemini 系列默认 0.1
		}
		return 0.5, false // 其他模型默认 0.5
	}
	return ratio, true
}

func GetCreateCacheRatio(name string) (float64, bool) {
	ratio, ok := defaultCreateCacheRatio[name]
	if !ok {
		return 1.25, false // Default to 1.25 if not found
	}
	return ratio, true
}

func GetCacheRatioCopy() map[string]float64 {
	cacheRatioMapMutex.RLock()
	defer cacheRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(cacheRatioMap))
	for k, v := range cacheRatioMap {
		copyMap[k] = v
	}
	return copyMap
}
