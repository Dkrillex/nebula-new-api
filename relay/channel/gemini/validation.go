package gemini

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// validVoices 支持的音色列表（30种）
var validVoices = map[string]string{
	"Zephyr":        "明快",
	"Kore":          "坚定",
	"Orus":          "坚定",
	"Autonoe":       "明快",
	"Umbriel":       "轻松",
	"Erinome":       "清晰",
	"Laomedeia":     "欢快",
	"Schedar":       "平稳",
	"Achird":        "友好",
	"Sadachbia":     "活泼",
	"Puck":          "欢快",
	"Fenrir":        "兴奋",
	"Aoede":         "轻快",
	"Enceladus":     "气声",
	"Algieba":       "流畅",
	"Algenib":       "沙哑",
	"Achernar":      "柔和",
	"Gacrux":        "成熟",
	"Zubenelgenubi": "随意",
	"Sadaltager":    "博学",
	"Charon":        "信息丰富",
	"Leda":          "青春活力",
	"Callirrhoe":    "轻松愉快",
	"Iapetus":       "清晰明了",
	"Despina":       "流畅自然",
	"Rasalgethi":    "信息丰富",
	"Alnilam":       "坚定有力",
	"Pulcherrima":   "积极向上",
	"Vindemiatrix":  "温柔舒缓",
	"Sulafat":       "温暖舒适",
}

// validLanguages 支持的语言列表（24种）
var validLanguages = map[string]string{
	"ar-EG": "阿拉伯语（埃及语）",
	"de-DE": "德语（德国）",
	"en-US": "英语（美国）",
	"es-US": "西班牙语（美国）",
	"fr-FR": "法语（法国）",
	"hi-IN": "印地语（印度）",
	"id-ID": "印度尼西亚语（印度尼西亚）",
	"it-IT": "意大利语（意大利）",
	"ja-JP": "日语（日本）",
	"ko-KR": "韩语（韩国）",
	"pt-BR": "葡萄牙语（巴西）",
	"ru-RU": "俄语（俄罗斯）",
	"nl-NL": "荷兰语（荷兰）",
	"pl-PL": "波兰语（波兰）",
	"th-TH": "泰语（泰国）",
	"tr-TR": "土耳其语（土耳其）",
	"vi-VN": "越南语（越南）",
	"ro-RO": "罗马尼亚语（罗马尼亚）",
	"uk-UA": "乌克兰语（乌克兰）",
	"bn-BD": "孟加拉语（孟加拉）",
	"en-IN": "英语（印度）",
	"mr-IN": "马拉地语（印度）",
	"ta-IN": "泰米尔语（印度）",
	"te-IN": "泰卢固语（印度）",
}

// IsValidVoice 检查音色是否有效
func IsValidVoice(voice string) bool {
	_, ok := validVoices[voice]
	return ok
}

// IsValidLanguage 检查语言代码是否有效
func IsValidLanguage(language string) bool {
	_, ok := validLanguages[language]
	return ok
}

// IsValidSensitivity 检查灵敏度是否有效
func IsValidSensitivity(sensitivity string) bool {
	validValues := []string{
		"low", "high", // 用户友好的值
		"START_SENSITIVITY_LOW", "START_SENSITIVITY_HIGH",
		"START_SENSITIVITY_UNSPECIFIED",
		"END_SENSITIVITY_LOW", "END_SENSITIVITY_HIGH",
		"END_SENSITIVITY_UNSPECIFIED",
	}
	for _, v := range validValues {
		if sensitivity == v {
			return true
		}
	}
	return false
}

// MapSensitivityValue 将用户友好的灵敏度值映射为 Gemini API 格式
func MapSensitivityValue(sensitivity string, isStart bool) string {
	switch sensitivity {
	case "low":
		if isStart {
			return "START_SENSITIVITY_LOW"
		}
		return "END_SENSITIVITY_LOW"
	case "high":
		if isStart {
			return "START_SENSITIVITY_HIGH"
		}
		return "END_SENSITIVITY_HIGH"
	default:
		// 如果已经是正确格式，直接返回
		return sensitivity
	}
}

// IsValidOutputMode 检查输出模式是否有效
func IsValidOutputMode(mode string) bool {
	return mode == "audio_only" || mode == "audio_with_text"
}

// ValidateIntParam 验证整数参数的范围
func ValidateIntParam(c *gin.Context, param string, min, max int) (int, error) {
	valStr := c.Query(param)
	if valStr == "" {
		return 0, nil // 未提供参数，返回 0（将使用默认值）
	}

	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: must be an integer", param)
	}

	if val < min || val > max {
		return 0, fmt.Errorf("invalid %s: must be between %d and %d", param, min, max)
	}

	return val, nil
}

// ValidateGeminiLiveParams 验证所有 Gemini Live API 参数
func ValidateGeminiLiveParams(c *gin.Context) error {
	// 验证音色
	voice := c.Query("voice")
	if voice != "" && !IsValidVoice(voice) {
		return fmt.Errorf("invalid voice: %s. Must be one of the 30 supported voices", voice)
	}

	// 验证语言
	language := c.Query("language")
	if language != "" && !IsValidLanguage(language) {
		return fmt.Errorf("invalid language: %s. Must be a valid BCP-47 language code", language)
	}

	// 验证开始灵敏度
	startSens := c.Query("start_sensitivity")
	if startSens != "" && !IsValidSensitivity(startSens) {
		return fmt.Errorf("invalid start_sensitivity: %s. Must be 'low' or 'high'", startSens)
	}

	// 验证结束灵敏度
	endSens := c.Query("end_sensitivity")
	if endSens != "" && !IsValidSensitivity(endSens) {
		return fmt.Errorf("invalid end_sensitivity: %s. Must be 'low' or 'high'", endSens)
	}

	// 验证输出模式
	outputMode := c.Query("output_mode")
	if outputMode != "" && !IsValidOutputMode(outputMode) {
		return fmt.Errorf("invalid output_mode: %s. Must be 'audio_only' or 'audio_with_text'", outputMode)
	}

	// 验证前缀内边距（0-1000ms）
	if _, err := ValidateIntParam(c, "prefix_padding_ms", 0, 1000); err != nil {
		return err
	}

	// 验证静默时长（0-2000ms）
	if _, err := ValidateIntParam(c, "silence_duration_ms", 0, 2000); err != nil {
		return err
	}

	// 验证上下文窗口阈值（5000-128000）
	if _, err := ValidateIntParam(c, "context_window_threshold", 5000, 128000); err != nil {
		return err
	}

	// 验证目标上下文大小（0-128000）
	if _, err := ValidateIntParam(c, "target_context_size", 0, 128000); err != nil {
		return err
	}

	return nil
}

// GetValidVoices 获取所有支持的音色列表
func GetValidVoices() map[string]string {
	return validVoices
}

// GetValidLanguages 获取所有支持的语言列表
func GetValidLanguages() map[string]string {
	return validLanguages
}
