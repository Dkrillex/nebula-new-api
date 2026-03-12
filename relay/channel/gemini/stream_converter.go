package gemini

import (
	"fmt"
	"one-api/constant"
	"one-api/dto"
	"strings"
)

// ConvertGeminiStreamToOpenAI 将 Gemini 流式响应转换为 OpenAI 格式的完整中间件
// 这个函数确保所有 chunks 都被正确处理，符合 OpenAI 流式响应规范
func ConvertGeminiStreamToOpenAI(geminiResponse *dto.GeminiChatResponse) (*dto.ChatCompletionsStreamResponse, bool, error) {
	if geminiResponse == nil {
		return nil, false, fmt.Errorf("geminiResponse is nil")
	}

	choices := make([]dto.ChatCompletionsStreamResponseChoice, 0, len(geminiResponse.Candidates))
	isStop := false

	// 遍历所有 candidates
	for _, candidate := range geminiResponse.Candidates {
		// 检查 finish reason
		if candidate.FinishReason != nil {
			isStop = true
		}

		// 创建 choice
		choice := dto.ChatCompletionsStreamResponseChoice{
			Index: int(candidate.Index),
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{},
		}

		// 处理 finish reason
		if candidate.FinishReason != nil {
			switch *candidate.FinishReason {
			case "STOP":
				choice.FinishReason = &constant.FinishReasonStop
			case "MAX_TOKENS":
				choice.FinishReason = &constant.FinishReasonLength
			case "SAFETY":
				choice.FinishReason = &constant.FinishReasonContentFilter
			case "RECITATION":
				choice.FinishReason = &constant.FinishReasonContentFilter
			default:
				choice.FinishReason = &constant.FinishReasonContentFilter
			}
		}

		// 如果 Parts 是 nil，但有 finishReason，仍然需要添加 choice（用于流结束标记）
		if candidate.Content.Parts == nil {
			if candidate.FinishReason != nil {
				choices = append(choices, choice)
			}
			continue
		}

		// 处理 parts
		var texts []string
		var thoughtTexts []string
		isTools := false
		hasThought := false
		hasContent := false

		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				// 处理图片
				if strings.HasPrefix(part.InlineData.MimeType, "image") {
					imgText := fmt.Sprintf("![image](data:%s;base64,%s)", part.InlineData.MimeType, part.InlineData.Data)
					texts = append(texts, imgText)
					hasContent = true
				}
			} else if part.FunctionCall != nil {
				// 处理工具调用
				isTools = true
				if call := getResponseToolCall(&part); call != nil {
					call.SetIndex(len(choice.Delta.ToolCalls))
					choice.Delta.ToolCalls = append(choice.Delta.ToolCalls, *call)
				}
			} else if part.Thought {
				// 处理思考内容
				hasThought = true
				if part.Text != "" {
					thoughtTexts = append(thoughtTexts, part.Text)
				}
			} else {
				// 处理普通文本内容
				hasContent = true
				if part.ExecutableCode != nil {
					texts = append(texts, fmt.Sprintf("```%s\n%s\n```\n", part.ExecutableCode.Language, part.ExecutableCode.Code))
				} else if part.CodeExecutionResult != nil {
					texts = append(texts, fmt.Sprintf("```output\n%s\n```\n", part.CodeExecutionResult.Output))
				} else if part.Text != "" && part.Text != "\n" {
					texts = append(texts, part.Text)
				}
			}
		}

		// 设置思考内容（如果存在）
		if hasThought && len(thoughtTexts) > 0 {
			choice.Delta.SetReasoningContent(strings.Join(thoughtTexts, "\n"))
		}

		// 设置普通内容（如果存在）
		if hasContent && len(texts) > 0 {
			choice.Delta.SetContentString(strings.Join(texts, "\n"))
		} else if !hasThought && len(texts) > 0 {
			// 向后兼容：如果没有思考内容，但有待发送的文本，也设置 content
			choice.Delta.SetContentString(strings.Join(texts, "\n"))
		}

		// 处理工具调用
		if isTools {
			choice.FinishReason = &constant.FinishReasonToolCalls
		}

		// 只有当 choice 有实际内容（content、reasoning_content、tool_calls 或 finish_reason）时才添加
		hasDeltaContent := choice.Delta.GetContentString() != "" ||
			choice.Delta.GetReasoningContent() != "" ||
			len(choice.Delta.ToolCalls) > 0 ||
			choice.FinishReason != nil

		if hasDeltaContent {
			choices = append(choices, choice)
		}
	}

	// 构建响应
	response := &dto.ChatCompletionsStreamResponse{
		Object:  "chat.completion.chunk",
		Choices: choices,
	}

	return response, isStop, nil
}
