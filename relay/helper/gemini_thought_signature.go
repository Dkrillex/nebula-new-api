package helper

import (
	"one-api/dto"
)

// GeminiThoughtSignatureSkip 当客户端未回传 thought_signature 时，用此值可让 Vertex/Gemini 跳过校验（会略影响表现，但可避免 400）
// 见 https://docs.cloud.google.com/vertex-ai/generative-ai/docs/thought-signatures
const GeminiThoughtSignatureSkip = "skip_thought_signature_validator"

// EnsureGeminiThoughtSignaturesForOpenAIRequest 对透传到 Vertex/Gemini OpenAI 兼容端点的请求，
// 为所有 assistant 消息中的 tool_calls 补全缺失的 thought_signature，避免第二次及以后请求报 400。
// Vertex Chat Completions API 要求 thought_signature 放在 extra_content.google.thought_signature，见：
// https://docs.cloud.google.com/vertex-ai/generative-ai/docs/thought-signatures#chat-completions
func EnsureGeminiThoughtSignaturesForOpenAIRequest(req *dto.GeneralOpenAIRequest) {
	if req == nil || len(req.Messages) == 0 {
		return
	}
	for i := range req.Messages {
		msg := &req.Messages[i]
		if msg.Role != "assistant" || msg.ToolCalls == nil {
			continue
		}
		toolCalls := msg.ParseToolCalls()
		if len(toolCalls) == 0 {
			continue
		}
		patched := false
		for j := range toolCalls {
			tc := &toolCalls[j]
			// 若已有 thought_signature（顶层或 extra_content）则不覆盖
			if tc.ThoughtSignature != "" {
				continue
			}
			if tc.ExtraContent != nil {
				if g, ok := tc.ExtraContent["google"].(map[string]any); ok && g["thought_signature"] != "" {
					continue
				}
			}
			// Vertex 要求放在 extra_content.google.thought_signature
			tc.ExtraContent = map[string]any{"google": map[string]any{"thought_signature": GeminiThoughtSignatureSkip}}
			patched = true
		}
		if patched {
			msg.SetToolCalls(toolCalls)
		}
	}
}
