package gemini

import (
	"one-api/dto"
	"strconv"

	"github.com/gin-gonic/gin"
)

// GeminiLiveConfig 存储 Gemini Live API 的配置参数
type GeminiLiveConfig struct {
	// 音色和语言
	Voice    string
	Language string

	// 输出模式
	OutputMode string

	// 功能开关
	GoogleSearch   bool
	ProactiveAudio bool
	EmpatheticMode bool

	// 语音识别灵敏度
	StartSensitivity string
	EndSensitivity   string

	// 音频配置
	PrefixPaddingMs   int
	SilenceDurationMs int

	// 上下文配置
	ContextWindowThreshold int
	TargetContextSize      int
}

// ParseGeminiLiveConfig 从 URL 参数解析配置
func ParseGeminiLiveConfig(c *gin.Context) *GeminiLiveConfig {
	config := &GeminiLiveConfig{
		// 默认值
		Voice:                  "Zephyr",
		OutputMode:             "audio_only",
		StartSensitivity:       "low",
		EndSensitivity:         "high",
		PrefixPaddingMs:        0,
		SilenceDurationMs:      0,
		ContextWindowThreshold: 128000,
		TargetContextSize:      102400,
	}

	// 从 URL 参数读取（如果提供则覆盖默认值）
	if voice := c.Query("voice"); voice != "" {
		config.Voice = voice
	}
	if language := c.Query("language"); language != "" {
		config.Language = language
	}
	if outputMode := c.Query("output_mode"); outputMode != "" {
		config.OutputMode = outputMode
	}

	// 布尔参数
	config.GoogleSearch = c.Query("google_search") == "true"
	config.ProactiveAudio = c.Query("proactive_audio") == "true"
	config.EmpatheticMode = c.Query("empathetic_mode") == "true"

	// 灵敏度参数
	if startSens := c.Query("start_sensitivity"); startSens != "" {
		config.StartSensitivity = startSens
	}
	if endSens := c.Query("end_sensitivity"); endSens != "" {
		config.EndSensitivity = endSens
	}

	// 整数参数
	if val, err := ValidateIntParam(c, "prefix_padding_ms", 0, 1000); err == nil && val > 0 {
		config.PrefixPaddingMs = val
	}
	if val, err := ValidateIntParam(c, "silence_duration_ms", 0, 2000); err == nil && val > 0 {
		config.SilenceDurationMs = val
	}
	if val, err := ValidateIntParam(c, "context_window_threshold", 5000, 128000); err == nil && val > 0 {
		config.ContextWindowThreshold = val
	}
	if val, err := ValidateIntParam(c, "target_context_size", 0, 128000); err == nil && val > 0 {
		config.TargetContextSize = val
	}

	return config
}

// ApplyToSetup 将配置应用到 GeminiLiveSetup
func (cfg *GeminiLiveConfig) ApplyToSetup(setup *dto.GeminiLiveSetup) {
	// 初始化 GenerationConfig
	if setup.GenerationConfig == nil {
		setup.GenerationConfig = &dto.GeminiLiveGenerationConfig{}
	}

	// 音色和语言配置
	if cfg.Voice != "" || cfg.Language != "" {
		if setup.GenerationConfig.SpeechConfig == nil {
			setup.GenerationConfig.SpeechConfig = &dto.GeminiLiveSpeechConfig{}
		}

		// 设置音色
		if cfg.Voice != "" {
			if setup.GenerationConfig.SpeechConfig.VoiceConfig == nil {
				setup.GenerationConfig.SpeechConfig.VoiceConfig = &dto.GeminiLiveVoiceConfig{}
			}
			setup.GenerationConfig.SpeechConfig.VoiceConfig.PrebuiltVoiceConfig = &dto.GeminiLivePrebuiltVoiceConfig{
				VoiceName: cfg.Voice,
			}
		}

		// 设置语言
		if cfg.Language != "" {
			setup.GenerationConfig.SpeechConfig.LanguageCode = cfg.Language
		}
	}

	// Google 搜索
	if cfg.GoogleSearch {
		if setup.Tools == nil {
			setup.Tools = &dto.GeminiLiveTool{}
		}
		setup.Tools.GoogleSearch = &dto.GeminiLiveGoogleSearch{}
	}

	// 主动音频和共情模式
	if cfg.ProactiveAudio || cfg.EmpatheticMode {
		setup.Proactivity = &dto.GeminiLiveProactivityConfig{
			ProactiveAudio: cfg.ProactiveAudio,
			EmpatheticMode: cfg.EmpatheticMode,
		}
	}

	// 输出转录
	if cfg.OutputMode == "audio_with_text" {
		setup.OutputAudioTranscription = &dto.GeminiLiveAudioTranscriptionConfig{}
	}

	// 语音识别配置
	setup.RealtimeInputConfig = &dto.GeminiLiveRealtimeInputConfig{
		AutomaticActivityDetection: &dto.GeminiLiveAutomaticActivityDetection{
			Disabled:                 false,
			StartOfSpeechSensitivity: MapSensitivityValue(cfg.StartSensitivity, true),
			EndOfSpeechSensitivity:   MapSensitivityValue(cfg.EndSensitivity, false),
			PrefixPaddingMs:          cfg.PrefixPaddingMs,
			SilenceDurationMs:        cfg.SilenceDurationMs,
		},
	}

	// 上下文配置
	// 注意：Gemini Live API 目前不支持这些字段，保留代码以备将来使用
	// setup.GenerationConfig.InputContextWindowThreshold = &cfg.ContextWindowThreshold
	// setup.GenerationConfig.TargetContextSize = &cfg.TargetContextSize
}

// MergeSessionConfig 合并 session.update 中的配置（覆盖 URL 参数）
func (cfg *GeminiLiveConfig) MergeSessionConfig(session *dto.RealtimeSession) {
	if session == nil {
		return
	}

	// 从 session 中读取音色（如果有）
	if session.Voice != "" {
		cfg.Voice = session.Voice
	}

	// 从 session 中读取其他配置...
	// 注意：OpenAI Realtime API 的 session 结构可能不包含所有这些字段
	// 这里主要是为了向后兼容性
}

// ParseIntWithDefault 解析整数，失败则返回默认值
func ParseIntWithDefault(val string, defaultVal int) int {
	if val == "" {
		return defaultVal
	}
	if parsed, err := strconv.Atoi(val); err == nil {
		return parsed
	}
	return defaultVal
}
