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
	// 过滤掉 AWS Bedrock 不支持的 input_examples 字段
	var filteredTools any
	if req.Tools != nil {
		// 使用 JSON 方式确保能处理所有类型
		filteredTools = filterInputExamplesFromToolsJSON(req.Tools)
	}

	return &AwsClaudeRequest{
		AnthropicVersion: "bedrock-2023-05-31",
		System:           req.System,
		Messages:         req.Messages,
		MaxTokens:        req.MaxTokens,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		TopK:             req.TopK,
		StopSequences:    req.StopSequences,
		Tools:            filteredTools,
		ToolChoice:       req.ToolChoice,
		Thinking:         req.Thinking,
	}
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
