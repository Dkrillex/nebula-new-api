package openai

import (
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"strings"
)

// ConvertOpenAIDeltaToCursor 将 OpenAI 的流式响应转换为 Cursor 的 SSE 格式
// OpenAI 格式: data: {"choices":[{"delta":{"content":"Hello"}}]}
// Cursor 格式: event: response.output_text.delta\ndata: {"delta":"Hello"}
func ConvertOpenAIDeltaToCursor(data []byte) (string, bool) {
	if len(data) == 0 {
		return "", false
	}

	// 移除 "data: " 前缀（如果存在）
	dataStr := string(data)
	if strings.HasPrefix(dataStr, "data: ") {
		dataStr = strings.TrimPrefix(dataStr, "data: ")
	}

	// 解析 OpenAI 流式响应
	var streamResponse dto.ChatCompletionsStreamResponse
	if err := common.Unmarshal([]byte(dataStr), &streamResponse); err != nil {
		return "", false
	}

	// 检查是否有 choices
	if len(streamResponse.Choices) == 0 {
		return "", false
	}

	choice := streamResponse.Choices[0]
	delta := choice.Delta

	// 处理文本内容增量
	if delta.Content != nil && *delta.Content != "" {
		cursorEvent := map[string]interface{}{
			"delta": *delta.Content,
		}
		if streamResponse.Id != "" {
			cursorEvent["id"] = streamResponse.Id
		}
		cursorJSON, _ := json.Marshal(cursorEvent)
		return fmt.Sprintf("event: response.output_text.delta\ndata: %s\n\n", string(cursorJSON)), true
	}

	// 处理工具调用增量
	if len(delta.ToolCalls) > 0 {
		// Cursor 的工具调用格式需要特殊处理
		// 这里先转换为 Cursor 的工具调用格式
		toolCalls := convertOpenAIToolCallsToCursor(delta.ToolCalls)
		if len(toolCalls) > 0 {
			cursorEvent := map[string]interface{}{
				"tool_calls": toolCalls,
			}
			if streamResponse.Id != "" {
				cursorEvent["id"] = streamResponse.Id
			}
			cursorJSON, _ := json.Marshal(cursorEvent)
			return fmt.Sprintf("event: response.output_item.added\ndata: %s\n\n", string(cursorJSON)), true
		}
	}

	// 处理完成事件
	if choice.FinishReason != nil && *choice.FinishReason != "" {
		cursorEvent := map[string]interface{}{
			"finish_reason": *choice.FinishReason,
		}
		if streamResponse.Id != "" {
			cursorEvent["id"] = streamResponse.Id
		}
		cursorJSON, _ := json.Marshal(cursorEvent)
		return fmt.Sprintf("event: response.completed\ndata: %s\n\n", string(cursorJSON)), true
	}

	return "", false
}

// convertOpenAIToolCallsToCursor 将 OpenAI 的工具调用转换为 Cursor 格式
func convertOpenAIToolCallsToCursor(openaiToolCalls []dto.ToolCallResponse) []map[string]interface{} {
	var cursorToolCalls []map[string]interface{}

	for _, toolCall := range openaiToolCalls {
		// OpenAI 格式: {"id":"call_xxx","type":"function","function":{"name":"read_file","arguments":"{...}"}}
		// Cursor 格式: {"tool":"functions.read_file","params":{...}}

		functionName := toolCall.Function.Name
		cursorToolName := "functions." + functionName

		// 解析 arguments
		var params map[string]interface{}
		if toolCall.Function.Arguments != "" {
			_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &params)
		} else if toolCall.Function.Parameters != nil {
			// 如果 Parameters 是 map，直接使用
			if paramsMap, ok := toolCall.Function.Parameters.(map[string]interface{}); ok {
				params = paramsMap
			}
		}

		// 对于 apply_patch，参数可能是字符串
		if functionName == "apply_patch" && params == nil {
			if toolCall.Function.Arguments != "" {
				params = map[string]interface{}{
					"patch": toolCall.Function.Arguments,
				}
			}
		}

		cursorToolCall := map[string]interface{}{
			"tool":   cursorToolName,
			"params": params,
		}

		if toolCall.ID != "" {
			cursorToolCall["call_id"] = toolCall.ID
		}

		cursorToolCalls = append(cursorToolCalls, cursorToolCall)
	}

	return cursorToolCalls
}

// ConvertOpenAIResponseToCursor 将 OpenAI 的非流式响应转换为 Cursor 格式
// OpenAI 格式: {"id":"xxx","choices":[{"message":{"content":"Hello"}}]}
// Cursor 格式: {"id":"xxx","output_text":"Hello"}
func ConvertOpenAIResponseToCursor(chatResponse *dto.OpenAITextResponse) map[string]interface{} {
	if chatResponse == nil {
		return nil
	}

	cursorResponse := map[string]interface{}{
		"id": chatResponse.Id,
	}

	// 提取文本内容
	if len(chatResponse.Choices) > 0 {
		choice := chatResponse.Choices[0]
		message := choice.Message

		// 提取 Content
		if message.Content != nil {
			if contentStr, ok := message.Content.(string); ok {
				cursorResponse["output_text"] = contentStr
			}
		}

		// 处理工具调用（ToolCalls 是 json.RawMessage，需要解析）
		if len(message.ToolCalls) > 0 {
			var toolCalls []dto.ToolCallResponse
			if err := common.Unmarshal(message.ToolCalls, &toolCalls); err == nil {
				cursorToolCalls := convertOpenAIToolCallsToCursor(toolCalls)
				cursorResponse["tool_calls"] = cursorToolCalls
			}
		}
	}

	// 添加 usage 信息（Usage 是嵌入的，不是指针）
	usage := chatResponse.Usage
	if usage.TotalTokens > 0 {
		cursorResponse["usage"] = usage
	}

	return cursorResponse
}
