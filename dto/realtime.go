package dto

import "one-api/types"

const (
	RealtimeEventTypeError              = "error"
	RealtimeEventTypeSessionUpdate      = "session.update"
	RealtimeEventTypeConversationCreate = "conversation.item.create"
	RealtimeEventTypeResponseCreate     = "response.create"
	RealtimeEventInputAudioBufferAppend = "input_audio_buffer.append"
)

const (
	RealtimeEventTypeResponseDone                      = "response.done"
	RealtimeEventTypeSessionUpdated                    = "session.updated"
	RealtimeEventTypeSessionCreated                    = "session.created"
	RealtimeEventResponseAudioDelta                    = "response.output_audio.delta"
	RealtimeEventResponseAudioDeltaLegacy              = "response.audio.delta" // Azure OpenAI 使用的旧格式
	RealtimeEventResponseAudioTranscriptionDelta       = "response.output_audio_transcript.delta"
	RealtimeEventResponseAudioTranscriptionDeltaLegacy = "response.audio_transcript.delta" // Azure OpenAI 使用的旧格式
	RealtimeEventResponseFunctionCallArgumentsDelta    = "response.function_call_arguments.delta"
	RealtimeEventResponseFunctionCallArgumentsDone     = "response.function_call_arguments.done"
	RealtimeEventConversationItemCreated               = "conversation.item.created"
)

type RealtimeEvent struct {
	EventId string `json:"event_id"`
	Type    string `json:"type"`
	//PreviousItemId string `json:"previous_item_id"`
	Session  *RealtimeSession   `json:"session,omitempty"`
	Item     *RealtimeItem      `json:"item,omitempty"`
	Error    *types.OpenAIError `json:"error,omitempty"`
	Response *RealtimeResponse  `json:"response,omitempty"`
	Delta    string             `json:"delta,omitempty"`
	Audio    string             `json:"audio,omitempty"`
}

type RealtimeResponse struct {
	Usage *RealtimeUsage `json:"usage"`
}

type RealtimeUsage struct {
	TotalTokens        int                `json:"total_tokens"`
	InputTokens        int                `json:"input_tokens"`
	OutputTokens       int                `json:"output_tokens"`
	InputTokenDetails  InputTokenDetails  `json:"input_token_details"`
	OutputTokenDetails OutputTokenDetails `json:"output_token_details"`
}

type RealtimeSession struct {
	Modalities              []string                `json:"modalities"`
	Instructions            string                  `json:"instructions"`
	Voice                   string                  `json:"voice"`
	InputAudioFormat        string                  `json:"input_audio_format"`
	OutputAudioFormat       string                  `json:"output_audio_format"`
	InputAudioTranscription InputAudioTranscription `json:"input_audio_transcription"`
	TurnDetection           interface{}             `json:"turn_detection"`
	Tools                   []RealTimeTool          `json:"tools"`
	ToolChoice              string                  `json:"tool_choice"`
	Temperature             float64                 `json:"temperature"`
	//MaxResponseOutputTokens int                     `json:"max_response_output_tokens"`
}

type InputAudioTranscription struct {
	Model string `json:"model"`
}

type RealTimeTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type RealtimeItem struct {
	Id        string            `json:"id"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	Role      string            `json:"role"`
	Content   []RealtimeContent `json:"content"`
	Name      *string           `json:"name,omitempty"`
	ToolCalls any               `json:"tool_calls,omitempty"`
	CallId    string            `json:"call_id,omitempty"`
}
type RealtimeContent struct {
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Audio      string `json:"audio,omitempty"` // Base64-encoded audio bytes.
	Transcript string `json:"transcript,omitempty"`
}

// SyncRealtimeRequest 外部系统实时对话请求结构体
// 注意：实时对话使用 WebSocket 连接，user_id、nebula_api_id 和 model 通过查询参数传递
// user_id: Java系统的用户ID（Java端验证用，Go系统不使用）
// nebula_api_id: Go系统的用户ID（对于Go系统来说，这就是userId）
type SyncRealtimeRequest struct {
	UserId      int    `json:"user_id" form:"user_id"`                                // Java系统用户ID，Go系统不使用，但保留以兼容
	NebulaApiId int    `json:"nebula_api_id" form:"nebula_api_id" binding:"required"` // Go系统的userId
	Model       string `json:"model" form:"model" binding:"required"`
	Group       string `json:"group" form:"group"`
}
