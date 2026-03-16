package ratio_setting

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

const exposedDataTTL = 30 * time.Second

type exposedCache struct {
	data      gin.H
	expiresAt time.Time
}

var (
	exposedData atomic.Value
	rebuildMu   sync.Mutex
)

func InvalidateExposedDataCache() {
	exposedData.Store((*exposedCache)(nil))
}

// RefreshExposedDataCache 立即删除并重建 exposed 缓存
// 用于模型数据更新后立即刷新缓存，确保下次访问时获取最新数据
func RefreshExposedDataCache() {
	rebuildMu.Lock()
	defer rebuildMu.Unlock()

	newData := gin.H{
		"model_ratio":                        GetModelRatioCopy(),
		"completion_ratio":                   GetCompletionRatioCopy(),
		"cache_ratio":                        GetCacheRatioCopy(),
		"model_price":                        GetModelPriceCopy(),
		"image_model_price_per_image":        GetImageModelPricePerImageCopy(),
		"origin_image_model_price_per_image": GetOriginImageModelPricePerImageCopy(),
	}
	exposedData.Store(&exposedCache{
		data:      newData,
		expiresAt: time.Now().Add(exposedDataTTL),
	})
}

func cloneGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func GetExposedData() gin.H {
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	rebuildMu.Lock()
	defer rebuildMu.Unlock()
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	newData := gin.H{
		"model_ratio":                        GetModelRatioCopy(),
		"completion_ratio":                   GetCompletionRatioCopy(),
		"cache_ratio":                        GetCacheRatioCopy(),
		"model_price":                        GetModelPriceCopy(),
		"image_model_price_per_image":        GetImageModelPricePerImageCopy(),
		"origin_image_model_price_per_image": GetOriginImageModelPricePerImageCopy(),
	}
	exposedData.Store(&exposedCache{
		data:      newData,
		expiresAt: time.Now().Add(exposedDataTTL),
	})
	return cloneGinH(newData)
}
