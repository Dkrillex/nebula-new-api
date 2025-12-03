package aws

import (
	"one-api/common"
	"one-api/dto"
)

type AwsClaudeRequest struct {
	// AnthropicVersion should be "bedrock-2023-05-31"
	AnthropicVersion string              `json:"anthropic_version"`
	System           any                 `json:"system,omitempty"`
	Messages         []dto.ClaudeMessage `json:"messages"`
	MaxTokens        uint                `json:"max_tokens,omitempty"`
	Temperature      *float64            `json:"temperature,omitempty"`
	TopP             float64             `json:"top_p,omitempty"`
	TopK             int                 `json:"top_k,omitempty"`
	StopSequences    []string            `json:"stop_sequences,omitempty"`
	Tools            any                 `json:"tools,omitempty"`
	ToolChoice       any                 `json:"tool_choice,omitempty"`
	Thinking         *dto.Thinking       `json:"thinking,omitempty"`
}

func copyRequest(req *dto.ClaudeRequest) *AwsClaudeRequest {
	// 使用通用白名单过滤函数，只保留 AWS Bedrock 支持的字段
	filteredReq := filterAwsBedrockSupportedFields(req)

	return &AwsClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		System:           filteredReq.System,
		Messages:         filteredReq.Messages,
		MaxTokens:        filteredReq.MaxTokens,
		Temperature:      filteredReq.Temperature,
		TopP:             filteredReq.TopP,
		TopK:             filteredReq.TopK,
		StopSequences:    filteredReq.StopSequences,
		Tools:            filteredReq.Tools,
		ToolChoice:       filteredReq.ToolChoice,
		Thinking:         filteredReq.Thinking,
	}
}

// filterAwsBedrockSupportedFields 使用白名单机制过滤，只保留 AWS Bedrock 支持的字段
func filterAwsBedrockSupportedFields(req *dto.ClaudeRequest) *dto.ClaudeRequest {
	// 通过 JSON 序列化/反序列化来处理
	jsonBytes, err := common.Marshal(req)
	if err != nil {
		return req
	}

	var reqMap map[string]any
	if err := common.Unmarshal(jsonBytes, &reqMap); err != nil {
		return req
	}

	// 定义 AWS Bedrock 支持的白名单字段
	allowedFields := map[string]bool{
		"model":                false, // 不需要，会在其他地方处理
		"system":               true,
		"messages":             true,
		"max_tokens":           true,
		"temperature":          true,
		"top_p":                true,
		"top_k":                true,
		"stop_sequences":       true,
		"tools":                true,
		"tool_choice":          true,
		"thinking":             true,
		"anthropic_version":    false, // 会在 AwsClaudeRequest 中设置
		"stream":               false, // AWS Bedrock 通过不同的 API 处理流式
		"prompt":               false, // 旧版 API，不支持
		"max_tokens_to_sample": false, // 旧版 API，不支持
		"context_management":   false, // 不支持
		"mcp_servers":          false, // 不支持
		"metadata":             false, // 不支持
		"service_tier":         false, // 不支持
	}

	// 过滤顶层字段
	filteredMap := make(map[string]any)
	for k, v := range reqMap {
		if allowedFields[k] {
			// 递归过滤嵌套结构
			filteredMap[k] = filterFieldByWhitelist(k, v)
		}
	}

	// 转换回 dto.ClaudeRequest
	var filteredReq dto.ClaudeRequest
	filteredBytes, err := common.Marshal(filteredMap)
	if err != nil {
		return req
	}
	if err := common.Unmarshal(filteredBytes, &filteredReq); err != nil {
		return req
	}

	return &filteredReq
}

// filterFieldByWhitelist 根据字段类型和路径进行白名单过滤
func filterFieldByWhitelist(fieldPath string, value any) any {
	if value == nil {
		return nil
	}

	switch fieldPath {
	case "system":
		return filterSystemField(value)
	case "messages":
		return filterMessagesField(value)
	case "tools":
		return filterToolsField(value)
	case "tool_choice":
		return filterToolChoiceField(value)
	case "thinking":
		return filterThinkingField(value)
	case "stop_sequences":
		return filterStopSequencesField(value)
	default:
		// 对于简单字段（max_tokens, temperature, top_p, top_k），直接返回
		return value
	}
}

// filterSystemField 过滤 system 字段，移除 cache_control.ttl
func filterSystemField(system any) any {
	return filterCacheControlTTLRecursive(system)
}

// filterMessagesField 过滤 messages 字段，移除 cache_control.ttl
func filterMessagesField(messages any) any {
	if messages == nil {
		return nil
	}

	messagesSlice, ok := messages.([]any)
	if !ok {
		return filterCacheControlTTLRecursive(messages)
	}

	filtered := make([]any, 0, len(messagesSlice))
	for _, msg := range messagesSlice {
		filtered = append(filtered, filterCacheControlTTLRecursive(msg))
	}
	return filtered
}

// filterToolsField 过滤 tools 字段，移除 input_examples
func filterToolsField(tools any) any {
	if tools == nil {
		return nil
	}

	toolsSlice, ok := tools.([]any)
	if !ok {
		return filterToolInputExamples(tools)
	}

	filtered := make([]any, 0, len(toolsSlice))
	for _, tool := range toolsSlice {
		filteredTool := filterToolInputExamples(tool)
		if filteredTool != nil {
			filtered = append(filtered, filteredTool)
		}
	}
	return filtered
}

// filterToolInputExamples 从单个 tool 中移除 input_examples
func filterToolInputExamples(tool any) any {
	if tool == nil {
		return nil
	}

	toolMap, ok := tool.(map[string]any)
	if !ok {
		// 通过 JSON 处理
		jsonBytes, err := common.Marshal(tool)
		if err != nil {
			return tool
		}
		var toolMap map[string]any
		if err := common.Unmarshal(jsonBytes, &toolMap); err != nil {
			return tool
		}
		return filterToolInputExamples(toolMap)
	}

	filtered := make(map[string]any)
	for k, v := range toolMap {
		if k == "input_examples" {
			// 跳过 input_examples 字段
			continue
		}
		if k == "input_schema" {
			// 递归过滤 input_schema 中的 input_examples
			if schemaMap, ok := v.(map[string]any); ok {
				filteredSchema := make(map[string]any)
				for sk, sv := range schemaMap {
					if sk != "input_examples" {
						filteredSchema[sk] = sv
					}
				}
				filtered[k] = filteredSchema
			} else {
				filtered[k] = v
			}
		} else {
			filtered[k] = v
		}
	}
	return filtered
}

// filterToolChoiceField 过滤 tool_choice 字段，只保留支持的字段
func filterToolChoiceField(toolChoice any) any {
	if toolChoice == nil {
		return nil
	}

	// 如果是字符串，直接返回
	if _, ok := toolChoice.(string); ok {
		return toolChoice
	}

	// 如果是对象，只保留支持的字段
	toolChoiceMap, ok := toolChoice.(map[string]any)
	if !ok {
		return toolChoice
	}

	allowedToolChoiceFields := map[string]bool{
		"type":                      true,
		"name":                      true,
		"disable_parallel_tool_use": true,
	}

	filtered := make(map[string]any)
	for k, v := range toolChoiceMap {
		if allowedToolChoiceFields[k] {
			filtered[k] = v
		}
	}

	return filtered
}

// filterThinkingField 过滤 thinking 字段，只保留支持的字段
func filterThinkingField(thinking any) any {
	if thinking == nil {
		return nil
	}

	thinkingMap, ok := thinking.(map[string]any)
	if !ok {
		return thinking
	}

	allowedThinkingFields := map[string]bool{
		"type":          true,
		"budget_tokens": true,
	}

	filtered := make(map[string]any)
	for k, v := range thinkingMap {
		if allowedThinkingFields[k] {
			filtered[k] = v
		}
	}

	return filtered
}

// filterStopSequencesField 过滤 stop_sequences 字段
func filterStopSequencesField(stopSequences any) any {
	// stop_sequences 应该是字符串数组，直接返回
	return stopSequences
}

// filterInputExamplesFromTools 从 tools 中过滤掉 input_examples 字段（AWS Bedrock 不支持）
func filterInputExamplesFromTools(tools any) any {
	if tools == nil {
		return nil
	}

	// 尝试转换为 []any
	toolsSlice, ok := tools.([]any)
	if !ok {
		// 尝试转换为 []*dto.Tool 或 []dto.Tool
		if _, ok := tools.([]*dto.Tool); ok {
			// dto.Tool 结构体本身不包含 input_examples，但为了安全起见，还是通过 JSON 处理
			// 因为可能有其他未知字段
			return filterInputExamplesFromToolsJSON(tools)
		}
		if _, ok := tools.([]dto.Tool); ok {
			return filterInputExamplesFromToolsJSON(tools)
		}
		// 如果是其他类型，尝试通过 JSON 序列化/反序列化来处理
		return filterInputExamplesFromToolsJSON(tools)
	}

	// 处理 []any 类型
	filtered := make([]any, 0, len(toolsSlice))
	for _, tool := range toolsSlice {
		filteredTool := filterInputExamplesFromTool(tool)
		if filteredTool != nil {
			filtered = append(filtered, filteredTool)
		}
	}
	return filtered
}

// filterInputExamplesFromTool 从单个 tool 中过滤掉 input_examples 字段
func filterInputExamplesFromTool(tool any) any {
	if tool == nil {
		return nil
	}

	// 尝试转换为 map[string]any
	toolMap, ok := tool.(map[string]any)
	if !ok {
		// 如果不是 map，尝试通过 JSON 处理
		return filterInputExamplesFromToolJSON(tool)
	}

	// 创建新的 map，排除 input_examples
	filtered := make(map[string]any)
	for k, v := range toolMap {
		if k == "input_examples" {
			// 跳过 input_examples 字段
			continue
		}
		// 如果 value 是 input_schema，也需要检查其中是否有 input_examples
		if k == "input_schema" {
			if schemaMap, ok := v.(map[string]any); ok {
				filteredSchema := make(map[string]any)
				for sk, sv := range schemaMap {
					if sk != "input_examples" {
						filteredSchema[sk] = sv
					}
				}
				filtered[k] = filteredSchema
			} else {
				filtered[k] = v
			}
		} else {
			filtered[k] = v
		}
	}
	return filtered
}

// filterInputExamplesFromToolJSON 通过 JSON 序列化/反序列化来过滤 input_examples
func filterInputExamplesFromToolJSON(tool any) any {
	jsonBytes, err := common.Marshal(tool)
	if err != nil {
		return tool
	}

	var toolMap map[string]any
	if err := common.Unmarshal(jsonBytes, &toolMap); err != nil {
		return tool
	}

	return filterInputExamplesFromTool(toolMap)
}

// filterInputExamplesFromToolsJSON 通过 JSON 序列化/反序列化来过滤 tools 中的 input_examples
func filterInputExamplesFromToolsJSON(tools any) any {
	jsonBytes, err := common.Marshal(tools)
	if err != nil {
		return tools
	}

	// 先尝试解析为 []any
	var toolsSlice []any
	if err := common.Unmarshal(jsonBytes, &toolsSlice); err == nil {
		return filterInputExamplesFromTools(toolsSlice)
	}

	// 如果失败，尝试解析为 []map[string]any
	var toolsMapSlice []map[string]any
	if err := common.Unmarshal(jsonBytes, &toolsMapSlice); err == nil {
		filtered := make([]any, 0, len(toolsMapSlice))
		for _, toolMap := range toolsMapSlice {
			filteredTool := filterInputExamplesFromTool(toolMap)
			if filteredTool != nil {
				filtered = append(filtered, filteredTool)
			}
		}
		return filtered
	}

	// 如果都失败，返回原值
	return tools
}

// NovaMessage Nova模型使用messages-v1格式
type NovaMessage struct {
	Role    string        `json:"role"`
	Content []NovaContent `json:"content"`
}

type NovaContent struct {
	Text string `json:"text"`
}

type NovaRequest struct {
	SchemaVersion   string               `json:"schemaVersion"`             // 请求版本，例如 "1.0"
	Messages        []NovaMessage        `json:"messages"`                  // 对话消息列表
	InferenceConfig *NovaInferenceConfig `json:"inferenceConfig,omitempty"` // 推理配置，可选
}

type NovaInferenceConfig struct {
	MaxTokens     int      `json:"maxTokens,omitempty"`     // 最大生成的 token 数
	Temperature   float64  `json:"temperature,omitempty"`   // 随机性 (默认 0.7, 范围 0-1)
	TopP          float64  `json:"topP,omitempty"`          // nucleus sampling (默认 0.9, 范围 0-1)
	TopK          int      `json:"topK,omitempty"`          // 限制候选 token 数 (默认 50, 范围 0-128)
	StopSequences []string `json:"stopSequences,omitempty"` // 停止生成的序列
}

// 转换OpenAI请求为Nova格式
func convertToNovaRequest(req *dto.GeneralOpenAIRequest) *NovaRequest {
	novaMessages := make([]NovaMessage, len(req.Messages))
	for i, msg := range req.Messages {
		novaMessages[i] = NovaMessage{
			Role:    msg.Role,
			Content: []NovaContent{{Text: msg.StringContent()}},
		}
	}

	novaReq := &NovaRequest{
		SchemaVersion: "messages-v1",
		Messages:      novaMessages,
	}

	// 设置推理配置
	if req.MaxTokens != 0 || (req.Temperature != nil && *req.Temperature != 0) || req.TopP != 0 || req.TopK != 0 || req.Stop != nil {
		novaReq.InferenceConfig = &NovaInferenceConfig{}
		if req.MaxTokens != 0 {
			novaReq.InferenceConfig.MaxTokens = int(req.MaxTokens)
		}
		if req.Temperature != nil && *req.Temperature != 0 {
			novaReq.InferenceConfig.Temperature = *req.Temperature
		}
		if req.TopP != 0 {
			novaReq.InferenceConfig.TopP = req.TopP
		}
		if req.TopK != 0 {
			novaReq.InferenceConfig.TopK = req.TopK
		}
		if req.Stop != nil {
			if stopSequences := parseStopSequences(req.Stop); len(stopSequences) > 0 {
				novaReq.InferenceConfig.StopSequences = stopSequences
			}
		}
	}

	return novaReq
}

// parseStopSequences 解析停止序列，支持字符串或字符串数组
func parseStopSequences(stop any) []string {
	if stop == nil {
		return nil
	}

	switch v := stop.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []string:
		return v
	case []interface{}:
		var sequences []string
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				sequences = append(sequences, str)
			}
		}
		return sequences
	}
	return nil
}

// filterCacheControlTTL 从 system 中过滤掉 cache_control.ttl 字段（AWS Bedrock 不支持）
func filterCacheControlTTL(system any) any {
	if system == nil {
		return nil
	}

	// 通过 JSON 序列化/反序列化来处理
	jsonBytes, err := common.Marshal(system)
	if err != nil {
		return system
	}

	var systemData any
	if err := common.Unmarshal(jsonBytes, &systemData); err != nil {
		return system
	}

	// 递归过滤 cache_control.ttl
	return filterCacheControlTTLRecursive(systemData)
}

// filterMessagesCacheControlTTL 从 messages 中过滤掉 cache_control.ttl 字段
func filterMessagesCacheControlTTL(messages []dto.ClaudeMessage) []dto.ClaudeMessage {
	if messages == nil {
		return nil
	}

	// 通过 JSON 序列化/反序列化来处理
	jsonBytes, err := common.Marshal(messages)
	if err != nil {
		return messages
	}

	var messagesData any
	if err := common.Unmarshal(jsonBytes, &messagesData); err != nil {
		return messages
	}

	// 递归过滤 cache_control.ttl
	filteredData := filterCacheControlTTLRecursive(messagesData)

	// 转换回 []dto.ClaudeMessage
	var filteredMessages []dto.ClaudeMessage
	filteredBytes, err := common.Marshal(filteredData)
	if err != nil {
		return messages
	}
	if err := common.Unmarshal(filteredBytes, &filteredMessages); err != nil {
		return messages
	}

	return filteredMessages
}

// filterCacheControlTTLRecursive 递归过滤 cache_control.ttl 字段
func filterCacheControlTTLRecursive(data any) any {
	if data == nil {
		return nil
	}

	switch v := data.(type) {
	case map[string]any:
		filtered := make(map[string]any)
		for k, val := range v {
			if k == "cache_control" {
				// 处理 cache_control 对象
				if cacheControl, ok := val.(map[string]any); ok {
					filteredCacheControl := make(map[string]any)
					for ck, cv := range cacheControl {
						// 跳过 ttl 字段
						if ck != "ttl" {
							filteredCacheControl[ck] = cv
						}
					}
					// 如果 cache_control 还有其他字段，保留它
					if len(filteredCacheControl) > 0 {
						filtered[k] = filteredCacheControl
					}
					// 如果 cache_control 只有 ttl，则完全移除 cache_control
				} else {
					// 如果不是 map，递归处理
					filtered[k] = filterCacheControlTTLRecursive(val)
				}
			} else {
				// 递归处理其他字段
				filtered[k] = filterCacheControlTTLRecursive(val)
			}
		}
		return filtered

	case []any:
		filtered := make([]any, 0, len(v))
		for _, item := range v {
			filtered = append(filtered, filterCacheControlTTLRecursive(item))
		}
		return filtered

	default:
		return data
	}
}
