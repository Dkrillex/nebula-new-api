package dto

// Gemini Live API 消息类型常量
const (
	GeminiLiveMessageTypeSetup                = "setup"
	GeminiLiveMessageTypeClientContent        = "clientContent"
	GeminiLiveMessageTypeRealtimeInput        = "realtimeInput"
	GeminiLiveMessageTypeToolResponse         = "toolResponse"
	GeminiLiveMessageTypeServerContent        = "serverContent"
	GeminiLiveMessageTypeToolCall             = "toolCall"
	GeminiLiveMessageTypeToolCallCancellation = "toolCallCancellation"
	GeminiLiveMessageTypeSetupComplete        = "setupComplete"
)

// Gemini Live API 内容类型
const (
	GeminiLiveContentTypeModel        = "model"
	GeminiLiveContentTypeModelTurn    = "modelTurn"
	GeminiLiveContentTypeInterrupted  = "interrupted"
	GeminiLiveContentTypeTurnComplete = "turnComplete"
)

// GeminiLiveMessage 是 Gemini Live API 的通用消息结构
type GeminiLiveMessage struct {
	// 客户端到服务器的消息（使用 snake_case，因为客户端发送这个格式）
	Setup         *GeminiLiveSetup         `json:"setup,omitempty"`
	ClientContent *GeminiLiveClientContent `json:"client_content,omitempty"`
	RealtimeInput *GeminiLiveRealtimeInput `json:"realtime_input,omitempty"`
	ToolResponse  *GeminiLiveToolResponse  `json:"tool_response,omitempty"`

	// 服务器到客户端的消息（使用 camelCase，因为上游返回这个格式）
	SetupComplete        *GeminiLiveSetupComplete        `json:"setupComplete,omitempty"`
	ServerContent        *GeminiLiveServerContent        `json:"serverContent,omitempty"`
	ToolCall             *GeminiLiveToolCall             `json:"toolCall,omitempty"`
	ToolCallCancellation *GeminiLiveToolCallCancellation `json:"toolCallCancellation,omitempty"`
	UsageMetadata        *GeminiUsageMetadata            `json:"usageMetadata,omitempty"`
}

// GeminiLiveSetup 初始会话配置
type GeminiLiveSetup struct {
	Model                    string                              `json:"model"`
	GenerationConfig         *GeminiLiveGenerationConfig         `json:"generation_config,omitempty"`
	SystemInstruction        *GeminiLiveContent                  `json:"system_instruction,omitempty"`
	Tools                    *GeminiLiveTool                     `json:"tools,omitempty"`
	SpeechConfig             *GeminiLiveSpeechConfig             `json:"speech_config,omitempty"`
	InputAudioTranscription  *GeminiLiveAudioTranscriptionConfig `json:"input_audio_transcription,omitempty"`
	OutputAudioTranscription *GeminiLiveAudioTranscriptionConfig `json:"output_audio_transcription,omitempty"`
	Proactivity              *GeminiLiveProactivityConfig        `json:"proactivity,omitempty"`
	RealtimeInputConfig      *GeminiLiveRealtimeInputConfig      `json:"realtime_input_config,omitempty"`
}

// GeminiLiveGenerationConfig 生成配置
type GeminiLiveGenerationConfig struct {
	Temperature                 *float64                `json:"temperature,omitempty"`
	TopP                        *float64                `json:"top_p,omitempty"`
	TopK                        *int                    `json:"top_k,omitempty"`
	MaxOutputTokens             *int                    `json:"max_output_tokens,omitempty"`
	ResponseModalities          []string                `json:"response_modalities,omitempty"`
	SpeechConfig                *GeminiLiveSpeechConfig `json:"speech_config,omitempty"`
	InputContextWindowThreshold *int                    `json:"input_context_window_threshold,omitempty"` // 上下文大小上限（5000-128000）
	TargetContextSize           *int                    `json:"target_context_size,omitempty"`            // 目标上下文大小（0-128000）
}

// GeminiLiveSpeechConfig 语音配置
type GeminiLiveSpeechConfig struct {
	VoiceConfig  *GeminiLiveVoiceConfig `json:"voice_config,omitempty"`
	LanguageCode string                 `json:"language_code,omitempty"` // 语言代码（如 "zh-CN"）
}

// GeminiLiveVoiceConfig 语音设置
type GeminiLiveVoiceConfig struct {
	PrebuiltVoiceConfig *GeminiLivePrebuiltVoiceConfig `json:"prebuilt_voice_config,omitempty"`
}

// GeminiLivePrebuiltVoiceConfig 预设语音配置
type GeminiLivePrebuiltVoiceConfig struct {
	VoiceName string `json:"voice_name,omitempty"`
}

// GeminiLiveAudioTranscriptionConfig 音频转录配置
type GeminiLiveAudioTranscriptionConfig struct {
	// 空结构体，存在即表示启用
}

// GeminiLiveProactivityConfig 主动性配置
type GeminiLiveProactivityConfig struct {
	ProactiveAudio bool `json:"proactive_audio,omitempty"` // 主动音频：启用后模型可以选择不回应与当前对话无关的音频
	EmpatheticMode bool `json:"empathetic_mode,omitempty"` // 共情对话：让模型根据输入内容的情绪表达和语气调整回答风格
}

// GeminiLiveRealtimeInputConfig 实时输入配置
type GeminiLiveRealtimeInputConfig struct {
	AutomaticActivityDetection *GeminiLiveAutomaticActivityDetection `json:"automatic_activity_detection,omitempty"`
	ActivityHandling           string                                `json:"activity_handling,omitempty"`
}

// GeminiLiveAutomaticActivityDetection 自动活动检测
type GeminiLiveAutomaticActivityDetection struct {
	Disabled                 bool   `json:"disabled,omitempty"`
	SilenceDurationMs        int    `json:"silence_duration_ms,omitempty"`
	PrefixPaddingMs          int    `json:"prefix_padding_ms,omitempty"`
	EndOfSpeechSensitivity   string `json:"end_of_speech_sensitivity,omitempty"`
	StartOfSpeechSensitivity string `json:"start_of_speech_sensitivity,omitempty"`
}

// GeminiLiveTool 工具定义
type GeminiLiveTool struct {
	FunctionDeclarations []GeminiLiveFunctionDeclaration `json:"function_declarations,omitempty"`
	GoogleSearch         *GeminiLiveGoogleSearch         `json:"google_search,omitempty"`
}

// GeminiLiveFunctionDeclaration 函数声明
type GeminiLiveFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// GeminiLiveGoogleSearch Google 搜索工具
type GeminiLiveGoogleSearch struct{}

// GeminiLiveClientContent 客户端内容消息
type GeminiLiveClientContent struct {
	Turns        []GeminiLiveContent `json:"turns,omitempty"`
	TurnComplete bool                `json:"turn_complete,omitempty"`
}

// GeminiLiveRealtimeInput 实时输入（音频）
type GeminiLiveRealtimeInput struct {
	MediaChunks []GeminiLiveMediaChunk `json:"media_chunks,omitempty"`
}

// GeminiLiveMediaChunk 媒体块
type GeminiLiveMediaChunk struct {
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data"` // Base64 编码的音频数据
}

// GeminiLiveContent 内容结构
type GeminiLiveContent struct {
	Role      string               `json:"role,omitempty"`
	Parts     []GeminiLivePartData `json:"parts,omitempty"`
	ModelTurn *GeminiLiveModelTurn `json:"model_turn,omitempty"`
}

// GeminiLivePartData 内容部分
type GeminiLivePartData struct {
	Text             string                      `json:"text,omitempty"`
	InlineData       *GeminiLiveInlineData       `json:"inline_data,omitempty"`
	FunctionCall     *GeminiLiveFunctionCall     `json:"function_call,omitempty"`
	FunctionResponse *GeminiLiveFunctionResponse `json:"function_response,omitempty"`
}

// GeminiLiveInlineData 内联数据（如音频）
type GeminiLiveInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"` // Base64 编码
}

// GeminiLiveFunctionCall 函数调用
type GeminiLiveFunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args,omitempty"`
	ID   string                 `json:"id,omitempty"`
}

// GeminiLiveFunctionResponse 函数响应
type GeminiLiveFunctionResponse struct {
	Name     string                 `json:"name"`
	Response map[string]interface{} `json:"response"`
	ID       string                 `json:"id,omitempty"`
}

// GeminiLiveToolResponse 工具响应消息
type GeminiLiveToolResponse struct {
	FunctionResponses []GeminiLiveFunctionResponse `json:"function_responses"`
}

// GeminiLiveSetupComplete 设置完成消息
type GeminiLiveSetupComplete struct{}

// GeminiLiveServerContent 服务器内容消息（服务器返回使用 camelCase）
type GeminiLiveServerContent struct {
	ModelTurn           *GeminiLiveModelTurn         `json:"modelTurn,omitempty"`
	TurnComplete        bool                         `json:"turnComplete,omitempty"`
	Interrupted         bool                         `json:"interrupted,omitempty"`
	GroundingMetadata   *GeminiLiveGroundingMetadata `json:"groundingMetadata,omitempty"`
	InputTranscription  *GeminiLiveTranscription     `json:"inputTranscription,omitempty"`
	OutputTranscription *GeminiLiveTranscription     `json:"outputTranscription,omitempty"`
}

// GeminiLiveTranscription 转录内容
type GeminiLiveTranscription struct {
	Text     string `json:"text,omitempty"`
	Finished bool   `json:"finished,omitempty"`
}

// GeminiLiveModelTurn 模型回合
type GeminiLiveModelTurn struct {
	Parts []GeminiLivePartData `json:"parts,omitempty"`
}

// GeminiLiveGroundingMetadata Grounding 元数据
type GeminiLiveGroundingMetadata struct {
	SearchEntryPoint  *GeminiLiveSearchEntryPoint  `json:"search_entry_point,omitempty"`
	GroundingChunks   []GeminiLiveGroundingChunk   `json:"grounding_chunks,omitempty"`
	GroundingSupports []GeminiLiveGroundingSupport `json:"grounding_supports,omitempty"`
	WebSearchQueries  []string                     `json:"web_search_queries,omitempty"`
}

// GeminiLiveSearchEntryPoint 搜索入口点
type GeminiLiveSearchEntryPoint struct {
	RenderedContent string `json:"rendered_content,omitempty"`
}

// GeminiLiveGroundingChunk Grounding 块
type GeminiLiveGroundingChunk struct {
	Web *GeminiLiveWebGroundingChunk `json:"web,omitempty"`
}

// GeminiLiveWebGroundingChunk Web Grounding 块
type GeminiLiveWebGroundingChunk struct {
	URI   string `json:"uri,omitempty"`
	Title string `json:"title,omitempty"`
}

// GeminiLiveGroundingSupport Grounding 支持
type GeminiLiveGroundingSupport struct {
	GroundingChunkIndices []int              `json:"grounding_chunk_indices,omitempty"`
	ConfidenceScores      []float64          `json:"confidence_scores,omitempty"`
	Segment               *GeminiLiveSegment `json:"segment,omitempty"`
}

// GeminiLiveSegment 文本段
type GeminiLiveSegment struct {
	StartIndex int    `json:"start_index,omitempty"`
	EndIndex   int    `json:"end_index,omitempty"`
	Text       string `json:"text,omitempty"`
}

// GeminiLiveToolCall 工具调用消息
type GeminiLiveToolCall struct {
	FunctionCalls []GeminiLiveFunctionCall `json:"function_calls"`
}

// GeminiLiveToolCallCancellation 工具调用取消
type GeminiLiveToolCallCancellation struct {
	IDs []string `json:"ids,omitempty"`
}

// GeminiLiveUsageMetadata 使用量元数据
type GeminiLiveUsageMetadata struct {
	PromptTokenCount     int `json:"prompt_token_count,omitempty"`
	CandidatesTokenCount int `json:"candidates_token_count,omitempty"`
	TotalTokenCount      int `json:"total_token_count,omitempty"`
	// 音频 token 详情
	AudioInputTokens  int `json:"audio_input_tokens,omitempty"`
	AudioOutputTokens int `json:"audio_output_tokens,omitempty"`
	TextInputTokens   int `json:"text_input_tokens,omitempty"`
	TextOutputTokens  int `json:"text_output_tokens,omitempty"`
}

// ProtocolMode 协议模式
type ProtocolMode string

const (
	ProtocolModeOpenAI ProtocolMode = "openai"
	ProtocolModeGemini ProtocolMode = "gemini"
)
