package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/relay/channel/openrouter"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	WebSearchMaxUsesLow    = 1
	WebSearchMaxUsesMedium = 5
	WebSearchMaxUsesHigh   = 10
)

// defaultEphemeralCacheControl 用于「每一个输入都写入缓存」：客户端未传 cache_control 时默认标记为 ephemeral，让上游对每段输入做缓存
var defaultEphemeralCacheControl = json.RawMessage(`{"type":"ephemeral"}`)

func cacheControlOrDefault(from json.RawMessage) json.RawMessage {
	if len(from) > 0 {
		return from
	}
	// 不默认加 ephemeral，避免上游缓存导致重试时返回同一份 tool_calls、Cursor 重复执行（双份 md）
	// 若需恢复「每段输入都写缓存」行为，可改回 return defaultEphemeralCacheControl
	return nil
}

// capCacheControlBlocks 保证整个请求中带 cache_control 的块不超过 max 个，避免上游返回 400（Found 6/5）导致 Cursor 重试和重复 AskQuestion
func capCacheControlBlocks(req *dto.ClaudeRequest, max int) {
	if max <= 0 {
		return
	}
	var withCC []*dto.ClaudeMediaMessage
	if sys, ok := req.System.([]dto.ClaudeMediaMessage); ok {
		for i := range sys {
			if len(sys[i].CacheControl) > 0 {
				withCC = append(withCC, &sys[i])
			}
		}
	}
	for i := range req.Messages {
		content := req.Messages[i].Content
		if contents, ok := content.([]dto.ClaudeMediaMessage); ok {
			for j := range contents {
				if len(contents[j].CacheControl) > 0 {
					withCC = append(withCC, &contents[j])
				}
			}
		}
	}
	for i := 0; i < len(withCC)-max; i++ {
		withCC[i].CacheControl = nil
	}
}

func stopReasonClaude2OpenAI(reason string) string {
	switch reason {
	case "stop_sequence":
		return "stop"
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return reason
	}
}

func RequestOpenAI2ClaudeComplete(textRequest dto.GeneralOpenAIRequest) *dto.ClaudeRequest {

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		Prompt:        "",
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopP:          textRequest.TopP,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
	}
	if claudeRequest.MaxTokensToSample == 0 {
		claudeRequest.MaxTokensToSample = 4096
	}
	prompt := ""
	for _, message := range textRequest.Messages {
		if message.Role == "user" {
			prompt += fmt.Sprintf("\n\nHuman: %s", message.StringContent())
		} else if message.Role == "assistant" {
			prompt += fmt.Sprintf("\n\nAssistant: %s", message.StringContent())
		} else if message.Role == "system" {
			if prompt == "" {
				prompt = message.StringContent()
			}
		}
	}
	prompt += "\n\nAssistant:"
	claudeRequest.Prompt = prompt
	return &claudeRequest
}

func RequestOpenAI2ClaudeMessage(c *gin.Context, textRequest dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) (*dto.ClaudeRequest, error) {
	claudeTools := make([]any, 0, len(textRequest.Tools))

	for _, tool := range textRequest.Tools {
		// 有效 name：优先 function，否则用 Cursor/扁平格式的顶层 name
		name := tool.Function.Name
		if name == "" {
			name = tool.Name
		}
		desc := tool.Function.Description
		if desc == "" {
			desc = tool.Description
		}
		// 有效 params：优先 function.parameters，否则 function.input_schema，再否则顶层 parameters/input_schema（Cursor 扁平格式）
		params, _ := tool.Function.Parameters.(map[string]any)
		usedInputSchema := false
		if params == nil {
			params, _ = tool.Function.InputSchema.(map[string]any)
			usedInputSchema = params != nil
		}
		if params == nil {
			params, _ = tool.Parameters.(map[string]any)
		}
		if params == nil {
			params, _ = tool.InputSchema.(map[string]any)
			usedInputSchema = params != nil
		}
		if params != nil && name != "" {
			claudeTool := dto.Tool{
				Name:        name,
				Description: desc,
			}
			claudeTool.InputSchema = make(map[string]interface{})
			if params["type"] != nil {
				if s, ok := params["type"].(string); ok {
					claudeTool.InputSchema["type"] = s
				}
			}
			if claudeTool.InputSchema["type"] == nil {
				claudeTool.InputSchema["type"] = "object"
			}
			claudeTool.InputSchema["properties"] = params["properties"]
			claudeTool.InputSchema["required"] = params["required"]
			// 过滤掉不被 AWS Bedrock 支持的字段（仅当使用 AWS 渠道时）
			unsupportedFields := map[string]bool{
				"type":       true,
				"properties": true,
				"required":   true,
			}
			// 如果是 AWS Bedrock，需要过滤掉 input_examples 字段
			if info != nil && info.ChannelType == constant.ChannelTypeAws {
				unsupportedFields["input_examples"] = true
			}
			for s, a := range params {
				if unsupportedFields[s] {
					continue
				}
				claudeTool.InputSchema[s] = a
			}
			claudeTools = append(claudeTools, &claudeTool)
			if common.DebugEnabled && usedInputSchema {
				logger.LogDebug(c, fmt.Sprintf("[Claude tools] tool from input_schema: name=%s", name))
			}
		} else {
			logger.LogWarn(c, fmt.Sprintf("[Claude tools] tool skipped (no parameters/input_schema or name): name=%s", name))
		}
	}
	if len(textRequest.Tools) > 0 {
		logger.LogInfo(c, fmt.Sprintf("[Claude tools] request_tools=%d converted=%d", len(textRequest.Tools), len(claudeTools)))
	}

	// Web search tool
	// https://docs.anthropic.com/en/docs/agents-and-tools/tool-use/web-search-tool
	if textRequest.WebSearchOptions != nil {
		webSearchTool := dto.ClaudeWebSearchTool{
			Type: "web_search_20250305",
			Name: "web_search",
		}

		// 处理 user_location
		if textRequest.WebSearchOptions.UserLocation != nil {
			anthropicUserLocation := &dto.ClaudeWebSearchUserLocation{
				Type: "approximate", // 固定为 "approximate"
			}

			// 解析 UserLocation JSON
			var userLocationMap map[string]interface{}
			if err := json.Unmarshal(textRequest.WebSearchOptions.UserLocation, &userLocationMap); err == nil {
				// 检查是否有 approximate 字段
				if approximateData, ok := userLocationMap["approximate"].(map[string]interface{}); ok {
					if timezone, ok := approximateData["timezone"].(string); ok && timezone != "" {
						anthropicUserLocation.Timezone = timezone
					}
					if country, ok := approximateData["country"].(string); ok && country != "" {
						anthropicUserLocation.Country = country
					}
					if region, ok := approximateData["region"].(string); ok && region != "" {
						anthropicUserLocation.Region = region
					}
					if city, ok := approximateData["city"].(string); ok && city != "" {
						anthropicUserLocation.City = city
					}
				}
			}

			webSearchTool.UserLocation = anthropicUserLocation
		}

		// 处理 search_context_size 转换为 max_uses
		if textRequest.WebSearchOptions.SearchContextSize != "" {
			switch textRequest.WebSearchOptions.SearchContextSize {
			case "low":
				webSearchTool.MaxUses = WebSearchMaxUsesLow
			case "medium":
				webSearchTool.MaxUses = WebSearchMaxUsesMedium
			case "high":
				webSearchTool.MaxUses = WebSearchMaxUsesHigh
			}
		}

		claudeTools = append(claudeTools, &webSearchTool)
	}

	claudeRequest := dto.ClaudeRequest{
		Model:         textRequest.Model,
		MaxTokens:     textRequest.GetMaxTokens(),
		StopSequences: nil,
		Temperature:   textRequest.Temperature,
		TopP:          textRequest.TopP,
		TopK:          textRequest.TopK,
		Stream:        textRequest.Stream,
		Tools:         claudeTools,
	}

	// 处理 tool_choice 和 parallel_tool_calls
	if textRequest.ToolChoice != nil || textRequest.ParallelTooCalls != nil {
		claudeToolChoice := mapToolChoice(textRequest.ToolChoice, textRequest.ParallelTooCalls)
		if claudeToolChoice != nil {
			claudeRequest.ToolChoice = claudeToolChoice
		}
	}

	if claudeRequest.MaxTokens == 0 {
		claudeRequest.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(textRequest.Model))
	}

	if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(textRequest.Model, "-thinking") {

		// 因为BudgetTokens 必须大于1024
		if claudeRequest.MaxTokens < 1280 {
			claudeRequest.MaxTokens = 1280
		}

		// BudgetTokens 为 max_tokens 的 80%
		budgetTokens := int(float64(claudeRequest.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)

		// AWS Bedrock 要求 max_tokens 必须严格大于 budget_tokens
		// 如果 budget_tokens >= max_tokens，则调整 max_tokens 使其至少比 budget_tokens 大 1
		if budgetTokens >= int(claudeRequest.MaxTokens) {
			claudeRequest.MaxTokens = uint(budgetTokens + 1)
		}

		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: common.GetPointer[int](budgetTokens),
		}
		// TODO: 临时处理
		// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
		claudeRequest.TopP = 0
		claudeRequest.Temperature = common.GetPointer[float64](1.0)
		claudeRequest.Model = strings.TrimSuffix(textRequest.Model, "-thinking")
	}

	if textRequest.ReasoningEffort != "" {
		var budgetTokens int
		switch textRequest.ReasoningEffort {
		case "low":
			budgetTokens = 1280
		case "medium":
			budgetTokens = 2048
		case "high":
			budgetTokens = 4096
		}

		// AWS Bedrock 要求 max_tokens 必须严格大于 budget_tokens
		// 如果 budget_tokens >= max_tokens，则调整 max_tokens 使其至少比 budget_tokens 大 1
		if budgetTokens >= int(claudeRequest.MaxTokens) {
			claudeRequest.MaxTokens = uint(budgetTokens + 1)
		}

		claudeRequest.Thinking = &dto.Thinking{
			Type:         "enabled",
			BudgetTokens: common.GetPointer[int](budgetTokens),
		}
	}

	// 指定了 reasoning 参数,覆盖 budgetTokens
	if textRequest.Reasoning != nil {
		var reasoning openrouter.RequestReasoning
		if err := common.Unmarshal(textRequest.Reasoning, &reasoning); err != nil {
			return nil, err
		}

		budgetTokens := reasoning.MaxTokens
		if budgetTokens > 0 {
			// AWS Bedrock 要求 max_tokens 必须严格大于 budget_tokens
			// 如果 budget_tokens >= max_tokens，则调整 max_tokens 使其至少比 budget_tokens 大 1
			if budgetTokens >= int(claudeRequest.MaxTokens) {
				claudeRequest.MaxTokens = uint(budgetTokens + 1)
			}

			claudeRequest.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: &budgetTokens,
			}
		}
	}

	// 统一检查：无论 thinking 是从哪里来的（用户传入、-thinking 后缀、reasoning_effort、reasoning 参数等），都要确保 max_tokens > budget_tokens
	// 这个要求适用于所有 Claude API（Anthropic 官方 API 和 AWS Bedrock）
	// budget_tokens 是思考预算，必须小于 max_tokens（总输出限制）
	if claudeRequest.Thinking != nil && claudeRequest.Thinking.BudgetTokens != nil {
		budgetTokens := *claudeRequest.Thinking.BudgetTokens
		// Claude API 要求 max_tokens 必须严格大于 budget_tokens
		// 如果 budget_tokens >= max_tokens，则调整 max_tokens 使其至少比 budget_tokens 大 1
		if budgetTokens >= int(claudeRequest.MaxTokens) {
			claudeRequest.MaxTokens = uint(budgetTokens + 1)
		}
	}

	if textRequest.Stop != nil {
		// stop maybe string/array string, convert to array string
		switch textRequest.Stop.(type) {
		case string:
			claudeRequest.StopSequences = []string{textRequest.Stop.(string)}
		case []interface{}:
			stopSequences := make([]string, 0)
			for _, stop := range textRequest.Stop.([]interface{}) {
				stopSequences = append(stopSequences, stop.(string))
			}
			claudeRequest.StopSequences = stopSequences
		}
	}
	formatMessages := make([]dto.Message, 0)
	lastMessage := dto.Message{
		Role: "tool",
	}
	for i, message := range textRequest.Messages {
		if message.Role == "" {
			textRequest.Messages[i].Role = "user"
		}
		fmtMessage := dto.Message{
			Role:    message.Role,
			Content: message.Content,
		}
		if message.Role == "tool" {
			fmtMessage.ToolCallId = message.ToolCallId
		}
		if message.Role == "assistant" && message.ToolCalls != nil {
			fmtMessage.ToolCalls = message.ToolCalls
		}
		if lastMessage.Role == message.Role && lastMessage.Role != "tool" {
			if lastMessage.IsStringContent() && message.IsStringContent() {
				fmtMessage.SetStringContent(strings.Trim(fmt.Sprintf("%s %s", lastMessage.StringContent(), message.StringContent()), "\""))
				// delete last message
				formatMessages = formatMessages[:len(formatMessages)-1]
			}
		}
		// 空内容或空数组会导致上游模型丢失上下文（如 Cursor Plan/Agent 出现重复“你好”），统一用占位
		if fmtMessage.Content == nil {
			fmtMessage.SetStringContent("...")
		} else if arr, ok := fmtMessage.Content.([]any); ok && len(arr) == 0 {
			fmtMessage.SetStringContent("...")
		}
		formatMessages = append(formatMessages, fmtMessage)
		lastMessage = fmtMessage
	}

	claudeMessages := make([]dto.ClaudeMessage, 0)
	isFirstMessage := true
	// 初始化system消息数组，用于累积多个system消息
	var systemMessages []dto.ClaudeMediaMessage

	for _, message := range formatMessages {
		if message.Role == "system" {
			// 根据Claude API规范，system字段使用数组格式更有通用性
			// 仅在最后一个 system 块加 cache_control，使「前缀」到该块时更容易达到官方最低可缓存长度（Opus 4.5 为 4096），避免每个小块都成 breakpoint 导致前缀不足而不写入缓存
			if message.IsStringContent() {
				systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
					Type:         "text",
					Text:         common.GetPointer[string](message.StringContent()),
					CacheControl: cacheControlOrDefault(nil), // 与 messages 一致：不默认加 ephemeral，避免重试时上游返回缓存导致重复执行
				})
			} else {
				contents := message.ParseContent()
				for i, ctx := range contents {
					if ctx.Type != "text" {
						continue
					}
					isLastSystemBlock := (i == len(contents)-1)
					cacheControl := json.RawMessage(nil)
					if isLastSystemBlock {
						cacheControl = cacheControlOrDefault(ctx.CacheControl)
					}
					systemMessages = append(systemMessages, dto.ClaudeMediaMessage{
						Type:         "text",
						Text:         common.GetPointer[string](ctx.Text),
						CacheControl: cacheControl,
					})
				}
			}
		} else {
			if isFirstMessage {
				isFirstMessage = false
				if message.Role != "user" {
					// fix: first message is assistant, add user message
					claudeMessage := dto.ClaudeMessage{
						Role: "user",
						Content: []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string]("..."),
							},
						},
					}
					claudeMessages = append(claudeMessages, claudeMessage)
				}
			}
			claudeMessage := dto.ClaudeMessage{
				Role: message.Role,
			}
			if message.Role == "tool" {
				if len(claudeMessages) > 0 && claudeMessages[len(claudeMessages)-1].Role == "user" {
					lastMessage := claudeMessages[len(claudeMessages)-1]
					if content, ok := lastMessage.Content.(string); ok {
						lastMessage.Content = []dto.ClaudeMediaMessage{
							{
								Type: "text",
								Text: common.GetPointer[string](content),
							},
						}
					}

					// 检查是否已存在相同的 tool_use_id，避免重复添加
					contents := lastMessage.Content.([]dto.ClaudeMediaMessage)
					toolResultExists := false
					for _, existingContent := range contents {
						if existingContent.Type == "tool_result" && existingContent.ToolUseId == message.ToolCallId {
							toolResultExists = true
							break
						}
					}

					// 只有不存在重复的 tool_use_id 时才添加新的 tool_result
					if !toolResultExists {
						lastMessage.Content = append(contents, dto.ClaudeMediaMessage{
							Type:      "tool_result",
							ToolUseId: message.ToolCallId,
							Content:   message.Content,
						})
						claudeMessages[len(claudeMessages)-1] = lastMessage
					}
					continue
				} else {
					// 检查是否已存在独立的tool_result消息，避免创建重复消息
					toolResultExists := false
					for i := len(claudeMessages) - 1; i >= 0; i-- {
						if claudeMessages[i].Role == "user" {
							if contents, ok := claudeMessages[i].Content.([]dto.ClaudeMediaMessage); ok {
								for _, content := range contents {
									if content.Type == "tool_result" && content.ToolUseId == message.ToolCallId {
										toolResultExists = true
										break
									}
								}
							}
							break // 只检查最近的用户消息
						}
					}

					// 只有不存在重复的 tool_use_id 时才创建新的用户消息
					if !toolResultExists {
						claudeMessage.Role = "user"
						claudeMessage.Content = []dto.ClaudeMediaMessage{
							{
								Type:      "tool_result",
								ToolUseId: message.ToolCallId,
								Content:   message.Content,
							},
						}
					} else {
						// 如果已存在，跳过当前消息
						continue
					}
				}
			} else if message.IsStringContent() && message.ToolCalls == nil {
				claudeMessage.Content = message.StringContent()
			} else {
				// 仅在每条消息的最后一个 content 块加 cache_control，使前缀到该块时更容易达到官方最低可缓存长度（如 Opus 4.5 为 4096）
				mediaContents := message.ParseContent()
				claudeMediaMessages := make([]dto.ClaudeMediaMessage, 0, len(mediaContents))
				for i, mediaMessage := range mediaContents {
					isLastContentBlock := (i == len(mediaContents)-1)
					cacheControl := json.RawMessage(nil)
					if isLastContentBlock {
						cacheControl = cacheControlOrDefault(mediaMessage.CacheControl)
					}
					claudeMediaMessage := dto.ClaudeMediaMessage{
						Type:         mediaMessage.Type,
						CacheControl: cacheControl,
					}
					if mediaMessage.Type == "text" {
						claudeMediaMessage.Text = common.GetPointer[string](mediaMessage.Text)
					} else {
						imageUrl := mediaMessage.GetImageMedia()
						claudeMediaMessage.Type = "image"
						claudeMediaMessage.Source = &dto.ClaudeMessageSource{
							Type: "base64",
						}
						// 判断是否是url
						if strings.HasPrefix(imageUrl.Url, "http") {
							// 是url，获取图片的类型和base64编码的数据
							fileData, err := service.GetFileBase64FromUrl(c, imageUrl.Url, "formatting image for Claude")
							if err != nil {
								return nil, fmt.Errorf("get file base64 from url failed: %s", err.Error())
							}
							claudeMediaMessage.Source.MediaType = fileData.MimeType
							claudeMediaMessage.Source.Data = fileData.Base64Data
						} else {
							_, format, base64String, err := service.DecodeBase64ImageData(imageUrl.Url)
							if err != nil {
								return nil, err
							}
							claudeMediaMessage.Source.MediaType = "image/" + format
							claudeMediaMessage.Source.Data = base64String
						}
					}
					claudeMediaMessages = append(claudeMediaMessages, claudeMediaMessage)
				}
				if message.ToolCalls != nil {
					for _, toolCall := range message.ParseToolCalls() {
						inputObj := make(map[string]any)
						if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &inputObj); err != nil {
							common.SysLog("tool call function arguments is not a map[string]any: " + fmt.Sprintf("%v", toolCall.Function.Arguments))
							continue
						}
						claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
							Type:  "tool_use",
							Id:    toolCall.ID,
							Name:  toolCall.Function.Name,
							Input: inputObj,
						})
					}
				}
				// Cursor 等客户端可能把 tool_use 放在 content 数组里而非 tool_calls 字段，需从 content 中解析
				if message.ToolCalls == nil && message.Content != nil {
					var contentArr []map[string]any
					contentBytes, _ := json.Marshal(message.Content)
					if err := json.Unmarshal(contentBytes, &contentArr); err == nil {
						for _, item := range contentArr {
							if t, _ := item["type"].(string); t == "tool_use" {
								id, _ := item["id"].(string)
								name, _ := item["name"].(string)
								input := item["input"]
								if input == nil {
									input = map[string]any{}
								}
								claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
									Type:  "tool_use",
									Id:    id,
									Name:  name,
									Input: input,
								})
								continue
							}
							// Cursor 等可能把多条 tool 结果放在一条 user 消息的 content 里（type: tool_result），ParseContent 不识别会丢
							if message.Role == "user" {
								if t, _ := item["type"].(string); t == "tool_result" {
									toolUseId, _ := item["tool_use_id"].(string)
									content := item["content"]
									claudeMediaMessages = append(claudeMediaMessages, dto.ClaudeMediaMessage{
										Type:      "tool_result",
										ToolUseId: toolUseId,
										Content:   content,
									})
								}
							}
						}
					}
				}
				// 若转换后仍为空（如 Cursor 发来 content:[]），避免上游收到空内容导致上下文丢失
				if len(claudeMediaMessages) == 0 {
					claudeMessage.Content = "..."
				} else {
					claudeMessage.Content = claudeMediaMessages
				}
			}
			claudeMessages = append(claudeMessages, claudeMessage)
		}
	}

	// 设置累积的system消息
	if len(systemMessages) > 0 {
		claudeRequest.System = systemMessages
	}

	claudeRequest.Prompt = ""
	claudeRequest.Messages = claudeMessages

	// 上游 Claude API 最多允许 4 个带 cache_control 的块，超过会 400，导致 Cursor 重试并出现重复 AskQuestion
	capCacheControlBlocks(&claudeRequest, 4)

	return &claudeRequest, nil
}

func StreamResponseClaude2OpenAI(reqMode int, claudeResponse *dto.ClaudeResponse, claudeInfo *ClaudeResponseInfo) *dto.ChatCompletionsStreamResponse {
	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Model = claudeResponse.Model
	response.Choices = make([]dto.ChatCompletionsStreamResponseChoice, 0)
	tools := make([]dto.ToolCallResponse, 0)
	fcIdx := 0
	if claudeResponse.Index != nil {
		fcIdx = *claudeResponse.Index - 1
		if fcIdx < 0 {
			fcIdx = 0
		}
		if claudeInfo != nil {
			claudeInfo.CurrentToolIndex = fcIdx
		}
	}
	var choice dto.ChatCompletionsStreamResponseChoice
	if reqMode == RequestModeCompletion {
		choice.Delta.SetContentString(claudeResponse.Completion)
		finishReason := stopReasonClaude2OpenAI(claudeResponse.StopReason)
		if finishReason != "null" {
			choice.FinishReason = &finishReason
		}
	} else {
		if claudeResponse.Type == "message_start" {
			// 方案D调试日志：记录每个SSE事件
			common.SysLog(fmt.Sprintf("[Claude SSE] event=message_start message_id=%s model=%s", claudeResponse.Message.Id, claudeResponse.Message.Model))
			response.Id = claudeResponse.Message.Id
			response.Model = claudeResponse.Message.Model
			//claudeUsage = &claudeResponse.Message.Usage
			choice.Delta.SetContentString("")
			choice.Delta.Role = "assistant"
			if claudeInfo != nil {
				claudeInfo.ToolCallCount = 0
				claudeInfo.ToolArgsBuffer = make(map[int]string)
				claudeInfo.LastToolCallNames = make(map[int]string)
				claudeInfo.PendingToolCalls = make(map[int]*PendingToolCall) // 方案D：初始化工具缓冲区
			}
		} else if claudeResponse.Type == "content_block_start" {
			if claudeResponse.ContentBlock != nil {
				// 如果是文本块，尽可能发送首段文本（若存在）
				if claudeResponse.ContentBlock.Type == "text" && claudeResponse.ContentBlock.Text != nil {
					choice.Delta.SetContentString(*claudeResponse.ContentBlock.Text)
				}
				if claudeResponse.ContentBlock.Type == "tool_use" {
					// ========== 方案D：只缓冲，不立即 flush ==========
					if claudeInfo != nil {
						toolId := claudeResponse.ContentBlock.Id
						if toolId == "" {
							toolId = claudeResponse.ContentBlock.ToolUseId
						}
						toolName := claudeResponse.ContentBlock.Name

						// 方案D调试日志：记录每个content_block_start事件的完整信息
						common.SysLog(fmt.Sprintf("[Claude SSE] event=content_block_start type=tool_use index=%v id=%s name=%s ToolUseId=%s", claudeResponse.Index, toolId, toolName, claudeResponse.ContentBlock.ToolUseId))

						// 检查是否是对同一 tool 的重复/补充 content_block_start（按 ID 去重）
						existingIdx := -1
						for idx, pending := range claudeInfo.PendingToolCalls {
							if pending != nil && pending.ID == toolId {
								existingIdx = idx
								break
							}
						}

						if existingIdx >= 0 {
							// 同 ID 的补充包：只更新 name（如果有）
							common.SysLog(fmt.Sprintf("[Claude SSE] tool_use same_id update: existingIdx=%d id=%s newName=%s", existingIdx, toolId, toolName))
							if toolName != "" {
								claudeInfo.PendingToolCalls[existingIdx].Name = toolName
								claudeInfo.LastToolCallNames[existingIdx] = toolName
							}
							fcIdx = existingIdx
							claudeInfo.CurrentToolIndex = fcIdx
						} else {
							// 新工具：分配新 index，存入缓冲区
							fcIdx = claudeInfo.ToolCallCount
							claudeInfo.CurrentToolIndex = fcIdx
							claudeInfo.ToolCallCount++

							// 初始化各种 map
							if claudeInfo.LastToolCallIds == nil {
								claudeInfo.LastToolCallIds = make(map[int]string)
							}
							if claudeInfo.LastToolCallNames == nil {
								claudeInfo.LastToolCallNames = make(map[int]string)
							}
							if claudeInfo.ToolArgsBuffer == nil {
								claudeInfo.ToolArgsBuffer = make(map[int]string)
							}
							if claudeInfo.PendingToolCalls == nil {
								claudeInfo.PendingToolCalls = make(map[int]*PendingToolCall)
							}

							claudeInfo.LastToolCallIds[fcIdx] = toolId
							claudeInfo.LastToolCallNames[fcIdx] = toolName
							claudeInfo.PendingToolCalls[fcIdx] = &PendingToolCall{
								ID:   toolId,
								Name: toolName,
							}
							common.SysLog(fmt.Sprintf("[Claude SSE] tool_use new_tool: fcIdx=%d id=%s name=%s", fcIdx, toolId, toolName))
						}

						// 处理 Input（参数）
						argsStr := ""
						if claudeResponse.ContentBlock.Input != nil {
							if b, err := json.Marshal(claudeResponse.ContentBlock.Input); err == nil {
								argsStr = string(b)
							}
							argsNotEmpty := argsStr != "" && argsStr != "{}"
							if argsNotEmpty {
								if claudeInfo.SentFullToolInput == nil {
									claudeInfo.SentFullToolInput = make(map[int]bool)
								}
								claudeInfo.SentFullToolInput[fcIdx] = true
							}
							if argsStr == "{}" {
								argsStr = ""
							}
						}

						// 更新 buffer（只有新值非空才覆盖）
						if argsStr != "" {
							claudeInfo.ToolArgsBuffer[fcIdx] = argsStr
							claudeInfo.PendingToolCalls[fcIdx].Arguments = argsStr
						}
					}
					// 方案D：不在此处发 chunk，等 message_delta 时一次性 flush 所有 pending tools
				}
			} else {
				return nil
			}
		} else if claudeResponse.Type == "content_block_delta" {
			// 方案D调试日志
			common.SysLog(fmt.Sprintf("[Claude SSE] event=content_block_delta index=%v delta_type=%s", claudeResponse.Index, claudeResponse.Delta.Type))

			if claudeResponse.Delta != nil {
				choice.Delta.Content = claudeResponse.Delta.Text
				// 部分上游（如 Bedrock 流式）在 content_block_start 中不传 name，而在 delta 中传；补全 LastToolCallNames 和 PendingToolCalls
				deltaName := claudeResponse.Delta.Name
				if deltaName == "" && claudeResponse.ContentBlock != nil {
					deltaName = claudeResponse.ContentBlock.Name
				}
				if deltaName != "" && claudeInfo != nil {
					fcIdxForName := fcIdx
					if claudeResponse.Index == nil {
						fcIdxForName = claudeInfo.CurrentToolIndex
					}
					common.SysLog(fmt.Sprintf("[Claude SSE] content_block_delta updating name: fcIdxForName=%d name=%s", fcIdxForName, deltaName))
					if claudeInfo.LastToolCallNames != nil {
						claudeInfo.LastToolCallNames[fcIdxForName] = deltaName
					}
					// 方案D：同步更新 PendingToolCalls 中的 name
					if claudeInfo.PendingToolCalls != nil && claudeInfo.PendingToolCalls[fcIdxForName] != nil {
						claudeInfo.PendingToolCalls[fcIdxForName].Name = deltaName
					}
				}
				switch claudeResponse.Delta.Type {
				case "input_json_delta":
					// 始终用 CurrentToolIndex：上游的 Index 是 content_block 序号（0 可能是 text），不是“第几个 tool”，否则 arguments 会错写到 index=0 导致 args_len=0
					if claudeInfo != nil {
						fcIdx = claudeInfo.CurrentToolIndex
					}
					// 若该 tool call 已在 content_block_start 中发送过完整 Arguments，则不再追加 buffer
					if claudeInfo != nil && claudeInfo.SentFullToolInput != nil && claudeInfo.SentFullToolInput[fcIdx] {
						break
					}
					// 只追加到 buffer，不在此处发 chunk；等 message_delta 时一次性 flush
					if claudeInfo != nil && claudeInfo.ToolArgsBuffer != nil && claudeResponse.Delta.PartialJson != nil {
						claudeInfo.ToolArgsBuffer[fcIdx] += *claudeResponse.Delta.PartialJson
					}
				case "signature_delta":
					signatureContent := "\n"
					choice.Delta.ReasoningContent = &signatureContent
				case "thinking_delta":
					thinkingContent := claudeResponse.Delta.Thinking
					choice.Delta.ReasoningContent = &thinkingContent
				}
			}
		} else if claudeResponse.Type == "content_block_stop" {
			// 方案D调试日志：content_block_stop 表示一个 content block 结束
			common.SysLog(fmt.Sprintf("[Claude SSE] event=content_block_stop index=%v", claudeResponse.Index))
			// 不在此处 flush，等 message_delta 统一处理
		} else if claudeResponse.Type == "message_delta" {
			// 方案D调试日志
			common.SysLog(fmt.Sprintf("[Claude SSE] event=message_delta stop_reason=%v", claudeResponse.Delta.StopReason))

			// ========== 方案D：一次性 flush 所有 pending tools ==========
			if claudeInfo != nil && claudeInfo.PendingToolCalls != nil {
				for idx := 0; idx < claudeInfo.ToolCallCount; idx++ {
					pending := claudeInfo.PendingToolCalls[idx]
					if pending == nil || pending.Flushed {
						continue
					}

					// 从 ToolArgsBuffer 获取最终的 arguments（可能在 delta 中累积）
					finalArgs := claudeInfo.ToolArgsBuffer[idx]
					// 从 LastToolCallNames 获取最终的 name（可能在 delta 中更新）
					finalName := claudeInfo.LastToolCallNames[idx]
					if finalName == "" {
						finalName = pending.Name
					}

					// 如果 name 为空，记录警告日志（Cursor 会报 Tool not found）
					if finalName == "" {
						common.SysLog(fmt.Sprintf("[Claude SSE] WARNING: flushing tool with empty name at message_delta, tool_call_id=%s, idx=%d", pending.ID, idx))
					}

					common.SysLog(fmt.Sprintf("[Claude SSE] flushing tool: idx=%d id=%s name=%s args_len=%d", idx, pending.ID, finalName, len(finalArgs)))

					tools = append(tools, dto.ToolCallResponse{
						Index: common.GetPointer(idx),
						ID:    pending.ID,
						Type:  "function",
						Function: dto.FunctionResponse{
							Name:      finalName,
							Arguments: finalArgs,
						},
					})
					pending.Flushed = true
				}
			}

			finishReason := stopReasonClaude2OpenAI(*claudeResponse.Delta.StopReason)
			if finishReason != "null" {
				choice.FinishReason = &finishReason
			}
			//claudeUsage = &claudeResponse.Usage
		} else if claudeResponse.Type == "message_stop" {
			return nil
		} else {
			return nil
		}
	}
	if len(tools) > 0 {
		choice.Delta.Content = nil // compatible with other OpenAI derivative applications, like LobeOpenAICompatibleFactory ...
		choice.Delta.ToolCalls = tools
	}
	response.Choices = append(response.Choices, choice)

	return &response
}

func ResponseClaude2OpenAI(reqMode int, claudeResponse *dto.ClaudeResponse) *dto.OpenAITextResponse {
	choices := make([]dto.OpenAITextResponseChoice, 0)
	fullTextResponse := dto.OpenAITextResponse{
		Id:      fmt.Sprintf("chatcmpl-%s", common.GetUUID()),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
	}
	var responseText string
	var responseThinking string
	if len(claudeResponse.Content) > 0 {
		responseText = claudeResponse.Content[0].GetText()
		responseThinking = claudeResponse.Content[0].Thinking
	}
	tools := make([]dto.ToolCallResponse, 0)
	thinkingContent := ""

	if reqMode == RequestModeCompletion {
		choice := dto.OpenAITextResponseChoice{
			Index: 0,
			Message: dto.Message{
				Role:    "assistant",
				Content: strings.TrimPrefix(claudeResponse.Completion, " "),
				Name:    nil,
			},
			FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
		}
		choices = append(choices, choice)
	} else {
		fullTextResponse.Id = claudeResponse.Id
		for _, message := range claudeResponse.Content {
			switch message.Type {
			case "tool_use":
				args, _ := json.Marshal(message.Input)
				tools = append(tools, dto.ToolCallResponse{
					ID:   message.Id,
					Type: "function", // compatible with other OpenAI derivative applications
					Function: dto.FunctionResponse{
						Name:      message.Name,
						Arguments: string(args),
					},
				})
			case "thinking":
				// 加密的不管， 只输出明文的推理过程
				thinkingContent = message.Thinking
			case "text":
				responseText = message.GetText()
			}
		}
	}
	choice := dto.OpenAITextResponseChoice{
		Index: 0,
		Message: dto.Message{
			Role: "assistant",
		},
		FinishReason: stopReasonClaude2OpenAI(claudeResponse.StopReason),
	}
	choice.SetStringContent(responseText)
	if len(responseThinking) > 0 {
		choice.ReasoningContent = responseThinking
	}
	if len(tools) > 0 {
		choice.Message.SetToolCalls(tools)
	}
	choice.Message.ReasoningContent = thinkingContent
	fullTextResponse.Model = claudeResponse.Model
	choices = append(choices, choice)
	fullTextResponse.Choices = choices
	return &fullTextResponse
}

// PendingToolCall 用于缓冲单个工具调用的完整信息，直到 message_delta 时一次性 flush
type PendingToolCall struct {
	ID        string // tool_call id (如 toolu_xxx)
	Name      string // 工具名 (如 LS, Read, Write)
	Arguments string // 已累积的 arguments JSON
	Flushed   bool   // 是否已 flush 给客户端
}

type ClaudeResponseInfo struct {
	ResponseId        string
	Created           int64
	Model             string
	ResponseText      strings.Builder
	Usage             *dto.Usage
	Done              bool
	LastToolCallIds   map[int]string           // index -> tool_call id
	LastToolCallNames map[int]string           // index -> tool_call name，flush 时拼完整 chunk 用
	ToolArgsBuffer    map[int]string           // index -> 已拼接的 arguments，每个 tool 只发一条完整 chunk 避免 Cursor 收到多条同 id 报 400 tool_use ids must be unique
	SentFullToolInput map[int]bool             // index -> 是否已在 content_block_start 中发送完整 Arguments，若已发送则不再转发 input_json_delta，避免 Cursor 拼出无效 JSON
	CurrentToolIndex  int                      // 当前正在流式输出的 tool_use 的 index；上游 content_block_delta 常不带 index，用此值避免所有 delta 被错误归到 0 导致 Write 等工具参数错乱/无效 JSON
	ToolCallCount     int                      // 本消息内已出现的 tool_use 个数；发给 Cursor 的 index 必须用“第几个 tool”的 0-based 序号，不能用上游的 content_block index（含 text 块会导致首 tool 变成 1 从而 Write 收空）
	PendingToolCalls  map[int]*PendingToolCall // 方案D：缓冲所有工具调用，只在 message_delta 时一次性 flush
}

func FormatClaudeResponseInfo(requestMode int, claudeResponse *dto.ClaudeResponse, oaiResponse *dto.ChatCompletionsStreamResponse, claudeInfo *ClaudeResponseInfo) bool {
	if requestMode == RequestModeCompletion {
		claudeInfo.ResponseText.WriteString(claudeResponse.Completion)
	} else {
		if claudeResponse.Type == "message_start" {
			claudeInfo.ResponseId = claudeResponse.Message.Id
			claudeInfo.Model = claudeResponse.Message.Model
			claudeInfo.ToolCallCount = 0
			claudeInfo.ToolArgsBuffer = make(map[int]string)
			claudeInfo.LastToolCallNames = make(map[int]string)

			// message_start, 获取usage
			claudeInfo.Usage.PromptTokens = claudeResponse.Message.Usage.InputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Message.Usage.CacheReadInputTokens
			claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Message.Usage.CacheCreationInputTokens
			claudeInfo.Usage.CompletionTokens = claudeResponse.Message.Usage.OutputTokens
		} else if claudeResponse.Type == "content_block_delta" {
			if claudeResponse.Delta.Text != nil {
				claudeInfo.ResponseText.WriteString(*claudeResponse.Delta.Text)
			}
			if claudeResponse.Delta.Thinking != "" {
				claudeInfo.ResponseText.WriteString(claudeResponse.Delta.Thinking)
			}
		} else if claudeResponse.Type == "message_delta" {
			// 最终的usage获取：以 message_delta 中的 usage 为准（上游可能在 message_start 只给预估值，最终缓存在 message_delta 中）
			if claudeResponse.Usage != nil {
				if claudeResponse.Usage.InputTokens > 0 {
					claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
				}
				claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
				claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
				// 从 message_delta 更新缓存 token 数（以流结束时的 usage 为准，避免整轮对话始终用 message_start 的固定值导致计费偏高）
				claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
				claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
			} else {
				// 上游在 message_delta 中未带 usage（如 tool_use 结束时部分代理只给 message_start 的预估值），计费会沿用 message_start 的 cache 值，可能偏高
				common.SysLog(fmt.Sprintf("[Claude usage] message_delta usage is nil, keeping message_start cache: cached=%d cached_creation=%d", claudeInfo.Usage.PromptTokensDetails.CachedTokens, claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens))
			}

			// 判断是否完整
			claudeInfo.Done = true
		} else if claudeResponse.Type == "message_stop" {
			// 部分上游在 message_stop 中带最终 usage，若存在则覆盖（避免仅 message_start 有 usage 导致 cache 不更新）
			if claudeResponse.Usage != nil {
				if claudeResponse.Usage.InputTokens > 0 {
					claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
				}
				claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
				claudeInfo.Usage.TotalTokens = claudeInfo.Usage.PromptTokens + claudeInfo.Usage.CompletionTokens
				claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
				claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
				common.SysLog(fmt.Sprintf("[Claude usage] message_stop usage updated: cached=%d cached_creation=%d", claudeResponse.Usage.CacheReadInputTokens, claudeResponse.Usage.CacheCreationInputTokens))
			}
			claudeInfo.Done = true
		} else if claudeResponse.Type == "content_block_start" {
			// 方案D：tool_use 的 index/计数只由 StreamResponseClaude2OpenAI 维护，此处不再更新 ToolCallCount/CurrentToolIndex/LastToolCallIds，避免首工具被错配到 index=1、arguments 错写到 index=0
		} else {
			return false
		}
	}
	if oaiResponse != nil {
		oaiResponse.Id = claudeInfo.ResponseId
		oaiResponse.Created = claudeInfo.Created
		oaiResponse.Model = claudeInfo.Model
	}
	return true
}

// mapClaudeErrorToStatusCode 根据 Claude 错误类型映射 HTTP 状态码
func mapClaudeErrorToStatusCode(claudeError *types.ClaudeError) int {
	if claudeError == nil {
		return http.StatusInternalServerError
	}

	// 根据 Claude 错误类型映射状态码
	switch claudeError.Type {
	case "invalid_request_error":
		return http.StatusBadRequest // 400
	case "authentication_error":
		return http.StatusUnauthorized // 401
	case "permission_error":
		return http.StatusForbidden // 403
	case "not_found_error":
		return http.StatusNotFound // 404
	case "request_too_large":
		return http.StatusRequestEntityTooLarge // 413
	case "rate_limit_error":
		return http.StatusTooManyRequests // 429
	case "api_error", "overloaded_error":
		return http.StatusServiceUnavailable // 503
	default:
		// 默认返回 500，保持向后兼容
		return http.StatusInternalServerError
	}
}

func HandleStreamResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, data string, requestMode int) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.UnmarshalJsonStr(data, &claudeResponse)
	if err != nil {
		logger.LogError(c, "claude stream unmarshal failed: "+err.Error())
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		statusCode := mapClaudeErrorToStatusCode(claudeError)
		logger.LogError(c, fmt.Sprintf("claude stream upstream error: type=%s message=%s", claudeError.Type, claudeError.Message))
		return types.WithClaudeError(*claudeError, statusCode)
	}
	if info.RelayFormat == types.RelayFormatClaude {
		FormatClaudeResponseInfo(requestMode, &claudeResponse, nil, claudeInfo)

		if requestMode == RequestModeCompletion {
		} else {
			if claudeResponse.Type == "message_start" {
				// message_start, 获取usage
				info.UpstreamModelName = claudeResponse.Message.Model
			} else if claudeResponse.Type == "content_block_delta" {
			} else if claudeResponse.Type == "message_delta" {
			}
		}
		helper.ClaudeChunkData(c, claudeResponse, data)
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if !FormatClaudeResponseInfo(requestMode, &claudeResponse, nil, claudeInfo) {
			return nil
		}
		response := StreamResponseClaude2OpenAI(requestMode, &claudeResponse, claudeInfo)
		if response != nil {
			response.Id = claudeInfo.ResponseId
		}

		if response != nil && len(response.Choices) > 0 && response.Choices[0].Delta.ToolCalls != nil && len(response.Choices[0].Delta.ToolCalls) > 0 {
			logger.LogDebug(c, fmt.Sprintf("[Claude stream] chunk has tool_calls: count=%d", len(response.Choices[0].Delta.ToolCalls)))
		}
		if response != nil && len(response.Choices) > 0 && response.Choices[0].FinishReason != nil && *response.Choices[0].FinishReason == "tool_calls" {
			logger.LogInfo(c, "[Claude stream] chunk finish_reason=tool_calls")
		}

		err = helper.ObjectData(c, response)
		if err != nil {
			logger.LogError(c, "send_stream_response_failed: "+err.Error())
		}
	}
	return nil
}

func HandleStreamFinalResponse(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, requestMode int) {

	if requestMode == RequestModeCompletion {
		claudeInfo.Usage = service.ResponseText2Usage(claudeInfo.ResponseText.String(), info.UpstreamModelName, info.PromptTokens)
	} else {
		if claudeInfo.Usage.PromptTokens == 0 {
			//上游出错
		}
		if claudeInfo.Usage.CompletionTokens == 0 || !claudeInfo.Done {
			if common.DebugEnabled {
				common.SysLog("claude response usage is not complete, maybe upstream error")
			}
			claudeInfo.Usage = service.ResponseText2Usage(claudeInfo.ResponseText.String(), info.UpstreamModelName, claudeInfo.Usage.PromptTokens)
		}
	}

	if info.RelayFormat == types.RelayFormatClaude {
		//
	} else if info.RelayFormat == types.RelayFormatOpenAI {
		if info.ShouldIncludeUsage {
			response := helper.GenerateFinalUsageResponse(claudeInfo.ResponseId, claudeInfo.Created, info.UpstreamModelName, *claudeInfo.Usage)
			err := helper.ObjectData(c, response)
			if err != nil {
				logger.LogError(c, "claude stream send final usage failed: "+err.Error())
			}
		}
		logger.LogInfo(c, "claude stream sending [DONE]")
		helper.Done(c)
	}
}

func ClaudeStreamHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, requestMode int) (*dto.Usage, *types.NewAPIError) {
	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	var err *types.NewAPIError
	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		err = HandleStreamResponseData(c, info, claudeInfo, data, requestMode)
		if err != nil {
			logger.LogError(c, "claude stream stopped by dataHandler: "+err.Error())
			return false
		}
		return true
	})
	if err != nil {
		logger.LogError(c, "claude stream handler exit with error: "+err.Error())
		return nil, err
	}

	HandleStreamFinalResponse(c, info, claudeInfo, requestMode)
	return claudeInfo.Usage, nil
}

func HandleClaudeResponseData(c *gin.Context, info *relaycommon.RelayInfo, claudeInfo *ClaudeResponseInfo, httpResp *http.Response, data []byte, requestMode int) *types.NewAPIError {
	var claudeResponse dto.ClaudeResponse
	err := common.Unmarshal(data, &claudeResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if claudeError := claudeResponse.GetClaudeError(); claudeError != nil && claudeError.Type != "" {
		statusCode := mapClaudeErrorToStatusCode(claudeError)
		return types.WithClaudeError(*claudeError, statusCode)
	}
	if requestMode == RequestModeCompletion {
		completionTokens := service.CountTextToken(claudeResponse.Completion, info.OriginModelName)
		claudeInfo.Usage.PromptTokens = info.PromptTokens
		claudeInfo.Usage.CompletionTokens = completionTokens
		claudeInfo.Usage.TotalTokens = info.PromptTokens + completionTokens
	} else {
		claudeInfo.Usage.PromptTokens = claudeResponse.Usage.InputTokens
		claudeInfo.Usage.CompletionTokens = claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.TotalTokens = claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens
		claudeInfo.Usage.PromptTokensDetails.CachedTokens = claudeResponse.Usage.CacheReadInputTokens
		claudeInfo.Usage.PromptTokensDetails.CachedCreationTokens = claudeResponse.Usage.CacheCreationInputTokens
	}
	var responseData []byte
	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		openaiResponse := ResponseClaude2OpenAI(requestMode, &claudeResponse)
		openaiResponse.Usage = *claudeInfo.Usage
		responseData, err = json.Marshal(openaiResponse)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		responseData = data
	case types.RelayFormatGemini:
		// Claude -> OpenAI -> Gemini
		openaiResponse := ResponseClaude2OpenAI(requestMode, &claudeResponse)
		openaiResponse.Usage = *claudeInfo.Usage
		geminiResp := service.ResponseOpenAI2Gemini(openaiResponse, info)
		responseData, err = json.Marshal(geminiResp)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	}

	if claudeResponse.Usage.ServerToolUse != nil && claudeResponse.Usage.ServerToolUse.WebSearchRequests > 0 {
		c.Set("claude_web_search_requests", claudeResponse.Usage.ServerToolUse.WebSearchRequests)
	}

	service.IOCopyBytesGracefully(c, httpResp, responseData)
	return nil
}

func ClaudeHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, requestMode int) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	claudeInfo := &ClaudeResponseInfo{
		ResponseId:   helper.GetResponseID(c),
		Created:      common.GetTimestamp(),
		Model:        info.UpstreamModelName,
		ResponseText: strings.Builder{},
		Usage:        &dto.Usage{},
	}
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if common.DebugEnabled {
		println("responseBody: ", string(responseBody))
	}
	handleErr := HandleClaudeResponseData(c, info, claudeInfo, resp, responseBody, requestMode)
	if handleErr != nil {
		return nil, handleErr
	}
	return claudeInfo.Usage, nil
}

func mapToolChoice(toolChoice any, parallelToolCalls *bool) *dto.ClaudeToolChoice {
	var claudeToolChoice *dto.ClaudeToolChoice

	// 处理 tool_choice 字符串值
	if toolChoiceStr, ok := toolChoice.(string); ok {
		switch toolChoiceStr {
		case "auto":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		case "required":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "any",
			}
		case "none":
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "none",
			}
		}
	} else if toolChoiceMap, ok := toolChoice.(map[string]interface{}); ok {
		// 处理 tool_choice 对象值
		if function, ok := toolChoiceMap["function"].(map[string]interface{}); ok {
			if toolName, ok := function["name"].(string); ok {
				claudeToolChoice = &dto.ClaudeToolChoice{
					Type: "tool",
					Name: toolName,
				}
			}
		}
	}

	// 处理 parallel_tool_calls
	if parallelToolCalls != nil {
		if claudeToolChoice == nil {
			// 如果没有 tool_choice，但有 parallel_tool_calls，创建默认的 auto 类型
			claudeToolChoice = &dto.ClaudeToolChoice{
				Type: "auto",
			}
		}

		// 设置 disable_parallel_tool_use
		// 如果 parallel_tool_calls 为 true，则 disable_parallel_tool_use 为 false
		claudeToolChoice.DisableParallelToolUse = !*parallelToolCalls
	}

	return claudeToolChoice
}
