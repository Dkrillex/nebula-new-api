package ratio_setting

import (
	"encoding/json"
	"one-api/common"
	"one-api/setting/operation_setting"
	"strconv"
	"strings"
	"sync"
)

// ImageTokenPricing 图像Token表定价结构
// 用于 gpt-image-1 等特殊图像模型，支持多价格和固定Token表
type ImageTokenPricing struct {
	InputTextPrice   float64                   `json:"input_text_price"`   // 输入文本价格（$/1M tokens）
	InputImagePrice  float64                   `json:"input_image_price"`  // 输入图像价格（$/1M tokens）
	OutputImagePrice float64                   `json:"output_image_price"` // 输出图像价格（$/1M tokens）
	TokenTable       map[string]map[string]int `json:"token_table"`        // Token表：quality -> size -> tokens
}

// from songquanpeng/one-api
const (
	USD2RMB = 7.0 // 暂定 1 USD = 7.0 RMB
	USD     = 500 // $0.002 = 1 -> $1 = 500
	RMB     = USD / USD2RMB
)

// modelRatio
// https://platform.openai.com/docs/models/model-endpoint-compatibility
// https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9dlf
// https://openai.com/pricing
// TODO: when a new api is enabled, check the pricing here
// 1 === $0.002 / 1K tokens
// 1 === ￥0.014 / 1k tokens

var defaultModelRatio = map[string]float64{
	//"midjourney":                50,
	"gpt-4-gizmo-*":  15,
	"gpt-4o-gizmo-*": 2.5,
	"gpt-4-all":      15,
	"gpt-4o-all":     15,
	"gpt-4":          15,
	//"gpt-4-0314":                   15, //deprecated
	"gpt-4-0613": 15,
	"gpt-4-32k":  30,
	//"gpt-4-32k-0314":               30, //deprecated
	"gpt-4-32k-0613":                          30,
	"gpt-4-1106-preview":                      5,    // $10 / 1M tokens
	"gpt-4-0125-preview":                      5,    // $10 / 1M tokens
	"gpt-4-turbo-preview":                     5,    // $10 / 1M tokens
	"gpt-4-vision-preview":                    5,    // $10 / 1M tokens
	"gpt-4-1106-vision-preview":               5,    // $10 / 1M tokens
	"chatgpt-4o-latest":                       2.5,  // $5 / 1M tokens
	"gpt-4o":                                  1.25, // $2.5 / 1M tokens
	"gpt-4o-audio-preview":                    1.25, // $2.5 / 1M tokens
	"gpt-4o-audio-preview-2024-10-01":         1.25, // $2.5 / 1M tokens
	"gpt-4o-2024-05-13":                       2.5,  // $5 / 1M tokens
	"gpt-4o-2024-08-06":                       1.25, // $2.5 / 1M tokens
	"gpt-4o-2024-11-20":                       1.25, // $2.5 / 1M tokens
	"gpt-4o-realtime-preview":                 2.5,
	"gpt-4o-realtime-preview-2024-10-01":      2.5,
	"gpt-4o-realtime-preview-2024-12-17":      2.5,
	"gpt-4o-mini-realtime-preview":            0.3,
	"gpt-4o-mini-realtime-preview-2024-12-17": 0.3,
	"gpt-4.1":                          1.0,  // $2 / 1M tokens
	"gpt-4.1-2025-04-14":               1.0,  // $2 / 1M tokens
	"gpt-4.1-mini":                     0.2,  // $0.4 / 1M tokens
	"gpt-4.1-mini-2025-04-14":          0.2,  // $0.4 / 1M tokens
	"gpt-4.1-nano":                     0.05, // $0.1 / 1M tokens
	"gpt-4.1-nano-2025-04-14":          0.05, // $0.1 / 1M tokens
	"gpt-image-1":                      2.5,  // $5 / 1M tokens
	"gpt-image-1-mini":                 1.0,  // $2 / 1M tokens (更经济的版本)
	"o1":                               7.5,  // $15 / 1M tokens
	"o1-2024-12-17":                    7.5,  // $15 / 1M tokens
	"o1-preview":                       7.5,  // $15 / 1M tokens
	"o1-preview-2024-09-12":            7.5,  // $15 / 1M tokens
	"o1-mini":                          0.55, // $1.1 / 1M tokens
	"o1-mini-2024-09-12":               0.55, // $1.1 / 1M tokens
	"o1-pro":                           75.0, // $150 / 1M tokens
	"o1-pro-2025-03-19":                75.0, // $150 / 1M tokens
	"o3-mini":                          0.55,
	"o3-mini-2025-01-31":               0.55,
	"o3-mini-high":                     0.55,
	"o3-mini-2025-01-31-high":          0.55,
	"o3-mini-low":                      0.55,
	"o3-mini-2025-01-31-low":           0.55,
	"o3-mini-medium":                   0.55,
	"o3-mini-2025-01-31-medium":        0.55,
	"o3":                               1.0,  // $2 / 1M tokens
	"o3-2025-04-16":                    1.0,  // $2 / 1M tokens
	"o3-pro":                           10.0, // $20 / 1M tokens
	"o3-pro-2025-06-10":                10.0, // $20 / 1M tokens
	"o3-deep-research":                 5.0,  // $10 / 1M tokens
	"o3-deep-research-2025-06-26":      5.0,  // $10 / 1M tokens
	"o4-mini":                          0.55, // $1.1 / 1M tokens
	"o4-mini-2025-04-16":               0.55, // $1.1 / 1M tokens
	"o4-mini-deep-research":            1.0,  // $2 / 1M tokens
	"o4-mini-deep-research-2025-06-26": 1.0,  // $2 / 1M tokens
	"gpt-4o-mini":                      0.075,
	"gpt-4o-mini-2024-07-18":           0.075,
	"gpt-4-turbo":                      5, // $0.01 / 1K tokens
	"gpt-4-turbo-2024-04-09":           5, // $0.01 / 1K tokens
	"gpt-4.5-preview":                  37.5,
	"gpt-4.5-preview-2025-02-27":       37.5,
	"gpt-5":                            0.625,
	"gpt-5-2025-08-07":                 0.625,
	"gpt-5-chat-latest":                0.625,
	"gpt-5-mini":                       0.125,
	"gpt-5-mini-2025-08-07":            0.125,
	"gpt-5-nano":                       0.025,
	"gpt-5-nano-2025-08-07":            0.025,
	//"gpt-3.5-turbo-0301":           0.75, //deprecated
	"gpt-3.5-turbo":          0.25,
	"gpt-3.5-turbo-0613":     0.75,
	"gpt-3.5-turbo-16k":      1.5, // $0.003 / 1K tokens
	"gpt-3.5-turbo-16k-0613": 1.5,
	"gpt-3.5-turbo-instruct": 0.75, // $0.0015 / 1K tokens
	"gpt-3.5-turbo-1106":     0.5,  // $0.001 / 1K tokens
	"gpt-3.5-turbo-0125":     0.25,
	"babbage-002":            0.2, // $0.0004 / 1K tokens
	"davinci-002":            1,   // $0.002 / 1K tokens
	"text-ada-001":           0.2,
	"text-babbage-001":       0.25,
	"text-curie-001":         1,
	//"text-davinci-002":               10,
	//"text-davinci-003":               10,
	"text-davinci-edit-001":                     10,
	"code-davinci-edit-001":                     10,
	"whisper-1":                                 15,  // $0.006 / minute -> $0.006 / 150 words -> $0.006 / 200 tokens -> $0.03 / 1k tokens
	"tts-1":                                     7.5, // 1k characters -> $0.015
	"tts-1-1106":                                7.5, // 1k characters -> $0.015
	"tts-1-hd":                                  15,  // 1k characters -> $0.03
	"tts-1-hd-1106":                             15,  // 1k characters -> $0.03
	"davinci":                                   10,
	"curie":                                     10,
	"babbage":                                   10,
	"ada":                                       10,
	"text-embedding-3-small":                    0.01,
	"text-embedding-3-large":                    0.065,
	"text-embedding-ada-002":                    0.05,
	"text-search-ada-doc-001":                   10,
	"text-moderation-stable":                    0.1,
	"text-moderation-latest":                    0.1,
	"claude-instant-1":                          0.4,   // $0.8 / 1M tokens
	"claude-2.0":                                4,     // $8 / 1M tokens
	"claude-2.1":                                4,     // $8 / 1M tokens
	"claude-3-haiku-20240307":                   0.125, // $0.25 / 1M tokens
	"claude-3-5-haiku-20241022":                 0.5,   // $1 / 1M tokens
	"claude-haiku-4-5-20251001":                 0.55,  // $1.1 / 1M tokens
	"claude-3-sonnet-20240229":                  1.5,   // $3 / 1M tokens
	"claude-3-5-sonnet-20240620":                1.5,
	"claude-3-5-sonnet-20241022":                1.5,
	"claude-3-7-sonnet-20250219":                1.5,
	"claude-3-7-sonnet-20250219-thinking":       1.5,
	"claude-sonnet-4-20250514":                  1.5,
	"claude-sonnet-4-5-20250929":                1.5,
	"claude-3-opus-20240229":                    7.5, // $15 / 1M tokens
	"claude-opus-4-20250514":                    7.5,
	"claude-opus-4-1-20250805":                  7.5,
	"ERNIE-4.0-8K":                              0.120 * RMB,
	"ERNIE-3.5-8K":                              0.012 * RMB,
	"ERNIE-3.5-8K-0205":                         0.024 * RMB,
	"ERNIE-3.5-8K-1222":                         0.012 * RMB,
	"ERNIE-Bot-8K":                              0.024 * RMB,
	"ERNIE-3.5-4K-0205":                         0.012 * RMB,
	"ERNIE-Speed-8K":                            0.004 * RMB,
	"ERNIE-Speed-128K":                          0.004 * RMB,
	"ERNIE-Lite-8K-0922":                        0.008 * RMB,
	"ERNIE-Lite-8K-0308":                        0.003 * RMB,
	"ERNIE-Tiny-8K":                             0.001 * RMB,
	"BLOOMZ-7B":                                 0.004 * RMB,
	"Embedding-V1":                              0.002 * RMB,
	"bge-large-zh":                              0.002 * RMB,
	"bge-large-en":                              0.002 * RMB,
	"tao-8k":                                    0.002 * RMB,
	"PaLM-2":                                    1,
	"gemini-1.5-pro-latest":                     1.25, // $3.5 / 1M tokens
	"gemini-1.5-flash-latest":                   0.075,
	"gemini-2.0-flash":                          0.05,
	"gemini-2.5-pro-exp-03-25":                  0.625,
	"gemini-2.5-pro-preview-03-25":              0.625,
	"gemini-2.5-pro":                            0.625,
	"gemini-2.5-flash-preview-04-17":            0.075,
	"gemini-2.5-flash-preview-04-17-thinking":   0.075,
	"gemini-2.5-flash-preview-04-17-nothinking": 0.075,
	"gemini-2.5-flash-preview-05-20":            0.075,
	"gemini-2.5-flash-preview-05-20-thinking":   0.075,
	"gemini-2.5-flash-preview-05-20-nothinking": 0.075,
	"gemini-2.5-flash-thinking-*":               0.075, // 用于为后续所有2.5 flash thinking budget 模型设置默认倍率
	"gemini-2.5-pro-thinking-*":                 0.625, // 用于为后续所有2.5 pro thinking budget 模型设置默认倍率
	"gemini-2.5-flash-lite-preview-thinking-*":  0.05,
	"gemini-2.5-flash-lite-preview-06-17":       0.05,
	"gemini-2.5-flash":                          0.15,
	"gemini-robotics-er-1.5-preview":            0.15,
	"gemini-embedding-001":                      0.075,
	"text-embedding-004":                        0.001,
	// Gemini Live API models
	"gemini-live-2.5-flash-native-audio":                 0.15,  // 基于 gemini-2.5-flash 价格
	"gemini-live-2.5-flash-preview-native-audio-09-2025": 0.075, // 预览版价格
	"gemini-2.5-flash-native-audio-preview-12-2025":      0.075, // 预览版价格
	"chatglm_turbo":                  0.3572,     // ￥0.005 / 1k tokens
	"chatglm_pro":                    0.7143,     // ￥0.01 / 1k tokens
	"chatglm_std":                    0.3572,     // ￥0.005 / 1k tokens
	"chatglm_lite":                   0.1429,     // ￥0.002 / 1k tokens
	"glm-4":                          7.143,      // ￥0.1 / 1k tokens
	"glm-4v":                         0.05 * RMB, // ￥0.05 / 1k tokens
	"glm-4-alltools":                 0.1 * RMB,  // ￥0.1 / 1k tokens
	"glm-3-turbo":                    0.3572,
	"glm-4-plus":                     0.05 * RMB,
	"glm-4-0520":                     0.1 * RMB,
	"glm-4-air":                      0.001 * RMB,
	"glm-4-airx":                     0.01 * RMB,
	"glm-4-long":                     0.001 * RMB,
	"glm-4-flash":                    0,
	"glm-4v-plus":                    0.01 * RMB,
	"qwen-turbo":                     0.8572, // ￥0.012 / 1k tokens
	"qwen-plus":                      10,     // ￥0.14 / 1k tokens
	"text-embedding-v1":              0.05,   // ￥0.0007 / 1k tokens
	"SparkDesk-v1.1":                 1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v2.1":                 1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v3.1":                 1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v3.5":                 1.2858, // ￥0.018 / 1k tokens
	"SparkDesk-v4.0":                 1.2858,
	"360GPT_S2_V9":                   0.8572, // ¥0.012 / 1k tokens
	"360gpt-turbo":                   0.0858, // ¥0.0012 / 1k tokens
	"360gpt-turbo-responsibility-8k": 0.8572, // ¥0.012 / 1k tokens
	"360gpt-pro":                     0.8572, // ¥0.012 / 1k tokens
	"360gpt2-pro":                    0.8572, // ¥0.012 / 1k tokens
	"embedding-bert-512-v1":          0.0715, // ¥0.001 / 1k tokens
	"embedding_s1_v1":                0.0715, // ¥0.001 / 1k tokens
	"semantic_similarity_s1_v1":      0.0715, // ¥0.001 / 1k tokens
	"hunyuan":                        7.143,  // ¥0.1 / 1k tokens  // https://cloud.tencent.com/document/product/1729/97731#e0e6be58-60c8-469f-bdeb-6c264ce3b4d0
	// https://platform.lingyiwanwu.com/docs#-计费单元
	// 已经按照 7.2 来换算美元价格
	"yi-34b-chat-0205":       0.18,
	"yi-34b-chat-200k":       0.864,
	"yi-vl-plus":             0.432,
	"yi-large":               20.0 / 1000 * RMB,
	"yi-medium":              2.5 / 1000 * RMB,
	"yi-vision":              6.0 / 1000 * RMB,
	"yi-medium-200k":         12.0 / 1000 * RMB,
	"yi-spark":               1.0 / 1000 * RMB,
	"yi-large-rag":           25.0 / 1000 * RMB,
	"yi-large-turbo":         12.0 / 1000 * RMB,
	"yi-large-preview":       20.0 / 1000 * RMB,
	"yi-large-rag-preview":   25.0 / 1000 * RMB,
	"command":                0.5,
	"command-nightly":        0.5,
	"command-light":          0.5,
	"command-light-nightly":  0.5,
	"command-r":              0.25,
	"command-r-plus":         1.5,
	"command-r-08-2024":      0.075,
	"command-r-plus-08-2024": 1.25,
	"deepseek-chat":          0.27 / 2,
	"deepseek-coder":         0.27 / 2,
	"deepseek-reasoner":      0.55 / 2, // 0.55 / 1k tokens
	// Perplexity online 模型对搜索额外收费，有需要应自行调整，此处不计入搜索费用
	"llama-3-sonar-small-32k-chat":   0.2 / 1000 * USD,
	"llama-3-sonar-small-32k-online": 0.2 / 1000 * USD,
	"llama-3-sonar-large-32k-chat":   1.0 / 1000 * USD,
	"llama-3-sonar-large-32k-online": 1.0 / 1000 * USD,
	// grok
	"grok-3-beta":           1.5,
	"grok-3-mini-beta":      0.15,
	"grok-2":                1,
	"grok-2-vision":         1,
	"grok-beta":             2.5,
	"grok-vision-beta":      2.5,
	"grok-3-fast-beta":      2.5,
	"grok-3-mini-fast-beta": 0.3,
	// submodel
	"NousResearch/Hermes-4-405B-FP8":          0.8,
	"Qwen/Qwen3-235B-A22B-Thinking-2507":      0.6,
	"Qwen/Qwen3-Coder-480B-A35B-Instruct-FP8": 0.8,
	"Qwen/Qwen3-235B-A22B-Instruct-2507":      0.3,
	"zai-org/GLM-4.5-FP8":                     0.8,
	"openai/gpt-oss-120b":                     0.5,
	"deepseek-ai/DeepSeek-R1-0528":            0.8,
	"deepseek-ai/DeepSeek-R1":                 0.8,
	"deepseek-ai/DeepSeek-V3-0324":            0.8,
	"deepseek-ai/DeepSeek-V3.1":               0.8,
}

var defaultModelPrice = map[string]float64{
	"suno_music":              0.1,
	"suno_lyrics":             0.01,
	"dall-e-3":                0.04,
	"imagen-3.0-generate-002": 0.03,
	"gpt-4-gizmo-*":           0.1,
	"mj_video":                0.8,
	"mj_imagine":              0.1,
	"mj_edits":                0.1,
	"mj_variation":            0.1,
	"mj_reroll":               0.1,
	"mj_blend":                0.1,
	"mj_modal":                0.1,
	"mj_zoom":                 0.1,
	"mj_shorten":              0.1,
	"mj_high_variation":       0.1,
	"mj_low_variation":        0.1,
	"mj_pan":                  0.1,
	"mj_inpaint":              0,
	"mj_custom_zoom":          0,
	"mj_describe":             0.05,
	"mj_upscale":              0.05,
	"swap_face":               0.05,
	"mj_upload":               0.05,
}

var defaultVideoModelPricePerSecond = map[string]float64{
	"sora-2":                        0.1,  // $0.1/秒
	"veo-3.0-generate-001":          0.15, // $0.15/秒,如果有音频是$0.4/秒
	"veo-3.0-fast-generate-001":     0.15, // $0.15/秒
	"veo-3.1-fast-generate-preview": 0.15, // $0.15/秒
	"veo-3.0-generate-preview":      0.2,  // $0.2/秒,如果有音频是$0.4/秒
	"veo-3.1-generate-preview":      0.2,  // $0.2/秒,如果有音频是$0.4/秒
}

// defaultVideoModelPricePerSecondWithResolutions 包含带分辨率配置的默认视频模型价格
var defaultVideoModelPricePerSecondWithResolutions = map[string]interface{}{
	"wan2.5-i2v-preview": map[string]interface{}{
		"default": 0.0738,
		"resolutions": map[string]interface{}{
			"480p":  0.0369,
			"720p":  0.0738,
			"1080p": 0.1233,
		},
	},
	"wan2.5-t2v-preview": map[string]interface{}{
		"default": 0.0738,
		"resolutions": map[string]interface{}{
			"480p":  0.0369,
			"720p":  0.0738,
			"1080p": 0.1233,
		},
	},
}

type VideoAudioPricing struct {
	NoAudio float64 `json:"noAudio,omitempty"`
	Audio   float64 `json:"audio,omitempty"`
}

var defaultVideoAudioPricing = map[string]VideoAudioPricing{
	"veo-3.0-generate-preview": {
		NoAudio: 0.2,
		Audio:   0.4,
	},
	"veo-3.1-generate-preview": {
		NoAudio: 0.2,
		Audio:   0.4,
	},
}

var defaultAudioRatio = map[string]float64{
	"gpt-4o-audio-preview":                               16,
	"gpt-4o-mini-audio-preview":                          66.67,
	"gpt-4o-realtime-preview":                            8,
	"gpt-4o-mini-realtime-preview":                       16.67,
	"gemini-live-2.5-flash-native-audio":                 4, // $0.20/百万tokens / $0.05基准价格 = 4倍
	"gemini-live-2.5-flash-preview-native-audio-09-2025": 4, // 与正式版相同
	"gemini-2.5-flash-native-audio-preview-12-2025":      4, // 与正式版相同
}

var defaultAudioCompletionRatio = map[string]float64{
	"gpt-4o-realtime":                                    2,
	"gpt-4o-mini-realtime":                               2,
	"gemini-live-2.5-flash-native-audio":                 4, // $0.80/百万tokens / $0.20基准价格 = 4倍
	"gemini-live-2.5-flash-preview-native-audio-09-2025": 4, // 与正式版相同
	"gemini-2.5-flash-native-audio-preview-12-2025":      4, // 与正式版相同
}

// MultiModalPricing 多模态模型定价结构
type MultiModalPricing struct {
	TextInputPrice            float64 `json:"text_input_price"`
	AudioInputPrice           float64 `json:"audio_input_price"`
	ImageVideoInputPrice      float64 `json:"image_video_input_price"`
	TextOutputPriceTextOnly   float64 `json:"text_output_price_text_only"`
	TextOutputPriceMultimodal float64 `json:"text_output_price_multimodal"`
	TextAudioOutputPrice      float64 `json:"text_audio_output_price"`
}

var defaultImageModelPricePerImage = map[string]float64{
	"qwen-image-plus":                 0.0246575,
	"doubao-seedream-4-0-250828":      0.0247,
	"qwen-image-edit-plus":            0.0246575,
	"qwen-image-edit-plus-2025-10-30": 0.0246575,
}

var defaultMultiModalPricing = map[string]MultiModalPricing{
	"qwen3-omni-flash": {
		TextInputPrice:            0.27,
		AudioInputPrice:           2.33,
		ImageVideoInputPrice:      0.48,
		TextOutputPriceTextOnly:   1.02,
		TextOutputPriceMultimodal: 1.87,
		TextAudioOutputPrice:      9.25,
	},
}

var (
	modelPriceMap      map[string]float64 = nil
	modelPriceMapMutex                    = sync.RWMutex{}
)

var (
	videoModelPricePerSecondMap      map[string]float64 = nil
	videoModelPricePerSecondMapMutex                    = sync.RWMutex{}
)

var (
	videoModelAudioPricePerSecondMap      map[string]VideoAudioPricing = nil
	videoModelAudioPricePerSecondMapMutex                              = sync.RWMutex{}
)

var (
	videoModelPricePerSecondRawMap      map[string]interface{} = nil
	videoModelPricePerSecondRawMapMutex                        = sync.RWMutex{}
)
var (
	modelRatioMap      map[string]float64 = nil
	modelRatioMapMutex                    = sync.RWMutex{}
)

var (
	CompletionRatio      map[string]float64 = nil
	CompletionRatioMutex                    = sync.RWMutex{}
)

var defaultCompletionRatio = map[string]float64{
	"gpt-4-gizmo-*":    2,
	"gpt-4o-gizmo-*":   3,
	"gpt-4-all":        2,
	"gpt-image-1":      8,
	"gpt-image-1-mini": 4,
}

// InitRatioSettings initializes all model related settings maps from database
func InitRatioSettings() {
	// Load modelPriceMap from database
	loadModelPriceFromDatabase()

	// Load modelRatioMap from database
	loadModelRatioFromDatabase()

	// Load CompletionRatio from database
	loadCompletionRatioFromDatabase()

	// Load cacheRatioMap from database
	loadCacheRatioFromDatabase()

	// Load imageRatioMap from database
	loadImageRatioFromDatabase()

	// Load videoModelPricePerSecondMap from database
	loadVideoModelPricePerSecondFromDatabase()

	// Load imageTokenPricingMap from database
	loadImageTokenPricingFromDatabase()

	// Load originImageTokenPricingMap from database
	loadOriginImageTokenPricingFromDatabase()

	// Load imageModelPricePerImageMap from database
	loadImageModelPricePerImageFromDatabase()

	// Load originImageModelPricePerImageMap from database
	loadOriginImageModelPricePerImageFromDatabase()

	// Load multiModalPricingMap from database
	loadMultiModalPricingFromDatabase()

	// Load audioRatioMap from database
	loadAudioRatioFromDatabase()

	// Load audioCompletionRatioMap from database
	loadAudioCompletionRatioFromDatabase()

	// Load imageCompletionRatioMap from database
	loadImageCompletionRatioFromDatabase()

	// Load originImageCompletionRatioMap from database
	loadOriginImageCompletionRatioFromDatabase()

	// Print loaded configuration
	printLoadedConfiguration()
}

func GetModelPriceMap() map[string]float64 {
	modelPriceMapMutex.RLock()
	defer modelPriceMapMutex.RUnlock()
	return modelPriceMap
}

func ModelPrice2JSONString() string {
	modelPriceMapMutex.RLock()
	defer modelPriceMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(modelPriceMap)
	if err != nil {
		common.SysError("error marshalling model price: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateModelPriceByJSONString(jsonStr string) error {
	modelPriceMapMutex.Lock()
	defer modelPriceMapMutex.Unlock()
	modelPriceMap = make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &modelPriceMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// GetModelPrice 返回模型的价格，如果模型不存在则返回-1，false
func GetModelPrice(name string, printErr bool) (float64, bool) {
	modelPriceMapMutex.RLock()
	defer modelPriceMapMutex.RUnlock()

	// 先尝试通配符格式（用于定价配置）
	pricingName := FormatMatchingModelNameForPricing(name)
	price, ok := modelPriceMap[pricingName]
	if ok {
		return price, true
	}

	// 如果通配符格式找不到，尝试基础模型名称
	baseName := FormatMatchingModelName(name)
	if baseName != pricingName {
		price, ok = modelPriceMap[baseName]
		if ok {
			return price, true
		}
	}

	if printErr {
		common.SysError("model price not found: " + name)
	}
	return -1, false
}

func UpdateModelRatioByJSONString(jsonStr string) error {
	modelRatioMapMutex.Lock()
	defer modelRatioMapMutex.Unlock()
	modelRatioMap = make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &modelRatioMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// 处理带有思考预算的模型名称，方便统一定价
func handleThinkingBudgetModel(name, prefix, wildcard string) string {
	if strings.HasPrefix(name, prefix) && strings.Contains(name, "-thinking-") {
		return wildcard
	}
	return name
}

func GetModelRatio(name string) (float64, bool, string) {
	modelRatioMapMutex.RLock()
	defer modelRatioMapMutex.RUnlock()

	// 先尝试通配符格式（用于定价配置）
	pricingName := FormatMatchingModelNameForPricing(name)
	ratio, ok := modelRatioMap[pricingName]
	if ok {
		return ratio, true, pricingName
	}

	// 如果通配符格式找不到，尝试基础模型名称
	baseName := FormatMatchingModelName(name)
	if baseName != pricingName {
		ratio, ok = modelRatioMap[baseName]
		if ok {
			return ratio, true, baseName
		}
	}

	return 37.5, operation_setting.SelfUseModeEnabled, baseName
}

func DefaultModelRatio2JSONString() string {
	jsonBytes, err := common.Marshal(defaultModelRatio)
	if err != nil {
		common.SysError("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func GetDefaultModelRatioMap() map[string]float64 {
	return defaultModelRatio
}

func GetDefaultModelPriceMap() map[string]float64 {
	return defaultModelPrice
}

func GetDefaultImageRatioMap() map[string]float64 {
	return defaultImageRatio
}

func GetDefaultAudioRatioMap() map[string]float64 {
	return defaultAudioRatio
}

func GetDefaultAudioCompletionRatioMap() map[string]float64 {
	return defaultAudioCompletionRatio
}

func GetDefaultVideoModelPricePerSecondMap() map[string]float64 {
	return defaultVideoModelPricePerSecond
}

func GetCompletionRatioMap() map[string]float64 {
	CompletionRatioMutex.RLock()
	defer CompletionRatioMutex.RUnlock()
	return CompletionRatio
}

func CompletionRatio2JSONString() string {
	CompletionRatioMutex.RLock()
	defer CompletionRatioMutex.RUnlock()

	jsonBytes, err := json.Marshal(CompletionRatio)
	if err != nil {
		common.SysError("error marshalling completion ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateCompletionRatioByJSONString(jsonStr string) error {
	CompletionRatioMutex.Lock()
	defer CompletionRatioMutex.Unlock()
	CompletionRatio = make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &CompletionRatio)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

func GetCompletionRatio(name string) float64 {
	CompletionRatioMutex.RLock()
	defer CompletionRatioMutex.RUnlock()

	name = FormatMatchingModelName(name)

	if strings.Contains(name, "/") {
		if ratio, ok := CompletionRatio[name]; ok {
			return ratio
		}
	}
	hardCodedRatio, contain := getHardcodedCompletionModelRatio(name)
	if contain {
		return hardCodedRatio
	}
	if ratio, ok := CompletionRatio[name]; ok {
		return ratio
	}
	return hardCodedRatio
}

func getHardcodedCompletionModelRatio(name string) (float64, bool) {

	isReservedModel := strings.HasSuffix(name, "-all") || strings.HasSuffix(name, "-gizmo-*")
	if isReservedModel {
		return 2, false
	}

	if strings.HasPrefix(name, "gpt-") {
		if strings.HasPrefix(name, "gpt-4o") {
			if name == "gpt-4o-2024-05-13" {
				return 3, false
			}
			return 4, false
		}
		// gpt-5 匹配
		if strings.HasPrefix(name, "gpt-5") {
			return 8, false
		}
		// gpt-4.5-preview匹配
		if strings.HasPrefix(name, "gpt-4.5-preview") {
			return 2, false
		}
		if strings.HasPrefix(name, "gpt-4-turbo") || strings.HasSuffix(name, "gpt-4-1106") || strings.HasSuffix(name, "gpt-4-1105") {
			return 3, false
		}
		// 没有特殊标记的 gpt-4 模型默认倍率为 2
		return 2, false
	}
	if strings.HasPrefix(name, "o1") || strings.HasPrefix(name, "o3") {
		return 4, false
	}
	if name == "chatgpt-4o-latest" {
		return 3, false
	}

	if strings.Contains(name, "claude-3") {
		return 5, false
	} else if strings.Contains(name, "claude-sonnet-4") || strings.Contains(name, "claude-opus-4") {
		return 5, false
	} else if strings.Contains(name, "claude-instant-1") || strings.Contains(name, "claude-2") {
		return 3, false
	}

	if strings.HasPrefix(name, "gpt-3.5") {
		if name == "gpt-3.5-turbo" || strings.HasSuffix(name, "0125") {
			// https://openai.com/blog/new-embedding-models-and-api-updates
			// Updated GPT-3.5 Turbo model and lower pricing
			return 3, false
		}
		if strings.HasSuffix(name, "1106") {
			return 2, false
		}
		return 4.0 / 3.0, false
	}
	if strings.HasPrefix(name, "mistral-") {
		return 3, false
	}
	if strings.HasPrefix(name, "gemini-") {
		if strings.HasPrefix(name, "gemini-1.5") {
			return 4, false
		} else if strings.HasPrefix(name, "gemini-2.0") {
			return 4, false
		} else if strings.HasPrefix(name, "gemini-2.5-pro") { // 移除preview来增加兼容性，这里假设正式版的倍率和preview一致
			return 8, false
		} else if strings.HasPrefix(name, "gemini-2.5-flash") { // 处理不同的flash模型倍率
			if strings.HasPrefix(name, "gemini-2.5-flash-preview") {
				if strings.HasSuffix(name, "-nothinking") {
					return 4, false
				}
				return 3.5 / 0.15, false
			}
			if strings.HasPrefix(name, "gemini-2.5-flash-lite") {
				return 4, false
			}
			return 2.5 / 0.3, false
		} else if strings.HasPrefix(name, "gemini-robotics-er-1.5") {
			return 2.5 / 0.3, false
		}
		return 4, false
	}
	if strings.HasPrefix(name, "command") {
		switch name {
		case "command-r":
			return 3, false
		case "command-r-plus":
			return 5, false
		case "command-r-08-2024":
			return 4, false
		case "command-r-plus-08-2024":
			return 4, false
		default:
			return 4, false
		}
	}
	// hint 只给官方上4倍率，由于开源模型供应商自行定价，不对其进行补全倍率进行强制对齐
	if strings.HasPrefix(name, "ERNIE-Speed-") {
		return 2, false
	} else if strings.HasPrefix(name, "ERNIE-Lite-") {
		return 2, false
	} else if strings.HasPrefix(name, "ERNIE-Character") {
		return 2, false
	} else if strings.HasPrefix(name, "ERNIE-Functions") {
		return 2, false
	}
	switch name {
	case "llama2-70b-4096":
		return 0.8 / 0.64, false
	case "llama3-8b-8192":
		return 2, false
	case "llama3-70b-8192":
		return 0.79 / 0.59, false
	}
	return 1, false
}

func GetAudioRatio(name string) float64 {
	audioRatioMapMutex.RLock()
	defer audioRatioMapMutex.RUnlock()
	name = FormatMatchingModelName(name)
	if ratio, ok := audioRatioMap[name]; ok {
		return ratio
	}
	return 20
}

func GetAudioCompletionRatio(name string) float64 {
	audioCompletionRatioMapMutex.RLock()
	defer audioCompletionRatioMapMutex.RUnlock()
	name = FormatMatchingModelName(name)
	if ratio, ok := audioCompletionRatioMap[name]; ok {

		return ratio
	}
	return 2
}

func ModelRatio2JSONString() string {
	modelRatioMapMutex.RLock()
	defer modelRatioMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(modelRatioMap)
	if err != nil {
		common.SysError("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

var defaultImageRatio = map[string]float64{
	"gpt-image-1":      2,
	"gpt-image-1-mini": 1,
}
var imageRatioMap map[string]float64
var imageRatioMapMutex sync.RWMutex

// ImageTokenPricing 相关变量
var (
	imageTokenPricingMap            map[string]ImageTokenPricing = nil
	imageTokenPricingMapMutex                                    = sync.RWMutex{}
	originImageTokenPricingMap      map[string]ImageTokenPricing = nil
	originImageTokenPricingMapMutex                              = sync.RWMutex{}
)

var (
	audioRatioMap      map[string]float64 = nil
	audioRatioMapMutex                    = sync.RWMutex{}
)
var (
	audioCompletionRatioMap      map[string]float64 = nil
	audioCompletionRatioMapMutex                    = sync.RWMutex{}
)

var (
	imageCompletionRatioMap      map[string]float64 = nil
	imageCompletionRatioMapMutex                    = sync.RWMutex{}
)

var (
	originImageCompletionRatioMap      map[string]float64 = nil
	originImageCompletionRatioMapMutex                    = sync.RWMutex{}
)

// 按张计费的图片模型价格（系统价格，用于实际扣费，单位：美元/张）
var (
	imageModelPricePerImageMap      map[string]float64 = nil
	imageModelPricePerImageMapMutex                    = sync.RWMutex{}
)

// 按张计费的图片模型原始价格（厂商价格，仅用于前端展示对比，单位：美元/张）
var (
	originImageModelPricePerImageMap      map[string]float64 = nil
	originImageModelPricePerImageMapMutex                    = sync.RWMutex{}
)

// 多模态模型定价
var (
	multiModalPricingMap      map[string]MultiModalPricing = nil
	multiModalPricingMapMutex                              = sync.RWMutex{}
)

func ImageRatio2JSONString() string {
	imageRatioMapMutex.RLock()
	defer imageRatioMapMutex.RUnlock()
	jsonBytes, err := common.Marshal(imageRatioMap)
	if err != nil {
		common.SysError("error marshalling cache ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateImageRatioByJSONString(jsonStr string) error {
	imageRatioMapMutex.Lock()
	defer imageRatioMapMutex.Unlock()
	imageRatioMap = make(map[string]float64)
	return common.Unmarshal([]byte(jsonStr), &imageRatioMap)
}

func GetImageRatio(name string) (float64, bool) {
	imageRatioMapMutex.RLock()
	defer imageRatioMapMutex.RUnlock()
	ratio, ok := imageRatioMap[name]
	if !ok {
		return 1, false // Default to 1 if not found
	}
	return ratio, true
}

func AudioRatio2JSONString() string {
	audioRatioMapMutex.RLock()
	defer audioRatioMapMutex.RUnlock()
	jsonBytes, err := common.Marshal(audioRatioMap)
	if err != nil {
		common.SysError("error marshalling audio ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateAudioRatioByJSONString(jsonStr string) error {

	tmp := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &tmp); err != nil {
		return err
	}
	audioRatioMapMutex.Lock()
	audioRatioMap = tmp
	audioRatioMapMutex.Unlock()
	InvalidateExposedDataCache()
	return nil
}

func GetAudioRatioCopy() map[string]float64 {
	audioRatioMapMutex.RLock()
	defer audioRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(audioRatioMap))
	for k, v := range audioRatioMap {
		copyMap[k] = v
	}
	return copyMap
}

func AudioCompletionRatio2JSONString() string {
	audioCompletionRatioMapMutex.RLock()
	defer audioCompletionRatioMapMutex.RUnlock()
	jsonBytes, err := common.Marshal(audioCompletionRatioMap)
	if err != nil {
		common.SysError("error marshalling audio completion ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateAudioCompletionRatioByJSONString(jsonStr string) error {
	tmp := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &tmp); err != nil {
		return err
	}
	audioCompletionRatioMapMutex.Lock()
	audioCompletionRatioMap = tmp
	audioCompletionRatioMapMutex.Unlock()
	InvalidateExposedDataCache()
	return nil
}

func GetAudioCompletionRatioCopy() map[string]float64 {
	audioCompletionRatioMapMutex.RLock()
	defer audioCompletionRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(audioCompletionRatioMap))
	for k, v := range audioCompletionRatioMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetImageCompletionRatio(name string) float64 {
	imageCompletionRatioMapMutex.RLock()
	defer imageCompletionRatioMapMutex.RUnlock()
	name = FormatMatchingModelName(name)
	if ratio, ok := imageCompletionRatioMap[name]; ok {
		return ratio
	}
	// 如果没有配置 ImageCompletionRatio，回退到 CompletionRatio
	return GetCompletionRatio(name)
}

func ImageCompletionRatio2JSONString() string {
	imageCompletionRatioMapMutex.RLock()
	defer imageCompletionRatioMapMutex.RUnlock()
	jsonBytes, err := common.Marshal(imageCompletionRatioMap)
	if err != nil {
		common.SysError("error marshalling image completion ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateImageCompletionRatioByJSONString(jsonStr string) error {
	tmp := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &tmp); err != nil {
		return err
	}
	imageCompletionRatioMapMutex.Lock()
	imageCompletionRatioMap = tmp
	imageCompletionRatioMapMutex.Unlock()
	InvalidateExposedDataCache()
	return nil
}

func GetImageCompletionRatioCopy() map[string]float64 {
	imageCompletionRatioMapMutex.RLock()
	defer imageCompletionRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(imageCompletionRatioMap))
	for k, v := range imageCompletionRatioMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetOriginImageCompletionRatio(name string) float64 {
	originImageCompletionRatioMapMutex.RLock()
	defer originImageCompletionRatioMapMutex.RUnlock()
	name = FormatMatchingModelName(name)
	if ratio, ok := originImageCompletionRatioMap[name]; ok {
		return ratio
	}
	return 0
}

func OriginImageCompletionRatio2JSONString() string {
	originImageCompletionRatioMapMutex.RLock()
	defer originImageCompletionRatioMapMutex.RUnlock()
	jsonBytes, err := common.Marshal(originImageCompletionRatioMap)
	if err != nil {
		common.SysError("error marshalling origin image completion ratio: " + err.Error())
	}
	return string(jsonBytes)
}

func UpdateOriginImageCompletionRatioByJSONString(jsonStr string) error {
	tmp := make(map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &tmp); err != nil {
		return err
	}
	originImageCompletionRatioMapMutex.Lock()
	originImageCompletionRatioMap = tmp
	originImageCompletionRatioMapMutex.Unlock()
	InvalidateExposedDataCache()
	return nil
}

func GetOriginImageCompletionRatioCopy() map[string]float64 {
	originImageCompletionRatioMapMutex.RLock()
	defer originImageCompletionRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(originImageCompletionRatioMap))
	for k, v := range originImageCompletionRatioMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetModelRatioCopy() map[string]float64 {
	modelRatioMapMutex.RLock()
	defer modelRatioMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(modelRatioMap))
	for k, v := range modelRatioMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetModelPriceCopy() map[string]float64 {
	modelPriceMapMutex.RLock()
	defer modelPriceMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(modelPriceMap))
	for k, v := range modelPriceMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetCompletionRatioCopy() map[string]float64 {
	CompletionRatioMutex.RLock()
	defer CompletionRatioMutex.RUnlock()
	copyMap := make(map[string]float64, len(CompletionRatio))
	for k, v := range CompletionRatio {
		copyMap[k] = v
	}
	return copyMap
}

// VideoModelPricePerSecond related functions
func GetVideoModelPricePerSecond(name string) (float64, bool) {
	name = FormatMatchingModelName(name)
	price, ok := getVideoPerSecondPriceFromPrimaryMap(name)
	if ok && price > 0 {
		return price, true
	}

	if audioPricing, ok := getVideoAudioPricing(name); ok {
		if audioPricing.NoAudio > 0 {
			return audioPricing.NoAudio, true
		}
		if audioPricing.Audio > 0 {
			return audioPricing.Audio, true
		}
	}

	// 对于 wan2.5 系列模型（i2v 和 t2v），如果缓存中没有，尝试从原始配置读取 default 值
	if name == "wan2.5-i2v-preview" || name == "wan2.5-t2v-preview" {
		if videoStr, exists := common.OptionMap["VideoModelPricePerSecond"]; exists {
			var rawMap map[string]interface{}
			if err := common.Unmarshal([]byte(videoStr), &rawMap); err == nil {
				// 优先查找当前模型，如果没有则查找 wan2.5-i2v-preview（因为配置可能只有 i2v）
				var wan25Config map[string]interface{}
				if config, ok := rawMap[name].(map[string]interface{}); ok {
					wan25Config = config
				} else if config, ok := rawMap["wan2.5-i2v-preview"].(map[string]interface{}); ok {
					wan25Config = config
				}

				if wan25Config != nil {
					// 尝试读取 default 值
					if def, ok := extractFloatFromMap(wan25Config, "default"); ok && def > 0 {
						return def, true
					}
				}
			}
		}
	}

	return -1, false
}

func GetVideoModelPricePerSecondWithAudio(name string, generateAudio bool) (float64, bool) {
	name = FormatMatchingModelName(name)

	if audioPricing, ok := getVideoAudioPricing(name); ok {
		if generateAudio {
			if audioPricing.Audio > 0 {
				return audioPricing.Audio, true
			}
			if audioPricing.NoAudio > 0 {
				return audioPricing.NoAudio, true
			}
		} else {
			if audioPricing.NoAudio > 0 {
				return audioPricing.NoAudio, true
			}
			if audioPricing.Audio > 0 {
				return audioPricing.Audio, true
			}
		}
	}

	// 回退到通用价格
	price, ok := getVideoPerSecondPriceFromPrimaryMap(name)
	if ok && price > 0 {
		return price, true
	}

	return -1, false
}

func UpdateVideoModelPricePerSecondByJSONString(jsonStr string) error {
	var rawMap map[string]interface{}
	if err := common.Unmarshal([]byte(jsonStr), &rawMap); err != nil {
		return err
	}

	videoModelPricePerSecondMapMutex.Lock()
	defer videoModelPricePerSecondMapMutex.Unlock()
	videoModelAudioPricePerSecondMapMutex.Lock()
	defer videoModelAudioPricePerSecondMapMutex.Unlock()
	videoModelPricePerSecondRawMapMutex.Lock()
	defer videoModelPricePerSecondRawMapMutex.Unlock()

	videoModelPricePerSecondRawMap = rawMap
	videoModelPricePerSecondMap, videoModelAudioPricePerSecondMap = buildVideoModelPriceCaches(rawMap)

	InvalidateExposedDataCache()
	return nil
}

func VideoModelPricePerSecond2JSONString() string {
	videoModelPricePerSecondRawMapMutex.RLock()
	defer videoModelPricePerSecondRawMapMutex.RUnlock()

	if len(videoModelPricePerSecondRawMap) > 0 {
		if jsonBytes, err := common.Marshal(videoModelPricePerSecondRawMap); err == nil {
			return string(jsonBytes)
		}
	}

	// 回退：使用主价格映射
	videoModelPricePerSecondMapMutex.RLock()
	defer videoModelPricePerSecondMapMutex.RUnlock()
	jsonBytes, err := common.Marshal(videoModelPricePerSecondMap)
	if err != nil {
		common.SysError("error marshalling video model price per second: " + err.Error())
	}
	return string(jsonBytes)
}

func GetVideoModelPricePerSecondCopy() map[string]float64 {
	videoModelPricePerSecondMapMutex.RLock()
	defer videoModelPricePerSecondMapMutex.RUnlock()
	copyMap := make(map[string]float64, len(videoModelPricePerSecondMap))
	for k, v := range videoModelPricePerSecondMap {
		copyMap[k] = v
	}
	return copyMap
}

func GetVideoModelAudioPricePerSecondCopy() map[string]VideoAudioPricing {
	videoModelAudioPricePerSecondMapMutex.RLock()
	defer videoModelAudioPricePerSecondMapMutex.RUnlock()
	copyMap := make(map[string]VideoAudioPricing, len(videoModelAudioPricePerSecondMap))
	for k, v := range videoModelAudioPricePerSecondMap {
		copyMap[k] = v
	}
	return copyMap
}

func getVideoPerSecondPriceFromPrimaryMap(name string) (float64, bool) {
	videoModelPricePerSecondMapMutex.RLock()
	defer videoModelPricePerSecondMapMutex.RUnlock()
	price, ok := videoModelPricePerSecondMap[name]
	return price, ok
}

func getVideoAudioPricing(name string) (VideoAudioPricing, bool) {
	videoModelAudioPricePerSecondMapMutex.RLock()
	defer videoModelAudioPricePerSecondMapMutex.RUnlock()
	pricing, ok := videoModelAudioPricePerSecondMap[name]
	return pricing, ok
}

func buildVideoModelPriceCaches(rawMap map[string]interface{}) (map[string]float64, map[string]VideoAudioPricing) {
	priceMap := make(map[string]float64, len(rawMap))
	audioMap := make(map[string]VideoAudioPricing)

	for model, value := range rawMap {
		targetKeys := []string{model}
		formatted := FormatMatchingModelName(model)
		if formatted != model {
			targetKeys = append(targetKeys, formatted)
		}

		switch v := value.(type) {
		case map[string]interface{}:
			pricing := VideoAudioPricing{}
			if noAudio, ok := extractFloatFromMap(v, "noAudio", "no_audio"); ok {
				pricing.NoAudio = noAudio
			}
			if audio, ok := extractFloatFromMap(v, "audio", "withAudio", "with_audio"); ok {
				pricing.Audio = audio
			}
			if pricing.NoAudio > 0 || pricing.Audio > 0 {
				for _, key := range targetKeys {
					audioMap[key] = pricing
				}
			}
			if def, ok := extractFloatFromMap(v, "default"); ok {
				for _, key := range targetKeys {
					priceMap[key] = def
				}
			} else if pricing.NoAudio > 0 {
				for _, key := range targetKeys {
					priceMap[key] = pricing.NoAudio
				}
			} else if pricing.Audio > 0 {
				for _, key := range targetKeys {
					priceMap[key] = pricing.Audio
				}
			}
		default:
			if f, ok := extractFloat(v); ok {
				for _, key := range targetKeys {
					priceMap[key] = f
				}
			}
		}
	}

	return priceMap, audioMap
}

func extractFloatFromMap(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		if val, exists := m[key]; exists {
			if f, ok := extractFloat(val); ok {
				return f, true
			}
		}
	}
	return 0, false
}

func extractFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// GetVideoModelPriceByResolution 根据分辨率获取视频模型价格
// 特殊处理 wan2.5 系列模型（i2v 和 t2v）的分辨率定价
func GetVideoModelPriceByResolution(modelName, resolution string) (float64, bool) {
	// wan2.5 系列模型（i2v 和 t2v）使用相同的分辨率定价
	if modelName != "wan2.5-i2v-preview" && modelName != "wan2.5-t2v-preview" {
		// 其他模型使用统一价格
		return GetVideoModelPricePerSecond(modelName)
	}

	// 从数据库配置读取 wan2.5 系列模型的分辨率价格
	// 注意：配置中可能使用 "wan2.5-i2v-preview" 作为 key，但 t2v 模型也使用相同的价格
	if videoStr, exists := common.OptionMap["VideoModelPricePerSecond"]; exists {
		var rawMap map[string]interface{}
		if err := common.Unmarshal([]byte(videoStr), &rawMap); err == nil {
			// 优先查找当前模型，如果没有则查找 wan2.5-i2v-preview（因为配置可能只有 i2v）
			var wan25Config map[string]interface{}
			if config, ok := rawMap[modelName].(map[string]interface{}); ok {
				wan25Config = config
			} else if config, ok := rawMap["wan2.5-i2v-preview"].(map[string]interface{}); ok {
				wan25Config = config
			}

			if wan25Config != nil {
				if resolutions, ok := wan25Config["resolutions"].(map[string]interface{}); ok {
					if price, ok := resolutions[strings.ToLower(resolution)].(float64); ok {
						return price, true
					}
				}
				// 使用default价格作为后备
				if def, ok := wan25Config["default"].(float64); ok {
					return def, true
				}
			}
		}
	}

	// 最终后备：使用默认720p价格
	return 0.0738, true
}

// 转换模型名，减少渠道必须配置各种带参数模型
// 此函数用于数据库查询，始终返回基础模型名称（去除所有后缀）
// 注意：不再自动处理 -thinking- 后缀，请通过渠道管理的模型重定向和参数覆盖来配置
func FormatMatchingModelName(name string) string {
	// 只处理 gpt-4-gizmo 和 gpt-4o-gizmo 的通配符匹配
	if strings.HasPrefix(name, "gpt-4-gizmo") {
		name = "gpt-4-gizmo-*"
	}
	if strings.HasPrefix(name, "gpt-4o-gizmo") {
		name = "gpt-4o-gizmo-*"
	}
	return name
}

// FormatMatchingModelNameForPricing 用于定价匹配，可能返回通配符格式
// 例如：gemini-2.5-flash-thinking-128 -> gemini-2.5-flash-thinking-*
// doubao-seed-1-6-thinking-250715 -> doubao-seed-1-6 (匹配基础模型)
// 注意：不再自动处理 -thinking- 后缀，请通过渠道管理的模型重定向和参数覆盖来配置
func FormatMatchingModelNameForPricing(name string) string {
	// 对于 -thinking-<数字> 格式，返回通配符以便统一配置定价
	if strings.Contains(name, "-thinking-") {
		if strings.HasPrefix(name, "gemini-2.5-flash-lite") {
			return "gemini-2.5-flash-lite-thinking-*"
		} else if strings.HasPrefix(name, "gemini-2.5-flash") {
			return "gemini-2.5-flash-thinking-*"
		} else if strings.HasPrefix(name, "gemini-2.5-pro") {
			return "gemini-2.5-pro-thinking-*"
		}
		// 对于其他模型（如 doubao-seed-1-6-thinking-250715），
		// 直接返回基础模型名称（doubao-seed-1-6），以便匹配基础模型的配置
		// 这样用户只需要配置 doubao-seed-1-6 的倍率，所有 thinking 变体都会使用这个倍率
		return FormatMatchingModelName(name)
	}
	// 其他情况使用基础格式化
	// 直接使用基础格式化，不再特殊处理 thinking 后缀
	return FormatMatchingModelName(name)
}

// loadModelPriceFromDatabase loads model price configuration from database
func loadModelPriceFromDatabase() {
	modelPriceMapMutex.Lock()
	defer modelPriceMapMutex.Unlock()

	// Try to get from database first
	if priceStr, exists := common.OptionMap["ModelPrice"]; exists && priceStr != "" {
		var priceMap map[string]float64
		if err := common.Unmarshal([]byte(priceStr), &priceMap); err == nil {
			modelPriceMap = priceMap
			common.SysLog("Loaded model price configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	modelPriceMap = defaultModelPrice
	common.SysLog("Using default model price configuration")
}

// loadModelRatioFromDatabase loads model ratio configuration from database
func loadModelRatioFromDatabase() {
	modelRatioMapMutex.Lock()
	defer modelRatioMapMutex.Unlock()

	// Try to get from database first
	if ratioStr, exists := common.OptionMap["ModelRatio"]; exists && ratioStr != "" {
		var ratioMap map[string]float64
		if err := common.Unmarshal([]byte(ratioStr), &ratioMap); err == nil {
			modelRatioMap = ratioMap
			common.SysLog("Loaded model ratio configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	modelRatioMap = defaultModelRatio
	common.SysLog("Using default model ratio configuration")
}

// loadCompletionRatioFromDatabase loads completion ratio configuration from database
func loadCompletionRatioFromDatabase() {
	CompletionRatioMutex.Lock()
	defer CompletionRatioMutex.Unlock()

	// Try to get from database first
	if completionStr, exists := common.OptionMap["CompletionRatio"]; exists && completionStr != "" {
		var completionMap map[string]float64
		if err := common.Unmarshal([]byte(completionStr), &completionMap); err == nil {
			CompletionRatio = completionMap
			common.SysLog("Loaded completion ratio configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	CompletionRatio = defaultCompletionRatio
	common.SysLog("Using default completion ratio configuration")
}

// loadCacheRatioFromDatabase loads cache ratio configuration from database
func loadCacheRatioFromDatabase() {
	cacheRatioMapMutex.Lock()
	defer cacheRatioMapMutex.Unlock()

	// Try to get from database first
	if cacheStr, exists := common.OptionMap["CacheRatio"]; exists && cacheStr != "" {
		var cacheMap map[string]float64
		if err := common.Unmarshal([]byte(cacheStr), &cacheMap); err == nil {
			cacheRatioMap = cacheMap
			common.SysLog("Loaded cache ratio configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	cacheRatioMap = defaultCacheRatio
	common.SysLog("Using default cache ratio configuration")
}

// loadAudioRatioFromDatabase loads audio ratio configuration from database
func loadAudioRatioFromDatabase() {
	audioRatioMapMutex.Lock()
	defer audioRatioMapMutex.Unlock()

	// Try to get from database first
	if audioStr, exists := common.OptionMap["AudioRatio"]; exists && audioStr != "" {
		var audioMap map[string]float64
		if err := common.Unmarshal([]byte(audioStr), &audioMap); err == nil {
			audioRatioMap = audioMap
			common.SysLog("Loaded audio ratio configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	audioRatioMap = defaultAudioRatio
	common.SysLog("Using default audio ratio configuration")
}

// loadAudioCompletionRatioFromDatabase loads audio completion ratio configuration from database
func loadAudioCompletionRatioFromDatabase() {
	audioCompletionRatioMapMutex.Lock()
	defer audioCompletionRatioMapMutex.Unlock()

	// Try to get from database first
	if audioCompletionStr, exists := common.OptionMap["AudioCompletionRatio"]; exists && audioCompletionStr != "" {
		var audioCompletionMap map[string]float64
		if err := common.Unmarshal([]byte(audioCompletionStr), &audioCompletionMap); err == nil {
			audioCompletionRatioMap = audioCompletionMap
			common.SysLog("Loaded audio completion ratio configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	audioCompletionRatioMap = defaultAudioCompletionRatio
	common.SysLog("Using default audio completion ratio configuration")
}

// loadImageCompletionRatioFromDatabase loads image completion ratio configuration from database
func loadImageCompletionRatioFromDatabase() {
	imageCompletionRatioMapMutex.Lock()
	defer imageCompletionRatioMapMutex.Unlock()

	// Try to get from database first
	if imageCompletionStr, exists := common.OptionMap["ImageCompletionRatio"]; exists && imageCompletionStr != "" {
		var imageCompletionMap map[string]float64
		if err := common.Unmarshal([]byte(imageCompletionStr), &imageCompletionMap); err == nil {
			imageCompletionRatioMap = imageCompletionMap
			common.SysLog("Loaded image completion ratio configuration from database")
			return
		}
	}

	// Fallback to empty map if database load fails
	imageCompletionRatioMap = make(map[string]float64)
	common.SysLog("Using empty image completion ratio configuration")
}

// loadOriginImageCompletionRatioFromDatabase loads origin image completion ratio configuration from database
func loadOriginImageCompletionRatioFromDatabase() {
	originImageCompletionRatioMapMutex.Lock()
	defer originImageCompletionRatioMapMutex.Unlock()

	// Try to get from database first
	if originImageCompletionStr, exists := common.OptionMap["OriginImageCompletionRatio"]; exists && originImageCompletionStr != "" {
		var originImageCompletionMap map[string]float64
		if err := common.Unmarshal([]byte(originImageCompletionStr), &originImageCompletionMap); err == nil {
			originImageCompletionRatioMap = originImageCompletionMap
			common.SysLog("Loaded origin image completion ratio configuration from database")
			return
		}
	}

	// Fallback to empty map if database load fails
	originImageCompletionRatioMap = make(map[string]float64)
	common.SysLog("Using empty origin image completion ratio configuration")
}

// loadImageRatioFromDatabase loads image ratio configuration from database
func loadImageRatioFromDatabase() {
	imageRatioMapMutex.Lock()
	defer imageRatioMapMutex.Unlock()

	// Try to get from database first
	if imageStr, exists := common.OptionMap["ImageRatio"]; exists && imageStr != "" {
		var imageMap map[string]float64
		if err := common.Unmarshal([]byte(imageStr), &imageMap); err == nil {
			imageRatioMap = imageMap
			common.SysLog("Loaded image ratio configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	imageRatioMap = defaultImageRatio
	common.SysLog("Using default image ratio configuration")
}

// loadVideoModelPricePerSecondFromDatabase loads video model price per second configuration from database
func loadVideoModelPricePerSecondFromDatabase() {
	videoModelPricePerSecondMapMutex.Lock()
	defer videoModelPricePerSecondMapMutex.Unlock()
	videoModelAudioPricePerSecondMapMutex.Lock()
	defer videoModelAudioPricePerSecondMapMutex.Unlock()
	videoModelPricePerSecondRawMapMutex.Lock()
	defer videoModelPricePerSecondRawMapMutex.Unlock()

	// Try to get from database first
	if videoStr, exists := common.OptionMap["VideoModelPricePerSecond"]; exists && videoStr != "" {
		// 先解析为 map[string]interface{} 以处理混合类型（float64和对象）
		var rawMap map[string]interface{}
		if err := common.Unmarshal([]byte(videoStr), &rawMap); err == nil {
			videoModelPricePerSecondRawMap = rawMap
			videoModelPricePerSecondMap, videoModelAudioPricePerSecondMap = buildVideoModelPriceCaches(rawMap)
			common.SysLog("Loaded video model price per second configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	videoModelPricePerSecondMap = make(map[string]float64, len(defaultVideoModelPricePerSecond))
	for k, v := range defaultVideoModelPricePerSecond {
		videoModelPricePerSecondMap[k] = v
	}
	videoModelAudioPricePerSecondMap = make(map[string]VideoAudioPricing, len(defaultVideoAudioPricing))
	videoModelPricePerSecondRawMap = make(map[string]interface{})
	for k, v := range defaultVideoAudioPricing {
		videoModelAudioPricePerSecondMap[k] = v
		videoModelPricePerSecondMap[k] = v.NoAudio
		videoModelPricePerSecondRawMap[k] = map[string]float64{
			"noAudio": v.NoAudio,
			"audio":   v.Audio,
		}
	}
	for k, v := range defaultVideoModelPricePerSecond {
		if _, exists := videoModelPricePerSecondRawMap[k]; !exists {
			videoModelPricePerSecondRawMap[k] = v
		}
	}
	// 添加 wan2.5 系列模型的默认配置（带分辨率）
	for k, v := range defaultVideoModelPricePerSecondWithResolutions {
		videoModelPricePerSecondRawMap[k] = v
		// 构建缓存，将 default 值放入 priceMap
		if config, ok := v.(map[string]interface{}); ok {
			if def, ok := extractFloatFromMap(config, "default"); ok {
				videoModelPricePerSecondMap[k] = def
			}
		}
	}
	common.SysLog("Using default video model price per second configuration")
}

// loadImageTokenPricingFromDatabase loads image token pricing configuration from database
func loadImageTokenPricingFromDatabase() {
	imageTokenPricingMapMutex.Lock()
	defer imageTokenPricingMapMutex.Unlock()

	// Try to get from database first
	if pricingStr, exists := common.OptionMap["ImageTokenPricing"]; exists && pricingStr != "" {
		var pricingMap map[string]ImageTokenPricing
		if err := common.Unmarshal([]byte(pricingStr), &pricingMap); err == nil {
			imageTokenPricingMap = pricingMap
			common.SysLog("Loaded image token pricing configuration from database")
			return
		}
	}

	// Fallback to empty map if database load fails
	imageTokenPricingMap = make(map[string]ImageTokenPricing)
	common.SysLog("Using empty image token pricing configuration")
}

// loadOriginImageTokenPricingFromDatabase loads origin image token pricing configuration from database
func loadOriginImageTokenPricingFromDatabase() {
	originImageTokenPricingMapMutex.Lock()
	defer originImageTokenPricingMapMutex.Unlock()

	// Try to get from database first
	if pricingStr, exists := common.OptionMap["OriginImageTokenPricing"]; exists && pricingStr != "" {
		var pricingMap map[string]ImageTokenPricing
		if err := common.Unmarshal([]byte(pricingStr), &pricingMap); err == nil {
			originImageTokenPricingMap = pricingMap
			common.SysLog("Loaded origin image token pricing configuration from database")
			return
		}
	}

	// Fallback to empty map if database load fails
	originImageTokenPricingMap = make(map[string]ImageTokenPricing)
	common.SysLog("Using empty origin image token pricing configuration")
}

// GetImageTokenPricing 返回模型的图像Token表定价配置
func GetImageTokenPricing(name string) (ImageTokenPricing, bool) {
	imageTokenPricingMapMutex.RLock()
	defer imageTokenPricingMapMutex.RUnlock()

	name = FormatMatchingModelName(name)

	pricing, ok := imageTokenPricingMap[name]
	return pricing, ok
}

// GetOriginImageTokenPricing 返回模型的原始图像Token表定价配置（用于展示对比）
func GetOriginImageTokenPricing(name string) (ImageTokenPricing, bool) {
	originImageTokenPricingMapMutex.RLock()
	defer originImageTokenPricingMapMutex.RUnlock()

	name = FormatMatchingModelName(name)

	pricing, ok := originImageTokenPricingMap[name]
	return pricing, ok
}

// UpdateImageTokenPricingByJSONString 通过JSON字符串更新图像Token表定价配置
func UpdateImageTokenPricingByJSONString(jsonStr string) error {
	imageTokenPricingMapMutex.Lock()
	defer imageTokenPricingMapMutex.Unlock()

	imageTokenPricingMap = make(map[string]ImageTokenPricing)
	err := common.Unmarshal([]byte(jsonStr), &imageTokenPricingMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// UpdateOriginImageTokenPricingByJSONString 通过JSON字符串更新原始图像Token表定价配置
func UpdateOriginImageTokenPricingByJSONString(jsonStr string) error {
	originImageTokenPricingMapMutex.Lock()
	defer originImageTokenPricingMapMutex.Unlock()

	originImageTokenPricingMap = make(map[string]ImageTokenPricing)
	err := common.Unmarshal([]byte(jsonStr), &originImageTokenPricingMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// ImageTokenPricing2JSONString 将图像Token表定价配置转换为JSON字符串
func ImageTokenPricing2JSONString() string {
	imageTokenPricingMapMutex.RLock()
	defer imageTokenPricingMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(imageTokenPricingMap)
	if err != nil {
		common.SysError("error marshalling image token pricing: " + err.Error())
	}
	return string(jsonBytes)
}

// OriginImageTokenPricing2JSONString 将原始图像Token表定价配置转换为JSON字符串
func OriginImageTokenPricing2JSONString() string {
	originImageTokenPricingMapMutex.RLock()
	defer originImageTokenPricingMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(originImageTokenPricingMap)
	if err != nil {
		common.SysError("error marshalling origin image token pricing: " + err.Error())
	}
	return string(jsonBytes)
}

// GetImageTokenPricingCopy 获取图像Token表定价配置的副本
func GetImageTokenPricingCopy() map[string]ImageTokenPricing {
	imageTokenPricingMapMutex.RLock()
	defer imageTokenPricingMapMutex.RUnlock()

	copyMap := make(map[string]ImageTokenPricing, len(imageTokenPricingMap))
	for k, v := range imageTokenPricingMap {
		copyMap[k] = v
	}
	return copyMap
}

// GetOriginImageTokenPricingCopy 获取原始图像Token表定价配置的副本
func GetOriginImageTokenPricingCopy() map[string]ImageTokenPricing {
	originImageTokenPricingMapMutex.RLock()
	defer originImageTokenPricingMapMutex.RUnlock()

	copyMap := make(map[string]ImageTokenPricing, len(originImageTokenPricingMap))
	for k, v := range originImageTokenPricingMap {
		copyMap[k] = v
	}
	return copyMap
}

// loadImageModelPricePerImageFromDatabase 从数据库加载按张计费价格（系统价格）
func loadImageModelPricePerImageFromDatabase() {
	imageModelPricePerImageMapMutex.Lock()
	defer imageModelPricePerImageMapMutex.Unlock()

	if priceStr, exists := common.OptionMap["ImageModelPricePerImage"]; exists && priceStr != "" {
		var priceMap map[string]float64
		if err := common.Unmarshal([]byte(priceStr), &priceMap); err == nil {
			imageModelPricePerImageMap = priceMap
			common.SysLog("Loaded image model price per image from database")
			return
		}
	}

	// Fallback to default if database load fails
	imageModelPricePerImageMap = make(map[string]float64, len(defaultImageModelPricePerImage))
	for k, v := range defaultImageModelPricePerImage {
		imageModelPricePerImageMap[k] = v
	}
	common.SysLog("Using default image model price per image configuration")
}

// loadOriginImageModelPricePerImageFromDatabase 从数据库加载原始按张计费价格（厂商价格）
func loadOriginImageModelPricePerImageFromDatabase() {
	originImageModelPricePerImageMapMutex.Lock()
	defer originImageModelPricePerImageMapMutex.Unlock()

	if priceStr, exists := common.OptionMap["OriginImageModelPricePerImage"]; exists && priceStr != "" {
		var priceMap map[string]float64
		if err := common.Unmarshal([]byte(priceStr), &priceMap); err == nil {
			originImageModelPricePerImageMap = priceMap
			common.SysLog("Loaded origin image model price per image from database")
			return
		}
	}

	originImageModelPricePerImageMap = make(map[string]float64)
	common.SysLog("Using empty origin image model price per image")
}

// GetImageModelPricePerImage 获取按张计费价格（系统价格）
func GetImageModelPricePerImage(name string) (float64, bool) {
	imageModelPricePerImageMapMutex.RLock()
	defer imageModelPricePerImageMapMutex.RUnlock()
	price, ok := imageModelPricePerImageMap[name]
	return price, ok
}

// GetOriginImageModelPricePerImage 获取原始按张计费价格（厂商价格）
func GetOriginImageModelPricePerImage(name string) (float64, bool) {
	originImageModelPricePerImageMapMutex.RLock()
	defer originImageModelPricePerImageMapMutex.RUnlock()
	price, ok := originImageModelPricePerImageMap[name]
	return price, ok
}

// UpdateImageModelPricePerImageByJSONString 更新按张计费价格（系统价格）
func UpdateImageModelPricePerImageByJSONString(jsonStr string) error {
	imageModelPricePerImageMapMutex.Lock()
	defer imageModelPricePerImageMapMutex.Unlock()
	imageModelPricePerImageMap = make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &imageModelPricePerImageMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// UpdateOriginImageModelPricePerImageByJSONString 更新原始按张计费价格（厂商价格）
func UpdateOriginImageModelPricePerImageByJSONString(jsonStr string) error {
	originImageModelPricePerImageMapMutex.Lock()
	defer originImageModelPricePerImageMapMutex.Unlock()
	originImageModelPricePerImageMap = make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &originImageModelPricePerImageMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// ImageModelPricePerImage2JSONString 将按张计费价格（系统价格）转换为JSON字符串
func ImageModelPricePerImage2JSONString() string {
	imageModelPricePerImageMapMutex.RLock()
	defer imageModelPricePerImageMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(imageModelPricePerImageMap)
	if err != nil {
		common.SysError("error marshalling image model price per image: " + err.Error())
	}
	return string(jsonBytes)
}

// OriginImageModelPricePerImage2JSONString 将原始按张计费价格（厂商价格）转换为JSON字符串
func OriginImageModelPricePerImage2JSONString() string {
	originImageModelPricePerImageMapMutex.RLock()
	defer originImageModelPricePerImageMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(originImageModelPricePerImageMap)
	if err != nil {
		common.SysError("error marshalling origin image model price per image: " + err.Error())
	}
	return string(jsonBytes)
}

// GetImageModelPricePerImageCopy 获取按张计费价格（系统价格）的副本
func GetImageModelPricePerImageCopy() map[string]float64 {
	imageModelPricePerImageMapMutex.RLock()
	defer imageModelPricePerImageMapMutex.RUnlock()

	copyMap := make(map[string]float64, len(imageModelPricePerImageMap))
	for k, v := range imageModelPricePerImageMap {
		copyMap[k] = v
	}
	return copyMap
}

// GetOriginImageModelPricePerImageCopy 获取原始按张计费价格（厂商价格）的副本
func GetOriginImageModelPricePerImageCopy() map[string]float64 {
	originImageModelPricePerImageMapMutex.RLock()
	defer originImageModelPricePerImageMapMutex.RUnlock()

	copyMap := make(map[string]float64, len(originImageModelPricePerImageMap))
	for k, v := range originImageModelPricePerImageMap {
		copyMap[k] = v
	}
	return copyMap
}

// loadMultiModalPricingFromDatabase 从数据库加载多模态模型定价配置
func loadMultiModalPricingFromDatabase() {
	multiModalPricingMapMutex.Lock()
	defer multiModalPricingMapMutex.Unlock()

	// Try to get from database first
	if pricingStr, exists := common.OptionMap["MultiModalPricing"]; exists && pricingStr != "" {
		var pricingMap map[string]MultiModalPricing
		if err := common.Unmarshal([]byte(pricingStr), &pricingMap); err == nil {
			multiModalPricingMap = pricingMap
			common.SysLog("Loaded multi-modal pricing configuration from database")
			return
		}
	}

	// Fallback to default if database load fails
	multiModalPricingMap = make(map[string]MultiModalPricing, len(defaultMultiModalPricing))
	for k, v := range defaultMultiModalPricing {
		multiModalPricingMap[k] = v
	}
	common.SysLog("Using default multi-modal pricing configuration")
}

// GetMultiModalPricing 获取多模态模型定价配置
func GetMultiModalPricing(name string) (MultiModalPricing, bool) {
	multiModalPricingMapMutex.RLock()
	defer multiModalPricingMapMutex.RUnlock()

	name = FormatMatchingModelName(name)
	pricing, ok := multiModalPricingMap[name]
	return pricing, ok
}

// GetMultiModalPricingCopy 获取多模态模型定价配置的副本
func GetMultiModalPricingCopy() map[string]MultiModalPricing {
	multiModalPricingMapMutex.RLock()
	defer multiModalPricingMapMutex.RUnlock()

	copyMap := make(map[string]MultiModalPricing, len(multiModalPricingMap))
	for k, v := range multiModalPricingMap {
		copyMap[k] = v
	}
	return copyMap
}

// UpdateMultiModalPricingByJSONString 通过JSON字符串更新多模态模型定价配置
func UpdateMultiModalPricingByJSONString(jsonStr string) error {
	multiModalPricingMapMutex.Lock()
	defer multiModalPricingMapMutex.Unlock()

	multiModalPricingMap = make(map[string]MultiModalPricing)
	err := common.Unmarshal([]byte(jsonStr), &multiModalPricingMap)
	if err == nil {
		InvalidateExposedDataCache()
	}
	return err
}

// MultiModalPricing2JSONString 将多模态模型定价配置转换为JSON字符串
func MultiModalPricing2JSONString() string {
	multiModalPricingMapMutex.RLock()
	defer multiModalPricingMapMutex.RUnlock()

	jsonBytes, err := common.Marshal(multiModalPricingMap)
	if err != nil {
		common.SysError("error marshalling multi-modal pricing: " + err.Error())
	}
	return string(jsonBytes)
}

// printLoadedConfiguration prints the loaded configuration summary
func printLoadedConfiguration() {
	modelPriceMapMutex.RLock()
	modelRatioMapMutex.RLock()
	CompletionRatioMutex.RLock()
	cacheRatioMapMutex.RLock()
	imageRatioMapMutex.RLock()
	videoModelPricePerSecondMapMutex.RLock()

	common.SysLog("=== Ratio Settings Configuration Loaded ===")

	// Print Model Price Map JSON
	if priceJSON, err := common.Marshal(modelPriceMap); err == nil {
		common.SysLog("Model Price Map (" + strconv.Itoa(len(modelPriceMap)) + " entries): " + string(priceJSON))
	}

	// Print Model Ratio Map JSON
	if ratioJSON, err := common.Marshal(modelRatioMap); err == nil {
		common.SysLog("Model Ratio Map (" + strconv.Itoa(len(modelRatioMap)) + " entries): " + string(ratioJSON))
	}

	// Print Completion Ratio Map JSON
	if completionJSON, err := common.Marshal(CompletionRatio); err == nil {
		common.SysLog("Completion Ratio Map (" + strconv.Itoa(len(CompletionRatio)) + " entries): " + string(completionJSON))
	}

	// Print Cache Ratio Map JSON
	if cacheJSON, err := common.Marshal(cacheRatioMap); err == nil {
		common.SysLog("Cache Ratio Map (" + strconv.Itoa(len(cacheRatioMap)) + " entries): " + string(cacheJSON))
	}

	// Print Image Ratio Map JSON
	if imageJSON, err := common.Marshal(imageRatioMap); err == nil {
		common.SysLog("Image Ratio Map (" + strconv.Itoa(len(imageRatioMap)) + " entries): " + string(imageJSON))
	}

	// Print Video Model Price Per Second Map JSON
	if videoJSON, err := common.Marshal(videoModelPricePerSecondMap); err == nil {
		common.SysLog("Video Model Price Per Second Map (" + strconv.Itoa(len(videoModelPricePerSecondMap)) + " entries): " + string(videoJSON))
	}

	common.SysLog("=== Configuration Loading Complete ===")

	modelPriceMapMutex.RUnlock()
	modelRatioMapMutex.RUnlock()
	CompletionRatioMutex.RUnlock()
	cacheRatioMapMutex.RUnlock()
	imageRatioMapMutex.RUnlock()
	videoModelPricePerSecondMapMutex.RUnlock()
}
