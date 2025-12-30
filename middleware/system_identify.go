package middleware

import (
	"fmt"
	"one-api/common"
	"one-api/constant"
	"one-api/model"
	"strings"

	"github.com/gin-gonic/gin"
)

// SystemIdentify OEM识别中间件
// 从请求Header或URI中提取oem_code，并加载OemConfig到Context
func SystemIdentify() gin.HandlerFunc {
	return func(c *gin.Context) {
		var oemCode string
		var oemConfig *model.OemConfig

		// 1. 优先从Nginx传递的Header获取（支持X-Oem-Code和X-System-Code）
		oemCode = c.GetHeader("X-Oem-Code")
		if oemCode == "" {
			oemCode = c.GetHeader("X-System-Code") // 向后兼容
		}

		// 2. 如果Header没有，从请求路径中提取
		if oemCode == "" {
			requestURI := c.Request.RequestURI
			oemCode = extractOemCodeFromPath(requestURI)
		}

		// 3. 如果还是没有，使用默认系统
		if oemCode == "" {
			oemCode = "nebula"
			common.SysLog("未找到OEM代码，使用默认值: nebula")
		}

		// 4. 获取OEM配置
		oemConfig = model.GetOemConfigByCode(oemCode)
		if oemConfig == nil {
			common.SysLog("OEM配置不存在: oemCode=" + oemCode + ", 使用默认nebula系统")
			oemCode = "nebula"
			oemConfig = model.GetOemConfigByCode(oemCode)
		}

		if oemConfig != nil && oemConfig.Enabled == 0 {
			common.SysLog("OEM已停用: oemCode=" + oemCode)
			// 可以选择使用默认系统或返回错误
			oemCode = "nebula"
			oemConfig = model.GetOemConfigByCode(oemCode)
		}

		// 5. 存储到Context
		common.SetContextKey(c, constant.ContextKeyOemCode, oemCode)
		// 向后兼容：同时设置SystemCode
		common.SetContextKey(c, constant.ContextKeySystemCode, oemCode)

		if oemConfig != nil {
			oemId := oemConfig.Id
			common.SetContextKey(c, constant.ContextKeyOemId, oemId)
			common.SetContextKey(c, constant.ContextKeyOemConfig, oemConfig)
			// 向后兼容：同时设置SystemConfig（使用向后兼容函数）
			common.SetContextKey(c, constant.ContextKeySystemConfig, oemConfig)
		}

		if oemConfig != nil {
			common.SysLog(fmt.Sprintf("OEM识别成功: oemCode=%s, oemId=%d, apiPrefix=%s", oemCode, oemConfig.Id, oemConfig.ApiPrefix))
		}

		c.Next()
	}
}

// extractOemCodeFromPath 从请求路径中提取OEM代码
func extractOemCodeFromPath(requestURI string) string {
	if requestURI == "" {
		return ""
	}

	// 移除开头的斜杠和查询参数
	path := strings.TrimPrefix(requestURI, "/")
	if idx := strings.Index(path, "?"); idx != -1 {
		path = path[:idx]
	}

	// 分割路径
	segments := strings.Split(path, "/")
	if len(segments) < 1 {
		return ""
	}

	firstSegment := segments[0]

	// 特殊处理：/prod-api 映射到 nebula
	if firstSegment == "prod-api" {
		return "nebula"
	}

	// 通过API前缀查询OEM代码
	apiPrefix := "/" + firstSegment
	oemConfig := model.GetOemConfigByApiPrefix(apiPrefix)
	if oemConfig != nil {
		return oemConfig.OemCode
	}

	// 尝试通过别名查询
	oemConfig = model.GetOemConfigByApiPrefixAlias(apiPrefix)
	if oemConfig != nil {
		return oemConfig.OemCode
	}

	return ""
}
