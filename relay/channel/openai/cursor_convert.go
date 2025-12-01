package openai

import (
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"strings"
)

// ConvertCursorResponsesToOpenAI 将 Cursor 的 v1/responses 请求转换为 OpenAI 的 v1/chat/completions 请求
// 这是文档要求的核心转换：Cursor → OpenAI Chat Completions
func ConvertCursorResponsesToOpenAI(req *dto.OpenAIResponsesRequest) (*dto.GeneralOpenAIRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}

	chatReq := &dto.GeneralOpenAIRequest{
		Model:  req.Model,
		Stream: req.Stream,
		TopP:   req.TopP,
	}

	// 设置 Temperature
	if req.Temperature != 0 {
		temp := req.Temperature
		chatReq.Temperature = &temp
	}

	// max_output_tokens → max_tokens（文档要求）
	if req.MaxOutputTokens > 0 {
		chatReq.MaxTokens = req.MaxOutputTokens
	}

	// user 字段转换（req.User 是 string 类型）
	if req.User != "" {
		chatReq.User = req.User
	}

	// instructions → system message（文档要求）
	if len(req.Instructions) > 0 {
		var instructions string
		if err := common.Unmarshal(req.Instructions, &instructions); err == nil {
			chatReq.Messages = append(chatReq.Messages, dto.Message{
				Role:    "system",
				Content: instructions,
			})
		} else {
			// 如果不是字符串，尝试作为 JSON 字符串
			chatReq.Messages = append(chatReq.Messages, dto.Message{
				Role:    "system",
				Content: string(req.Instructions),
			})
		}
	}

	// input → messages（文档要求）
	// Cursor 的 input 可能是两种格式：
	// 1. 标准 Responses API 格式：包含 type 字段（input_text, input_image, input_file）
	// 2. Cursor 特殊格式：直接包含 role 和 content 字段的对象数组
	if len(req.Input) > 0 {
		// 先尝试解析为 Cursor 格式（role + content）
		var cursorMessages []map[string]interface{}
		if err := common.Unmarshal(req.Input, &cursorMessages); err == nil && len(cursorMessages) > 0 {
			// 检查第一个元素是否有 role 字段（Cursor 格式）
			if _, ok := cursorMessages[0]["role"].(string); ok {
				// 这是 Cursor 格式，直接转换
				for _, msg := range cursorMessages {
					role, _ := msg["role"].(string)
					content := msg["content"]

					chatReq.Messages = append(chatReq.Messages, dto.Message{
						Role:    role,
						Content: content,
					})
				}
			} else {
				// 不是 Cursor 格式，使用标准 ParseInput 方法
				inputs := req.ParseInput()
				for _, input := range inputs {
					if input.Type == "input_text" || input.Type == "" {
						// 文本输入转换为 user message
						if input.Text != "" {
							chatReq.Messages = append(chatReq.Messages, dto.Message{
								Role:    "user",
								Content: input.Text,
							})
						}
					} else if input.Type == "input_image" {
						// 图片输入转换为 user message with image
						// Message.Content 可以是数组格式
						imageContent := []map[string]interface{}{
							{
								"type": "image_url",
								"image_url": map[string]interface{}{
									"url": input.ImageUrl,
								},
							},
						}
						if input.Text != "" {
							// 如果有文本，先添加文本
							imageContent = append([]map[string]interface{}{
								{
									"type": "text",
									"text": input.Text,
								},
							}, imageContent...)
						}
						chatReq.Messages = append(chatReq.Messages, dto.Message{
							Role:    "user",
							Content: imageContent,
						})
					} else if input.Type == "input_file" {
						// 文件输入（暂时作为文本处理）
						if input.FileUrl != "" {
							chatReq.Messages = append(chatReq.Messages, dto.Message{
								Role:    "user",
								Content: input.FileUrl,
							})
						}
					}
				}
			}
		} else {
			// 解析失败，尝试使用标准 ParseInput 方法
			inputs := req.ParseInput()
			for _, input := range inputs {
				if input.Type == "input_text" || input.Type == "" {
					if input.Text != "" {
						chatReq.Messages = append(chatReq.Messages, dto.Message{
							Role:    "user",
							Content: input.Text,
						})
					}
				} else if input.Type == "input_image" {
					imageContent := []map[string]interface{}{
						{
							"type": "image_url",
							"image_url": map[string]interface{}{
								"url": input.ImageUrl,
							},
						},
					}
					if input.Text != "" {
						imageContent = append([]map[string]interface{}{
							{
								"type": "text",
								"text": input.Text,
							},
						}, imageContent...)
					}
					chatReq.Messages = append(chatReq.Messages, dto.Message{
						Role:    "user",
						Content: imageContent,
					})
				} else if input.Type == "input_file" {
					if input.FileUrl != "" {
						chatReq.Messages = append(chatReq.Messages, dto.Message{
							Role:    "user",
							Content: input.FileUrl,
						})
					}
				}
			}
		}
	}

	// tools 转换（Cursor 格式 → OpenAI 格式）
	if len(req.Tools) > 0 {
		tools, err := ConvertCursorToolsToOpenAI(req.Tools)
		if err != nil {
			return nil, fmt.Errorf("failed to convert tools: %w", err)
		}
		// 只有当 tools 转换成功且不为空时，才设置 tools
		if len(tools) > 0 {
			chatReq.Tools = tools
		}
	}

	// tool_choice 转换
	// 注意：OpenAI API 要求 tool_choice 只有在 tools 存在时才能设置
	if len(req.ToolChoice) > 0 {
		var toolChoice any
		if err := common.Unmarshal(req.ToolChoice, &toolChoice); err == nil {
			// 只有当 tools 存在时，才设置 tool_choice
			if len(chatReq.Tools) > 0 {
				chatReq.ToolChoice = toolChoice
			}
			// 如果 tools 为空，忽略 tool_choice（避免 API 错误）
		}
	}

	// parallel_tool_calls 转换（文档要求同名传递）
	if len(req.ParallelToolCalls) > 0 {
		// OpenAI 的 parallel_tool_calls 是布尔值，Cursor 可能是 JSON
		var parallelToolCalls bool
		if err := common.Unmarshal(req.ParallelToolCalls, &parallelToolCalls); err == nil {
			// 如果成功解析为布尔值，可以设置到某个字段
			// 注意：OpenAI Chat Completions API 可能不支持此字段，需要检查
		}
	}

	// metadata 转换（文档要求同名传递）
	// 注意：OpenAI Chat Completions 的 metadata 字段可能不支持，这里保留原始 JSON
	if len(req.Metadata) > 0 {
		// 尝试解析为 map，如果失败则保留原始 JSON
		var metadata map[string]interface{}
		if err := common.Unmarshal(req.Metadata, &metadata); err == nil {
			// 如果 chatReq.Metadata 支持 map 类型，直接赋值
			// 否则需要根据实际 DTO 结构调整
		}
		// 注意：GeneralOpenAIRequest 可能没有 Metadata 字段，需要检查
	}

	// prompt_cache_key 转换
	if len(req.PromptCacheKey) > 0 {
		var promptCacheKey string
		if err := common.Unmarshal(req.PromptCacheKey, &promptCacheKey); err == nil {
			chatReq.PromptCacheKey = promptCacheKey
		}
	}

	// store 转换
	// 注意：OpenAI Chat Completions 可能不支持 store 字段
	if len(req.Store) > 0 {
		var store bool
		if err := common.Unmarshal(req.Store, &store); err == nil {
			// 如果 chatReq.Store 支持 bool 类型，直接赋值
			// 否则需要根据实际 DTO 结构调整
		}
	}

	// 验证：确保至少有一个 message
	if len(chatReq.Messages) == 0 {
		return nil, fmt.Errorf("messages is required: input field is empty or invalid, and no messages were generated from input")
	}

	return chatReq, nil
}

// ConvertCursorToolsToOpenAI 将 Cursor 的工具格式转换为 OpenAI 的工具格式
// 支持两种格式：
// 1. 标准 OpenAI 格式: [{"type":"function","name":"read_file","description":"...","parameters":{...},"strict":false}]
// 2. 旧 Cursor 格式: [{"tool":"functions.read_file","params":{...}}]
// 输出 OpenAI 格式: [{"type":"function","function":{"name":"read_file","description":"...","parameters":{...}}}]
func ConvertCursorToolsToOpenAI(cursorTools json.RawMessage) ([]dto.ToolCallRequest, error) {
	var cursorToolsArray []map[string]interface{}
	if err := common.Unmarshal(cursorTools, &cursorToolsArray); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor tools: %w", err)
	}

	var openaiTools []dto.ToolCallRequest
	for _, cursorTool := range cursorToolsArray {
		// 首先检查是否是标准 OpenAI 格式（有 type 和 name 字段）
		if toolType, hasType := cursorTool["type"].(string); hasType && toolType == "function" {
			// 标准 OpenAI 格式，直接转换
			functionName, _ := cursorTool["name"].(string)
			description, _ := cursorTool["description"].(string)

			// parameters 字段应该是 JSON Schema 对象（包含 type, properties, required 等）
			var parameters any
			if params, ok := cursorTool["parameters"]; ok {
				parameters = params
			} else {
				// 如果没有 parameters，使用空对象
				parameters = map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				}
			}

			// 构建 OpenAI 工具格式
			openaiTool := dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        functionName,
					Description: description,
					Parameters:  parameters,
				},
			}

			// 注意：OpenAI 的 FunctionRequest 可能不支持 strict 字段
			// 如果支持，可以在这里添加
			// if strict, ok := cursorTool["strict"].(bool); ok {
			//     // 如果 FunctionRequest 有 Strict 字段，设置它
			// }

			openaiTools = append(openaiTools, openaiTool)
			continue
		}

		// 否则，尝试旧的 Cursor 格式（tool 和 params）
		toolName, ok := cursorTool["tool"].(string)
		if !ok {
			// 既不是标准格式，也不是旧格式，跳过
			continue
		}

		// 解析 tool 名称：functions.read_file -> read_file
		// 或者 multi_tool_use.parallel -> 特殊处理
		if toolName == "multi_tool_use.parallel" {
			// 处理并行工具调用
			if params, ok := cursorTool["params"].(map[string]interface{}); ok {
				if toolUses, ok := params["tool_uses"].([]interface{}); ok {
					for _, toolUse := range toolUses {
						if toolUseMap, ok := toolUse.(map[string]interface{}); ok {
							recipientName, _ := toolUseMap["recipient_name"].(string)
							parameters, _ := toolUseMap["parameters"].(map[string]interface{})

							// recipient_name: functions.read_file -> read_file
							functionName := strings.TrimPrefix(recipientName, "functions.")

							openaiTools = append(openaiTools, dto.ToolCallRequest{
								Type: "function",
								Function: dto.FunctionRequest{
									Name:        functionName,
									Description: "", // Cursor 格式可能没有 description
									Parameters:  parameters,
								},
							})
						}
					}
				}
			}
		} else {
			// 普通工具调用（旧格式）
			functionName := strings.TrimPrefix(toolName, "functions.")

			// 获取参数
			var parameters any
			if params, ok := cursorTool["params"].(map[string]interface{}); ok {
				// params 可能是 JSON Schema 对象，也可能是运行参数
				// 检查是否有 type 字段（JSON Schema 的特征）
				if _, hasType := params["type"]; hasType {
					// 是 JSON Schema，直接使用
					parameters = params
				} else {
					// 可能是运行参数，需要包装成 JSON Schema
					// 但这种情况应该很少见，先直接使用
					parameters = params
				}
			} else if _, ok := cursorTool["params"].(string); ok {
				// apply_patch 等工具的参数可能是字符串
				// 这种情况下，我们需要将字符串作为参数的一部分
				// 注意：这里我们创建一个通用的 JSON Schema，因为字符串参数的具体内容无法在工具定义时确定
				parameters = map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"patch": map[string]interface{}{
							"type":        "string",
							"description": "The patch string",
						},
					},
					"required": []string{"patch"},
				}
			} else {
				// 没有参数，使用空对象
				parameters = map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				}
			}

			// 获取 description（如果有）
			description := ""
			if desc, ok := cursorTool["description"].(string); ok {
				description = desc
			}

			openaiTools = append(openaiTools, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        functionName,
					Description: description,
					Parameters:  parameters,
				},
			})
		}
	}

	return openaiTools, nil
}

// ConvertChatCompletionsToResponsesRequest 将 Chat Completions 格式的请求转换为 Responses 格式的请求
// 用于 openai/ 前缀的模型：需要将 Chat Completions 格式转换回 Responses 格式，以调用上游的 /v1/responses 接口
func ConvertChatCompletionsToResponsesRequest(chatReq *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	if chatReq == nil {
		return nil, fmt.Errorf("request is nil")
	}

	responsesReq := &dto.OpenAIResponsesRequest{
		Model:  chatReq.Model,
		Stream: chatReq.Stream,
		TopP:   chatReq.TopP,
		User:   chatReq.User,
	}

	// 设置 Temperature
	if chatReq.Temperature != nil {
		responsesReq.Temperature = *chatReq.Temperature
	}

	// max_tokens → max_output_tokens
	if chatReq.MaxTokens > 0 {
		responsesReq.MaxOutputTokens = uint(chatReq.MaxTokens)
	}

	// messages → input（转换为 Responses 格式）
	if len(chatReq.Messages) > 0 {
		// 提取 system message 作为 instructions
		var userMessages []dto.Message
		for _, msg := range chatReq.Messages {
			if msg.Role == "system" {
				// system message 转换为 instructions
				if contentStr, ok := msg.Content.(string); ok {
					responsesReq.Instructions = json.RawMessage(fmt.Sprintf(`"%s"`, contentStr))
				} else {
					instructionsJSON, _ := common.Marshal(msg.Content)
					responsesReq.Instructions = instructionsJSON
				}
			} else {
				userMessages = append(userMessages, msg)
			}
		}

		// 将 user messages 转换为 input 格式（Cursor 格式：role + content）
		if len(userMessages) > 0 {
			inputJSON, err := common.Marshal(userMessages)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal messages to input: %w", err)
			}
			responsesReq.Input = inputJSON
		}
	}

	// tools 转换（OpenAI 格式 → Cursor 格式）
	if len(chatReq.Tools) > 0 {
		toolsJSON, err := common.Marshal(chatReq.Tools)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tools: %w", err)
		}
		responsesReq.Tools = toolsJSON
	}

	// tool_choice 转换
	if chatReq.ToolChoice != nil {
		toolChoiceJSON, err := common.Marshal(chatReq.ToolChoice)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool_choice: %w", err)
		}
		responsesReq.ToolChoice = toolChoiceJSON
	}

	// metadata 转换
	if len(chatReq.Metadata) > 0 {
		responsesReq.Metadata = chatReq.Metadata
	}

	// prompt_cache_key 转换
	if chatReq.PromptCacheKey != "" {
		responsesReq.PromptCacheKey = json.RawMessage(fmt.Sprintf(`"%s"`, chatReq.PromptCacheKey))
	}

	return responsesReq, nil
}
