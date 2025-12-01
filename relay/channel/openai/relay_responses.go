package openai

import (
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	if responsesResponse.HasImageGenerationCall() {
		c.Set("image_generation_call", true)
		c.Set("image_generation_call_quality", responsesResponse.GetQuality())
		c.Set("image_generation_call_size", responsesResponse.GetSize())
	}

	// 检查是否需要转换为 Chat Completions 格式
	if c.GetBool("convert_responses_to_chat") {
		// 打印原始 Responses 响应格式（完整 JSON）
		originalResponseJSON, _ := common.Marshal(responsesResponse)
		logger.LogInfo(c, "[Responses->Chat转换] ========== 步骤1: 原始 Responses 响应格式 ==========")
		logger.LogInfo(c, fmt.Sprintf("[Responses->Chat转换] 完整JSON: %s", string(originalResponseJSON)))

		// 打印 Responses 响应的关键字段
		logger.LogInfo(c, "[Responses->Chat转换] Responses关键字段:")
		logger.LogInfo(c, fmt.Sprintf("  - ID: %s", responsesResponse.ID))
		logger.LogInfo(c, fmt.Sprintf("  - Model: %s", responsesResponse.Model))
		logger.LogInfo(c, fmt.Sprintf("  - Status: %s", responsesResponse.Status))
		logger.LogInfo(c, fmt.Sprintf("  - CreatedAt: %d", responsesResponse.CreatedAt))
		logger.LogInfo(c, fmt.Sprintf("  - Output数量: %d", len(responsesResponse.Output)))
		for i, output := range responsesResponse.Output {
			outputJSON, _ := common.Marshal(output)
			logger.LogInfo(c, fmt.Sprintf("  - Output[%d]: %s", i, common.TruncateBase64Content(string(outputJSON))))
		}
		if responsesResponse.Usage != nil {
			usageJSON, _ := common.Marshal(responsesResponse.Usage)
			logger.LogInfo(c, fmt.Sprintf("  - Usage: %s", string(usageJSON)))
		}
		if len(responsesResponse.Metadata) > 0 {
			logger.LogInfo(c, fmt.Sprintf("  - Metadata: %s", string(responsesResponse.Metadata)))
		}

		chatResponse := convertResponsesToChatCompletions(&responsesResponse, info, c)

		// 打印转换后的 Chat Completions 响应格式（完整 JSON）
		convertedResponseJSON, _ := common.Marshal(chatResponse)
		logger.LogInfo(c, "[Responses->Chat转换] ========== 步骤2: 转换后的 Chat Completions 响应格式 ==========")
		logger.LogInfo(c, fmt.Sprintf("[Responses->Chat转换] 完整JSON: %s", string(convertedResponseJSON)))

		// 打印 Chat Completions 响应的关键字段
		logger.LogInfo(c, "[Responses->Chat转换] Chat Completions关键字段:")
		logger.LogInfo(c, fmt.Sprintf("  - Id: %s", chatResponse.Id))
		logger.LogInfo(c, fmt.Sprintf("  - Model: %s", chatResponse.Model))
		logger.LogInfo(c, fmt.Sprintf("  - Object: %s", chatResponse.Object))
		logger.LogInfo(c, fmt.Sprintf("  - Created: %v", chatResponse.Created))
		logger.LogInfo(c, fmt.Sprintf("  - Choices数量: %d", len(chatResponse.Choices)))
		for i, choice := range chatResponse.Choices {
			choiceJSON, _ := common.Marshal(choice)
			logger.LogInfo(c, fmt.Sprintf("  - Choice[%d]: %s", i, common.TruncateBase64Content(string(choiceJSON))))
		}
		if chatResponse.Usage.PromptTokens > 0 || chatResponse.Usage.CompletionTokens > 0 {
			usageJSON, _ := common.Marshal(chatResponse.Usage)
			logger.LogInfo(c, fmt.Sprintf("  - Usage: %s", string(usageJSON)))
		}

		// 对比分析：提取关键字段进行对比
		logger.LogInfo(c, "[Responses->Chat转换] ========== 字段对比分析 ==========")
		logger.LogInfo(c, fmt.Sprintf("  ID: %s -> %s", responsesResponse.ID, chatResponse.Id))
		logger.LogInfo(c, fmt.Sprintf("  Model: %s -> %s", responsesResponse.Model, chatResponse.Model))
		logger.LogInfo(c, fmt.Sprintf("  Output数量: %d -> Choices数量: %d", len(responsesResponse.Output), len(chatResponse.Choices)))
		if responsesResponse.Usage != nil {
			logger.LogInfo(c, fmt.Sprintf("  Usage - InputTokens: %d -> PromptTokens: %d", responsesResponse.Usage.InputTokens, chatResponse.Usage.PromptTokens))
			logger.LogInfo(c, fmt.Sprintf("  Usage - OutputTokens: %d -> CompletionTokens: %d", responsesResponse.Usage.OutputTokens, chatResponse.Usage.CompletionTokens))
			logger.LogInfo(c, fmt.Sprintf("  Usage - TotalTokens: %d -> TotalTokens: %d", responsesResponse.Usage.TotalTokens, chatResponse.Usage.TotalTokens))
		}
		logger.LogInfo(c, "[Responses->Chat转换] ==========================================")

		// 将 Chat Completions 响应转换为 map，以便添加 Responses 格式
		chatResponseMap := make(map[string]any)
		chatResponseJSON, _ := common.Marshal(chatResponse)
		if err := common.Unmarshal(chatResponseJSON, &chatResponseMap); err == nil {
			// 将原始的 Responses 响应转换为 map
			responsesMap := make(map[string]any)
			responsesJSON, _ := common.Marshal(responsesResponse)
			if err := common.Unmarshal(responsesJSON, &responsesMap); err == nil {
				// 1. 将 Responses 格式的所有字段平铺到最外层（如果 Chat Completions 中没有同名字段）
				for key, value := range responsesMap {
					// 只添加 Chat Completions 中不存在的字段，避免覆盖
					if _, exists := chatResponseMap[key]; !exists {
						chatResponseMap[key] = value
					}
				}

				// 2. 同时保留完整的 Responses 对象在 responses 字段中
				chatResponseMap["responses"] = responsesMap
				logger.LogInfo(c, "[Responses->Chat转换] 已添加 Responses 格式字段到最外层，并保留完整对象在 responses 字段中")
			}
		}

		responseBody, err = common.Marshal(chatResponseMap)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// compute usage
	usage := dto.Usage{}
	if responsesResponse.Usage != nil {
		usage.PromptTokens = responsesResponse.Usage.InputTokens
		usage.CompletionTokens = responsesResponse.Usage.OutputTokens
		usage.TotalTokens = responsesResponse.Usage.TotalTokens
		if responsesResponse.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = responsesResponse.Usage.InputTokensDetails.CachedTokens
		}
	}
	if info == nil || info.ResponsesUsageInfo == nil || info.ResponsesUsageInfo.BuiltInTools == nil {
		return &usage, nil
	}
	// 解析 Tools 用量
	for _, tool := range responsesResponse.Tools {
		buildToolinfo, ok := info.ResponsesUsageInfo.BuiltInTools[common.Interface2String(tool["type"])]
		if !ok || buildToolinfo == nil {
			logger.LogError(c, fmt.Sprintf("BuiltInTools not found for tool type: %v", tool["type"]))
			continue
		}
		buildToolinfo.CallCount++
	}
	return &usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var usage = &dto.Usage{}
	var responseTextBuilder strings.Builder

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err == nil {
			// 检查是否需要转换为 Chat Completions 格式
			if c.GetBool("convert_responses_to_chat") {
				chatStreamData := convertResponsesStreamToChatCompletions(streamResponse, info, c)
				if chatStreamData != "" {
					// 添加调试日志，记录转换后的数据格式
					if streamResponse.Type == "response.output_item.added" ||
						streamResponse.Type == "response.function_call_arguments.delta" ||
						streamResponse.Type == "response.custom_tool_call_input.delta" ||
						streamResponse.Type == "response.output_item.done" {
						logger.LogInfo(c, fmt.Sprintf("[Responses->Chat转换] 事件类型: %s, 转换后的数据: %s", streamResponse.Type, common.TruncateBase64Content(chatStreamData)))
					}
					helper.StringData(c, chatStreamData)
				}
			} else {
				sendResponsesStreamData(c, streamResponse, data)
			}
			switch streamResponse.Type {
			case "response.completed":
				if streamResponse.Response != nil {
					if streamResponse.Response.Usage != nil {
						if streamResponse.Response.Usage.InputTokens != 0 {
							usage.PromptTokens = streamResponse.Response.Usage.InputTokens
						}
						if streamResponse.Response.Usage.OutputTokens != 0 {
							usage.CompletionTokens = streamResponse.Response.Usage.OutputTokens
						}
						if streamResponse.Response.Usage.TotalTokens != 0 {
							usage.TotalTokens = streamResponse.Response.Usage.TotalTokens
						}
						if streamResponse.Response.Usage.InputTokensDetails != nil {
							usage.PromptTokensDetails.CachedTokens = streamResponse.Response.Usage.InputTokensDetails.CachedTokens
						}
					}
					if streamResponse.Response.HasImageGenerationCall() {
						c.Set("image_generation_call", true)
						c.Set("image_generation_call_quality", streamResponse.Response.GetQuality())
						c.Set("image_generation_call_size", streamResponse.Response.GetSize())
					}
				}
			case "response.output_text.delta":
				// 处理输出文本
				responseTextBuilder.WriteString(streamResponse.Delta)
			case dto.ResponsesOutputTypeItemDone:
				// 函数调用处理
				if streamResponse.Item != nil {
					switch streamResponse.Item.Type {
					case dto.BuildInCallWebSearchCall:
						if info != nil && info.ResponsesUsageInfo != nil && info.ResponsesUsageInfo.BuiltInTools != nil {
							if webSearchTool, exists := info.ResponsesUsageInfo.BuiltInTools[dto.BuildInToolWebSearchPreview]; exists && webSearchTool != nil {
								webSearchTool.CallCount++
							}
						}
					}
				}
			}
		} else {
			logger.LogError(c, "failed to unmarshal stream response: "+err.Error())
		}
		return true
	})

	if usage.CompletionTokens == 0 {
		// 计算输出文本的 token 数量
		tempStr := responseTextBuilder.String()
		if len(tempStr) > 0 {
			// 非正常结束，使用输出文本的 token 数量
			completionTokens := service.CountTextToken(tempStr, info.UpstreamModelName)
			usage.CompletionTokens = completionTokens
		}
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens != 0 {
		usage.PromptTokens = info.PromptTokens
	}

	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens

	return usage, nil
}

// convertResponsesToChatCompletions 将 Responses 格式转换为 Chat Completions 格式
func convertResponsesToChatCompletions(responsesResp *dto.OpenAIResponsesResponse, info *relaycommon.RelayInfo, c *gin.Context) *dto.OpenAITextResponse {
	chatResp := &dto.OpenAITextResponse{
		Id:      responsesResp.ID,
		Model:   responsesResp.Model,
		Object:  "chat.completion",
		Created: int64(responsesResp.CreatedAt),
		Choices: make([]dto.OpenAITextResponseChoice, 0),
	}

	// 转换 Usage
	if responsesResp.Usage != nil {
		chatResp.Usage = dto.Usage{
			PromptTokens:     responsesResp.Usage.InputTokens,
			CompletionTokens: responsesResp.Usage.OutputTokens,
			TotalTokens:      responsesResp.Usage.TotalTokens,
		}
		if responsesResp.Usage.InputTokensDetails != nil {
			chatResp.Usage.PromptTokensDetails.CachedTokens = responsesResp.Usage.InputTokensDetails.CachedTokens
		}
	}

	// 转换 Output 到 Choices
	// 只处理 type="message" 的 output，跳过 type="reasoning" 等其他类型
	choiceIndex := 0
	for _, output := range responsesResp.Output {
		// 只处理 message 类型的 output，且状态为 completed
		if output.Type != "message" || output.Status != "completed" {
			continue
		}

		choice := dto.OpenAITextResponseChoice{
			Index:        choiceIndex,
			FinishReason: "stop",
		}

		// 构建消息内容
		// Responses API 的 content 数组中，type 是 "output_text" 而不是 "text"
		var textContents []string
		// 确保 Content 不为 nil
		if output.Content == nil {
			output.Content = []dto.ResponsesOutputContent{}
		}
		for _, content := range output.Content {
			if content.Type == "output_text" && content.Text != "" {
				textContents = append(textContents, content.Text)
			}
		}

		// 将内容转换为 Message
		var messageContent any
		if len(textContents) == 1 {
			// 单个文本内容，直接使用字符串
			messageContent = textContents[0]
		} else if len(textContents) > 1 {
			// 多个文本内容，合并为一个字符串（或者可以保持数组格式，根据 Chat Completions 规范）
			// 这里先合并，如果需要保持数组格式可以后续调整
			combinedText := ""
			for i, text := range textContents {
				if i > 0 {
					combinedText += "\n"
				}
				combinedText += text
			}
			messageContent = combinedText
		}

		choice.Message = dto.Message{
			Role:    output.Role,
			Content: messageContent,
		}

		// 处理工具调用（如果有）
		// Responses API 的工具调用可能存储在：
		// 1. content 的 annotations 中
		// 2. content 的 type 为 "tool_call" 或类似类型
		// 3. 作为单独的 output 项（但当前只处理 type="message" 的 output）
		var toolCalls []dto.ToolCallResponse
		toolCallIndex := 0
		// 确保 Content 不为 nil（已在上面处理，这里再次确保）
		if output.Content == nil {
			output.Content = []dto.ResponsesOutputContent{}
		}
		logger.LogInfo(c, fmt.Sprintf("[工具调用转换] 开始处理 output[%d] 的工具调用，content 数量: %d", choiceIndex, len(output.Content)))
		for i := range output.Content {
			content := &output.Content[i]
			// 确保 Annotations 不为 nil（避免序列化为 null）
			if content.Annotations == nil {
				content.Annotations = []interface{}{}
			}
			logger.LogInfo(c, fmt.Sprintf("[工具调用转换] content[%d]: type=%s, text长度=%d, annotations数量=%d", i, content.Type, len(content.Text), len(content.Annotations)))
			// 检查 content type 是否是工具调用
			if content.Type == "tool_call" || content.Type == "function_call" {
				// 尝试从 content 中提取工具调用信息
				// content.Text 可能包含 JSON 格式的工具调用信息
				if content.Text != "" {
					var toolCallData map[string]interface{}
					if err := common.Unmarshal([]byte(content.Text), &toolCallData); err == nil {
						toolCall := dto.ToolCallResponse{
							ID:   getStringFromMap(toolCallData, "id", ""),
							Type: "function",
							Function: dto.FunctionResponse{
								Name:      getStringFromMap(toolCallData, "name", ""),
								Arguments: getStringFromMap(toolCallData, "arguments", ""),
							},
						}
						// 如果 ID 为空，生成一个
						if toolCall.ID == "" {
							toolCall.ID = fmt.Sprintf("call_%d_%d", choiceIndex, toolCallIndex)
						}
						toolCalls = append(toolCalls, toolCall)
						toolCallIndex++
					}
				}
			}

			// 检查 annotations 中是否有工具调用
			if len(content.Annotations) > 0 {
				for _, annotation := range content.Annotations {
					if annMap, ok := annotation.(map[string]interface{}); ok {
						// 检查是否是工具调用类型
						if annType, ok := annMap["type"].(string); ok {
							if annType == "tool_call" || annType == "function_call" {
								// 提取工具调用信息
								if functionData, ok := annMap["function"].(map[string]interface{}); ok {
									toolCall := dto.ToolCallResponse{
										ID:   getStringFromMap(annMap, "id", ""),
										Type: "function",
										Function: dto.FunctionResponse{
											Name:      getStringFromMap(functionData, "name", ""),
											Arguments: getStringFromMap(functionData, "arguments", ""),
										},
									}
									// 如果 ID 为空，生成一个
									if toolCall.ID == "" {
										toolCall.ID = fmt.Sprintf("call_%d_%d", choiceIndex, toolCallIndex)
									}
									toolCalls = append(toolCalls, toolCall)
									toolCallIndex++
								}
							}
						}
					}
				}
			}
		}

		// 如果有工具调用，设置到 message 中并更新 finish_reason
		if len(toolCalls) > 0 {
			logger.LogInfo(c, fmt.Sprintf("[工具调用转换] 找到 %d 个工具调用", len(toolCalls)))
			for i, tc := range toolCalls {
				logger.LogInfo(c, fmt.Sprintf("[工具调用转换] toolCall[%d]: id=%s, name=%s, arguments=%s", i, tc.ID, tc.Function.Name, tc.Function.Arguments))
			}
			choice.Message.SetToolCalls(toolCalls)
			choice.FinishReason = "tool_calls"
		} else {
			logger.LogInfo(c, "[工具调用转换] 未找到工具调用")
		}

		chatResp.Choices = append(chatResp.Choices, choice)
		choiceIndex++
	}

	// 如果没有 choices，创建一个空的
	if len(chatResp.Choices) == 0 {
		chatResp.Choices = []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: "",
				},
				FinishReason: "stop",
			},
		}
	}

	return chatResp
}

// getStringFromMap 从 map 中安全地获取字符串值
func getStringFromMap(m map[string]interface{}, key string, defaultValue string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
		// 如果不是字符串，尝试转换为 JSON 字符串
		if jsonBytes, err := common.Marshal(val); err == nil {
			return string(jsonBytes)
		}
	}
	return defaultValue
}

// convertResponsesStreamToChatCompletions 将 Responses 流式响应转换为 Chat Completions 流式响应
// c 用于存储工具调用参数的累积状态
func convertResponsesStreamToChatCompletions(streamResponse dto.ResponsesStreamResponse, info *relaycommon.RelayInfo, c *gin.Context) string {
	var chatStreamResponse dto.ChatCompletionsStreamResponse

	// 根据不同的响应类型进行转换
	switch streamResponse.Type {
	case "response.created":
		// 初始化响应
		if streamResponse.Response != nil {
			chatStreamResponse = dto.ChatCompletionsStreamResponse{
				Id:      streamResponse.Response.ID,
				Object:  "chat.completion.chunk",
				Created: int64(streamResponse.Response.CreatedAt),
				Model:   streamResponse.Response.Model,
				Choices: []dto.ChatCompletionsStreamResponseChoice{
					{
						Index: 0,
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							Role: "assistant",
						},
					},
				},
			}
		}
	case "response.output_text.delta":
		// 文本增量
		if streamResponse.Delta != "" {
			responseID := ""
			if streamResponse.Response != nil {
				responseID = streamResponse.Response.ID
			}
			chatStreamResponse = dto.ChatCompletionsStreamResponse{
				Id:     responseID,
				Object: "chat.completion.chunk",
				Model:  info.UpstreamModelName,
				Choices: []dto.ChatCompletionsStreamResponseChoice{
					{
						Index: 0,
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							Content: &streamResponse.Delta,
						},
					},
				},
			}
		}
	case dto.ResponsesOutputTypeItemAdded:
		// 处理 output item 添加事件（工具调用开始）
		if streamResponse.Item != nil && (streamResponse.Item.Type == "function_call" || streamResponse.Item.Type == "custom_tool_call") {
			responseID := ""
			if streamResponse.Response != nil {
				responseID = streamResponse.Response.ID
			}

			callID := streamResponse.Item.CallID
			if callID == "" {
				callID = streamResponse.Item.ID
			}
			if callID == "" {
				callID = fmt.Sprintf("call_%d", 0)
			}

			// 初始化工具调用参数累积状态（存储在 gin.Context 中）
			itemID := streamResponse.Item.ID
			if itemID != "" {
				// 使用 item_id 作为 key 存储工具调用信息
				toolCallInfo := map[string]interface{}{
					"call_id":   callID,
					"name":      streamResponse.Item.Name,
					"arguments": "",
					"input":     "",
					"type":      streamResponse.Item.Type, // 记录类型，用于区分 function_call 和 custom_tool_call
				}
				c.Set("tool_call_"+itemID, toolCallInfo)
			}

			// 发送初始工具调用（参数为空）
			// 使用 OutputIndex 作为工具调用的索引（如果为 0 则使用 0）
			toolCallIndex := 0
			if streamResponse.OutputIndex > 0 {
				toolCallIndex = streamResponse.OutputIndex - 1 // OutputIndex 从 1 开始，转换为从 0 开始
			}
			toolCall := dto.ToolCallResponse{
				ID:    callID,
				Type:  "function",
				Index: &toolCallIndex, // 设置工具调用的索引
				Function: dto.FunctionResponse{
					Name:      streamResponse.Item.Name,
					Arguments: "", // 初始参数为空
				},
			}

			chatStreamResponse = dto.ChatCompletionsStreamResponse{
				Id:     responseID,
				Object: "chat.completion.chunk",
				Model:  info.UpstreamModelName,
				Choices: []dto.ChatCompletionsStreamResponseChoice{
					{
						Index: 0,
						Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
							ToolCalls: []dto.ToolCallResponse{toolCall},
						},
					},
				},
			}
		} else {
			// 非工具调用类型（如 reasoning），返回空字符串（不发送）
			return ""
		}
	case dto.ResponsesOutputTypeItemDone:
		// 处理 output item 完成事件（工具调用完成）
		if streamResponse.Item != nil {
			responseID := ""
			if streamResponse.Response != nil {
				responseID = streamResponse.Response.ID
			}

			// 检查是否是工具调用相关的 item
			var toolCalls []dto.ToolCallResponse

			// 如果 item 是 function_call 或 custom_tool_call 类型，直接从 item 中提取工具调用信息
			if streamResponse.Item.Type == "function_call" || streamResponse.Item.Type == "custom_tool_call" {
				callID := streamResponse.Item.CallID
				if callID == "" {
					callID = streamResponse.Item.ID
				}
				if callID == "" {
					callID = fmt.Sprintf("call_%d", 0)
				}

				// 从 arguments 或 input 字段获取参数（在 response.output_item.done 时已完整）
				arguments := ""
				if streamResponse.Item.Type == "function_call" {
					if streamResponse.Item.Arguments != "" {
						arguments = streamResponse.Item.Arguments
					} else {
						// 如果 Item.Arguments 为空，尝试从累积状态中获取
						itemID := streamResponse.Item.ID
						if itemID != "" {
							toolCallInfoKey := "tool_call_" + itemID
							if toolCallInfoVal, exists := c.Get(toolCallInfoKey); exists {
								if toolCallInfo, ok := toolCallInfoVal.(map[string]interface{}); ok {
									if args, ok := toolCallInfo["arguments"].(string); ok && args != "" {
										arguments = args
									}
								}
							}
						}
						// 如果仍然为空，尝试从 Content 中提取
						if arguments == "" && streamResponse.Item.Content != nil {
							for i := range streamResponse.Item.Content {
								content := &streamResponse.Item.Content[i]
								if content.Type == "tool_call" || content.Type == "function_call" {
									if content.Text != "" {
										var toolCallData map[string]interface{}
										if err := common.Unmarshal([]byte(content.Text), &toolCallData); err == nil {
											arguments = getStringFromMap(toolCallData, "arguments", "")
										}
									}
								}
							}
						}
					}
				} else if streamResponse.Item.Type == "custom_tool_call" {
					// custom_tool_call 使用 input 字段
					if streamResponse.Item.Input != "" {
						arguments = streamResponse.Item.Input
					} else {
						// 如果 Item.Input 为空，尝试从累积状态中获取
						itemID := streamResponse.Item.ID
						if itemID != "" {
							toolCallInfoKey := "tool_call_" + itemID
							if toolCallInfoVal, exists := c.Get(toolCallInfoKey); exists {
								if toolCallInfo, ok := toolCallInfoVal.(map[string]interface{}); ok {
									if input, ok := toolCallInfo["input"].(string); ok && input != "" {
										arguments = input
									}
								}
							}
						}
					}
				}

				// 使用 OutputIndex 作为工具调用的索引（如果为 0 则使用 0）
				toolCallIndex := 0
				if streamResponse.OutputIndex > 0 {
					toolCallIndex = streamResponse.OutputIndex - 1 // OutputIndex 从 1 开始，转换为从 0 开始
				}
				toolCall := dto.ToolCallResponse{
					ID:    callID,
					Type:  "function",
					Index: &toolCallIndex, // 设置工具调用的索引
					Function: dto.FunctionResponse{
						Name:      streamResponse.Item.Name,
						Arguments: arguments,
					},
				}
				toolCalls = append(toolCalls, toolCall)

				// 清理累积状态
				itemID := streamResponse.Item.ID
				if itemID != "" {
					c.Set("tool_call_"+itemID, nil)
				}
			} else {
				// 确保 Content 不为 nil（避免序列化为 null）
				if streamResponse.Item.Content == nil {
					streamResponse.Item.Content = []dto.ResponsesOutputContent{}
				}
				for i := range streamResponse.Item.Content {
					content := &streamResponse.Item.Content[i]
					// 确保 Annotations 不为 nil（避免序列化为 null）
					if content.Annotations == nil {
						content.Annotations = []interface{}{}
					}
					if content.Type == "tool_call" || content.Type == "function_call" {
						if content.Text != "" {
							var toolCallData map[string]interface{}
							if err := common.Unmarshal([]byte(content.Text), &toolCallData); err == nil {
								toolCall := dto.ToolCallResponse{
									ID:   getStringFromMap(toolCallData, "id", ""),
									Type: "function",
									Function: dto.FunctionResponse{
										Name:      getStringFromMap(toolCallData, "name", ""),
										Arguments: getStringFromMap(toolCallData, "arguments", ""),
									},
								}
								if toolCall.ID == "" {
									toolCall.ID = fmt.Sprintf("call_%d", len(toolCalls))
								}
								toolCalls = append(toolCalls, toolCall)
							}
						}
					}
				}
			}

			// 如果有工具调用，发送工具调用 chunk
			if len(toolCalls) > 0 {
				chatStreamResponse = dto.ChatCompletionsStreamResponse{
					Id:     responseID,
					Object: "chat.completion.chunk",
					Model:  info.UpstreamModelName,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
								ToolCalls: toolCalls,
							},
						},
					},
				}
			} else {
				// 如果没有工具调用（比如是 reasoning 类型），返回空字符串（不发送）
				// 这样可以避免发送空的响应导致客户端解析错误
				return ""
			}
		} else {
			// Item 为 nil，返回空字符串（不发送）
			return ""
		}
	case "response.function_call_arguments.delta":
		// 处理工具调用参数增量事件
		// 注意：在流式传输中，我们只累积参数，不立即发送
		// 因为 Cursor 可能在接收到不完整的 JSON 时尝试解析，导致 "invalid arguments" 错误
		// 完整的工具调用会在 response.output_item.done 时发送
		if streamResponse.ItemID != "" && streamResponse.Delta != "" {
			// 从 gin.Context 获取工具调用信息
			itemID := streamResponse.ItemID
			toolCallInfoKey := "tool_call_" + itemID
			toolCallInfoVal, exists := c.Get(toolCallInfoKey)
			if !exists {
				return ""
			}

			toolCallInfo, ok := toolCallInfoVal.(map[string]interface{})
			if !ok {
				return ""
			}

			// 累积参数（但不发送，等待完成事件）
			currentArgs := ""
			if args, ok := toolCallInfo["arguments"].(string); ok {
				currentArgs = args
			}
			currentArgs += streamResponse.Delta
			toolCallInfo["arguments"] = currentArgs
			c.Set(toolCallInfoKey, toolCallInfo)

			// 不发送增量更新，返回空字符串
			// 完整的工具调用会在 response.output_item.done 时发送
			return ""
		} else {
			return ""
		}
	case "response.function_call_arguments.done":
		// 参数传输完成，不需要额外处理（完整参数会在 response.output_item.done 时发送）
		return ""
	case "response.custom_tool_call_input.delta":
		// 处理 custom_tool_call 输入增量事件
		// 注意：在流式传输中，我们只累积输入，不立即发送
		// 因为 Cursor 可能在接收到不完整的输入时尝试解析，导致 "invalid arguments" 错误
		// 完整的工具调用会在 response.output_item.done 时发送
		if streamResponse.ItemID != "" && streamResponse.Delta != "" {
			// 从 gin.Context 获取工具调用信息
			itemID := streamResponse.ItemID
			toolCallInfoKey := "tool_call_" + itemID
			toolCallInfoVal, exists := c.Get(toolCallInfoKey)
			if !exists {
				return ""
			}

			toolCallInfo, ok := toolCallInfoVal.(map[string]interface{})
			if !ok {
				return ""
			}

			// 累积输入（custom_tool_call 使用 input 字段，但不发送，等待完成事件）
			currentInput := ""
			if input, ok := toolCallInfo["input"].(string); ok {
				currentInput = input
			}
			currentInput += streamResponse.Delta
			toolCallInfo["input"] = currentInput
			c.Set(toolCallInfoKey, toolCallInfo)

			// 不发送增量更新，返回空字符串
			// 完整的工具调用会在 response.output_item.done 时发送
			return ""
		} else {
			return ""
		}
	case "response.custom_tool_call_input.done":
		// 输入传输完成，不需要额外处理（完整输入会在 response.output_item.done 时发送）
		return ""
	case "response.completed":
		// 完成响应
		if streamResponse.Response != nil {
			chatStreamResponse = dto.ChatCompletionsStreamResponse{
				Id:      streamResponse.Response.ID,
				Object:  "chat.completion.chunk",
				Created: int64(streamResponse.Response.CreatedAt),
				Model:   streamResponse.Response.Model,
				Choices: []dto.ChatCompletionsStreamResponseChoice{
					{
						Index:        0,
						FinishReason: stringPtr("stop"),
						Delta:        dto.ChatCompletionsStreamResponseChoiceDelta{},
					},
				},
			}
			if streamResponse.Response.Usage != nil {
				chatStreamResponse.Usage = &dto.Usage{
					PromptTokens:     streamResponse.Response.Usage.InputTokens,
					CompletionTokens: streamResponse.Response.Usage.OutputTokens,
					TotalTokens:      streamResponse.Response.Usage.TotalTokens,
				}
			}
		}
	default:
		// 其他类型，返回空字符串（不发送）
		return ""
	}

	// 检查 chatStreamResponse 是否被正确初始化（Choices 不能为 nil）
	if len(chatStreamResponse.Choices) == 0 && chatStreamResponse.Id == "" {
		// 如果响应没有被初始化，返回空字符串（不发送）
		return ""
	}

	// 确保 Choices 不为 nil（避免序列化为 null）
	if chatStreamResponse.Choices == nil {
		chatStreamResponse.Choices = []dto.ChatCompletionsStreamResponseChoice{}
	}

	// 直接返回纯 Chat Completions 格式，不混入 Responses 字段
	// 因为 Cursor 调用的是 /v1/chat/completions，期望的是标准 OpenAI Chat Completions 流式响应格式
	jsonData, err := common.Marshal(chatStreamResponse)
	if err != nil {
		common.SysError(fmt.Sprintf("failed to marshal chat stream response: %v", err))
		return ""
	}

	return string(jsonData)
}

// stringPtr 返回字符串指针
func stringPtr(s string) *string {
	return &s
}

// convertChatCompletionsToResponses 将 Chat Completions 格式转换为 Responses 格式
func convertChatCompletionsToResponses(chatResp *dto.OpenAITextResponse) *dto.OpenAIResponsesResponse {
	if chatResp == nil {
		return nil
	}

	responsesResp := &dto.OpenAIResponsesResponse{
		ID:        chatResp.Id,
		Object:    "responses",
		CreatedAt: 0,
		Status:    "completed",
		Model:     chatResp.Model,
		Output:    make([]dto.ResponsesOutput, 0),
	}

	// 转换 Created 时间
	if created, ok := chatResp.Created.(int64); ok {
		responsesResp.CreatedAt = int(created)
	} else if created, ok := chatResp.Created.(int); ok {
		responsesResp.CreatedAt = created
	}

	// 转换 Usage
	if chatResp.Usage.TotalTokens > 0 {
		responsesResp.Usage = &dto.Usage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		}
		if chatResp.Usage.PromptTokensDetails.CachedTokens > 0 {
			responsesResp.Usage.InputTokensDetails = &dto.InputTokenDetails{
				CachedTokens: chatResp.Usage.PromptTokensDetails.CachedTokens,
			}
		}
	}

	// 转换 Choices 到 Output
	for _, choice := range chatResp.Choices {
		output := dto.ResponsesOutput{
			Type:    "message",
			ID:      fmt.Sprintf("msg_%d", choice.Index),
			Status:  "completed",
			Role:    choice.Message.Role,
			Content: make([]dto.ResponsesOutputContent, 0),
		}

		// 转换 Content
		if choice.Message.Content != nil {
			var textContent string
			if contentStr, ok := choice.Message.Content.(string); ok {
				textContent = contentStr
			} else if contentBytes, err := common.Marshal(choice.Message.Content); err == nil {
				textContent = string(contentBytes)
			}

			if textContent != "" {
				output.Content = append(output.Content, dto.ResponsesOutputContent{
					Type:        "output_text",
					Text:        textContent,
					Annotations: make([]interface{}, 0),
				})
			}
		}

		// 转换工具调用
		if len(choice.Message.ToolCalls) > 0 {
			var toolCalls []dto.ToolCallResponse
			if err := common.Unmarshal(choice.Message.ToolCalls, &toolCalls); err == nil {
				for _, toolCall := range toolCalls {
					// 创建工具调用的 content
					toolCallContent := dto.ResponsesOutputContent{
						Type:        "tool_call",
						Text:        "",
						Annotations: make([]interface{}, 0),
					}

					// 构建工具调用 JSON
					toolCallData := map[string]interface{}{
						"id":   toolCall.ID,
						"type": "function",
						"name": toolCall.Function.Name,
					}
					if toolCall.Function.Arguments != "" {
						toolCallData["arguments"] = toolCall.Function.Arguments
					}

					if toolCallJSON, err := common.Marshal(toolCallData); err == nil {
						toolCallContent.Text = string(toolCallJSON)
					}

					output.Content = append(output.Content, toolCallContent)
				}
			}
		}

		responsesResp.Output = append(responsesResp.Output, output)
	}

	return responsesResp
}

// convertChatCompletionsStreamToResponses 将 Chat Completions 流式响应转换为 Responses 流式响应
func convertChatCompletionsStreamToResponses(streamResp *dto.ChatCompletionsStreamResponse) *dto.ResponsesStreamResponse {
	if streamResp == nil {
		return nil
	}

	responsesStreamResp := &dto.ResponsesStreamResponse{}

	// 根据流式响应的内容确定类型
	if len(streamResp.Choices) > 0 {
		choice := streamResp.Choices[0]
		delta := choice.Delta

		// 处理文本增量
		if delta.Content != nil && *delta.Content != "" {
			responsesStreamResp.Type = "response.output_text.delta"
			responsesStreamResp.Delta = *delta.Content
		}

		// 处理工具调用
		if len(delta.ToolCalls) > 0 {
			responsesStreamResp.Type = dto.ResponsesOutputTypeItemAdded
			// 转换工具调用
			for _, toolCall := range delta.ToolCalls {
				output := dto.ResponsesOutput{
					Type:   "function_call",
					ID:     toolCall.ID,
					Status: "in_progress",
					Name:   toolCall.Function.Name,
					CallID: toolCall.ID,
				}
				if toolCall.Function.Arguments != "" {
					output.Arguments = toolCall.Function.Arguments
				}
				responsesStreamResp.Item = &output
				break // 只处理第一个工具调用
			}
		}

		// 处理完成事件
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			responsesStreamResp.Type = "response.completed"
		}
	}

	// 设置 Response 信息（如果有）
	if streamResp.Id != "" {
		responsesStreamResp.Response = &dto.OpenAIResponsesResponse{
			ID:     streamResp.Id,
			Model:  streamResp.Model,
			Status: "completed",
		}
		if streamResp.Usage != nil {
			responsesStreamResp.Response.Usage = &dto.Usage{
				InputTokens:  streamResp.Usage.PromptTokens,
				OutputTokens: streamResp.Usage.CompletionTokens,
				TotalTokens:  streamResp.Usage.TotalTokens,
			}
		}
	}

	return responsesStreamResp
}
