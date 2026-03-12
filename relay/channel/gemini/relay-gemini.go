package gemini

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/relay/channel/openai"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// https://cloud.google.com/vertex-ai/generative-ai/docs/model-reference/inference?hl=zh-cn#blob
var geminiSupportedMimeTypes = map[string]bool{
	"application/pdf": true,
	"audio/mpeg":      true,
	"audio/mp3":       true,
	"audio/wav":       true,
	"image/png":       true,
	"image/jpeg":      true,
	"image/webp":      true,
	"text/plain":      true,
	"video/mov":       true,
	"video/mpeg":      true,
	"video/mp4":       true,
	"video/mpg":       true,
	"video/avi":       true,
	"video/wmv":       true,
	"video/mpegps":    true,
	"video/flv":       true,
}

// Gemini 允许的思考预算范围
const (
	pro25MinBudget       = 128
	pro25MaxBudget       = 32768
	flash25MaxBudget     = 24576
	flash25LiteMinBudget = 512
	flash25LiteMaxBudget = 24576
)

func isNew25ProModel(modelName string) bool {
	return strings.HasPrefix(modelName, "gemini-2.5-pro") &&
		!strings.HasPrefix(modelName, "gemini-2.5-pro-preview-05-06") &&
		!strings.HasPrefix(modelName, "gemini-2.5-pro-preview-03-25")
}

func is25FlashLiteModel(modelName string) bool {
	return strings.HasPrefix(modelName, "gemini-2.5-flash-lite")
}

func isGemini3ProModel(modelName string) bool {
	// 支持 gemini-3-pro 和 gemini-3.1-pro 系列模型
	return strings.HasPrefix(modelName, "gemini-3-pro") || strings.HasPrefix(modelName, "gemini-3.1-pro")
}

// isGemini31Model 判断是否为 Gemini 3.1 系列模型（包括 Pro 和 Flash-Lite）
// 根据 Google 官方文档，Gemini 3.1 系列模型都支持 thinking_level 参数
func isGemini31Model(modelName string) bool {
	return strings.HasPrefix(modelName, "gemini-3.1-")
}

// clampThinkingBudget 根据模型名称将预算限制在允许的范围内
func clampThinkingBudget(modelName string, budget int) int {
	isNew25Pro := isNew25ProModel(modelName)
	is25FlashLite := is25FlashLiteModel(modelName)

	if is25FlashLite {
		if budget < flash25LiteMinBudget {
			return flash25LiteMinBudget
		}
		if budget > flash25LiteMaxBudget {
			return flash25LiteMaxBudget
		}
	} else if isNew25Pro {
		if budget < pro25MinBudget {
			return pro25MinBudget
		}
		if budget > pro25MaxBudget {
			return pro25MaxBudget
		}
	} else { // 其他模型
		if budget < 0 {
			return 0
		}
		if budget > flash25MaxBudget {
			return flash25MaxBudget
		}
	}
	return budget
}

// "effort": "high" - Allocates a large portion of tokens for reasoning (approximately 80% of max_tokens)
// "effort": "medium" - Allocates a moderate portion of tokens (approximately 50% of max_tokens)
// "effort": "low" - Allocates a smaller portion of tokens (approximately 20% of max_tokens)
func clampThinkingBudgetByEffort(modelName string, effort string) int {
	isNew25Pro := isNew25ProModel(modelName)
	is25FlashLite := is25FlashLiteModel(modelName)

	maxBudget := 0
	if is25FlashLite {
		maxBudget = flash25LiteMaxBudget
	}
	if isNew25Pro {
		maxBudget = pro25MaxBudget
	} else {
		maxBudget = flash25MaxBudget
	}
	switch effort {
	case "high":
		maxBudget = maxBudget * 80 / 100
	case "medium":
		maxBudget = maxBudget * 50 / 100
	case "low":
		maxBudget = maxBudget * 20 / 100
	}
	return clampThinkingBudget(modelName, maxBudget)
}

func ThinkingAdaptor(geminiRequest *dto.GeminiChatRequest, info *relaycommon.RelayInfo, oaiRequest ...dto.GeneralOpenAIRequest) {
	if model_setting.GetGeminiSettings().ThinkingAdapterEnabled {
		modelName := info.UpstreamModelName
		// Gemini 3.1 系列模型（包括 Pro 和 Flash-Lite）支持 thinkingLevel（LOW/HIGH）
		// 根据 Google 官方文档，Gemini 3.1 Flash-Lite 也支持 thinking_level 参数
		if isGemini31Model(modelName) {
			level := "HIGH"
			if strings.HasSuffix(modelName, "-thinking-low") {
				level = "LOW"
			} else if strings.HasSuffix(modelName, "-thinking-high") {
				level = "HIGH"
			} else if len(oaiRequest) > 0 {
				// reasoning_effort 映射到 thinkingLevel
				switch strings.ToLower(oaiRequest[0].ReasoningEffort) {
				case "low", "medium":
					level = "LOW"
				case "high":
					level = "HIGH"
				}
			}

			geminiRequest.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
				IncludeThoughts: true,
				ThinkingLevel:   level,
			}
			return
		}

		isNew25Pro := strings.HasPrefix(modelName, "gemini-2.5-pro") &&
			!strings.HasPrefix(modelName, "gemini-2.5-pro-preview-05-06") &&
			!strings.HasPrefix(modelName, "gemini-2.5-pro-preview-03-25")

		if strings.Contains(modelName, "-thinking-") {
			parts := strings.SplitN(modelName, "-thinking-", 2)
			if len(parts) == 2 && parts[1] != "" {
				if budgetTokens, err := strconv.Atoi(parts[1]); err == nil {
					clampedBudget := clampThinkingBudget(modelName, budgetTokens)
					geminiRequest.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
						ThinkingBudget:  common.GetPointer(clampedBudget),
						IncludeThoughts: true,
					}
				}
			}
		} else if strings.HasSuffix(modelName, "-thinking") {
			unsupportedModels := []string{
				"gemini-2.5-pro-preview-05-06",
				"gemini-2.5-pro-preview-03-25",
			}
			isUnsupported := false
			for _, unsupportedModel := range unsupportedModels {
				if strings.HasPrefix(modelName, unsupportedModel) {
					isUnsupported = true
					break
				}
			}

			if isUnsupported {
				geminiRequest.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
					IncludeThoughts: true,
				}
			} else {
				geminiRequest.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
					IncludeThoughts: true,
				}
				if geminiRequest.GenerationConfig.MaxOutputTokens > 0 {
					budgetTokens := model_setting.GetGeminiSettings().ThinkingAdapterBudgetTokensPercentage * float64(geminiRequest.GenerationConfig.MaxOutputTokens)
					clampedBudget := clampThinkingBudget(modelName, int(budgetTokens))
					geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget = common.GetPointer(clampedBudget)
				} else {
					if len(oaiRequest) > 0 {
						// 如果有reasoningEffort参数，则根据其值设置思考预算
						geminiRequest.GenerationConfig.ThinkingConfig.ThinkingBudget = common.GetPointer(clampThinkingBudgetByEffort(modelName, oaiRequest[0].ReasoningEffort))
					}
				}
			}
		} else if strings.HasSuffix(modelName, "-nothinking") {
			if !isNew25Pro {
				geminiRequest.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
					ThinkingBudget: common.GetPointer(0),
				}
			}
		}
	}
}

// Setting safety to the lowest possible values since Gemini is already powerless enough
func CovertGemini2OpenAI(c *gin.Context, textRequest dto.GeneralOpenAIRequest, info *relaycommon.RelayInfo) (*dto.GeminiChatRequest, error) {

	geminiRequest := dto.GeminiChatRequest{
		Contents: make([]dto.GeminiChatContent, 0, len(textRequest.Messages)),
		GenerationConfig: dto.GeminiChatGenerationConfig{
			Temperature:     textRequest.Temperature,
			TopP:            textRequest.TopP,
			MaxOutputTokens: textRequest.GetMaxTokens(),
			Seed:            int64(textRequest.Seed),
		},
	}

	if model_setting.IsGeminiModelSupportImagine(info.UpstreamModelName) {
		geminiRequest.GenerationConfig.ResponseModalities = []string{
			"TEXT",
			"IMAGE",
		}
	}

	thinkingConfigured := false

	// 检查 enable_thinking 参数（仅在 extra_body 中没有 thinking_config 时处理）
	if textRequest.EnableThinking != nil {
		enableThinking := false
		switch v := textRequest.EnableThinking.(type) {
		case bool:
			enableThinking = v
		case string:
			enableThinking = strings.ToLower(v) == "true" || v == "1"
		}

		// 检查 extra_body 中是否已有 thinking_config，如果有则跳过 enable_thinking 的处理
		hasThinkingConfigInExtraBody := false
		if len(textRequest.ExtraBody) > 0 {
			var extraBody map[string]interface{}
			if err := common.Unmarshal(textRequest.ExtraBody, &extraBody); err == nil {
				if googleBody, ok := extraBody["google"].(map[string]interface{}); ok {
					if _, ok := googleBody["thinking_config"].(map[string]interface{}); ok {
						hasThinkingConfigInExtraBody = true
					}
				}
			}
		}

		// 只有在 extra_body 中没有 thinking_config 时，才处理 enable_thinking
		if enableThinking && !hasThinkingConfigInExtraBody && !strings.HasSuffix(info.UpstreamModelName, "-nothinking") {
			// Gemini 3.1 系列模型（包括 Flash-Lite）支持 thinking_level
			if isGemini31Model(info.UpstreamModelName) {
				level := "HIGH"
				includeThoughts := true // 默认启用思考内容返回
				// 如果请求中有 reasoning_effort，也考虑它
				if textRequest.ReasoningEffort != "" {
					switch strings.ToLower(textRequest.ReasoningEffort) {
					case "low", "medium":
						level = "LOW"
					case "high":
						level = "HIGH"
					}
				}
				geminiRequest.GenerationConfig.ThinkingConfig = &dto.GeminiThinkingConfig{
					IncludeThoughts: includeThoughts,
					ThinkingLevel:   level,
				}
				thinkingConfigured = true
				common.SysLog(fmt.Sprintf("[Gemini] 模型 %s 通过 enable_thinking 启用思考模式，thinkingLevel=%s, includeThoughts=%v", info.UpstreamModelName, level, includeThoughts))
			}
		}
	}

	if len(textRequest.ExtraBody) > 0 {
		var extraBody map[string]interface{}
		if err := common.Unmarshal(textRequest.ExtraBody, &extraBody); err != nil {
			return nil, fmt.Errorf("invalid extra body: %w", err)
		}
		// 直接透传 extra_body.google 中的所有字段到 geminiRequest，不做任何转换、覆盖或修改
		// eg. {"google":{"thinking_config":{"thinking_budget":5324,"include_thoughts":true},"tools":[...]}}
		if googleBody, ok := extraBody["google"].(map[string]interface{}); ok {
			// 处理 thinking_config：需要放在 generationConfig.thinkingConfig 路径下
			if thinkingConfig, ok := googleBody["thinking_config"].(map[string]interface{}); ok {
				// 将 thinking_config 直接序列化为 JSON，然后反序列化为 GeminiThinkingConfig
				// 这样可以利用 GeminiThinkingConfig 的 UnmarshalJSON 方法处理 snake_case 和 camelCase
				thinkingConfigJson, err := common.Marshal(thinkingConfig)
				if err == nil {
					var directThinkingConfig dto.GeminiThinkingConfig
					if err := common.Unmarshal(thinkingConfigJson, &directThinkingConfig); err == nil {
						geminiRequest.GenerationConfig.ThinkingConfig = &directThinkingConfig
						thinkingConfigured = true
						common.SysLog(fmt.Sprintf("[Gemini] 模型 %s 通过 extra_body.google.thinking_config 直接透传配置，不做任何转换", info.UpstreamModelName))
					}
				}
			}

			// 处理其他字段（如 tools），直接合并到 geminiRequest
			if toolsRaw, ok := googleBody["tools"]; ok {
				existingTools := geminiRequest.GetTools()
				if toolsArr, err := common.Any2Type[[]dto.GeminiChatTool](toolsRaw); err == nil {
					existingTools = append(existingTools, toolsArr...)
				} else if toolObj, err := common.Any2Type[dto.GeminiChatTool](toolsRaw); err == nil {
					existingTools = append(existingTools, toolObj)
				}
				if len(existingTools) > 0 {
					geminiRequest.SetTools(existingTools)
				}
			}
		}
	}

	if !thinkingConfigured {
		ThinkingAdaptor(&geminiRequest, info, textRequest)
	}

	safetySettings := make([]dto.GeminiChatSafetySettings, 0, len(SafetySettingList))
	for _, category := range SafetySettingList {
		safetySettings = append(safetySettings, dto.GeminiChatSafetySettings{
			Category:  category,
			Threshold: model_setting.GetGeminiSafetySetting(category),
		})
	}
	geminiRequest.SafetySettings = safetySettings

	// openaiContent.FuncToToolCalls()
	if textRequest.Tools != nil {
		functions := make([]dto.FunctionRequest, 0, len(textRequest.Tools))
		googleSearch := false
		codeExecution := false
		urlContext := false
		for _, tool := range textRequest.Tools {
			if tool.Function.Name == "googleSearch" {
				googleSearch = true
				continue
			}
			if tool.Function.Name == "codeExecution" {
				codeExecution = true
				continue
			}
			if tool.Function.Name == "urlContext" {
				urlContext = true
				continue
			}
			if tool.Function.Parameters != nil {

				params, ok := tool.Function.Parameters.(map[string]interface{})
				if ok {
					if props, hasProps := params["properties"].(map[string]interface{}); hasProps {
						if len(props) == 0 {
							tool.Function.Parameters = nil
						}
					}
				}
			}
			// Clean the parameters before appending
			cleanedParams := cleanFunctionParameters(tool.Function.Parameters)
			tool.Function.Parameters = cleanedParams
			functions = append(functions, tool.Function)
		}
		geminiTools := geminiRequest.GetTools()
		if codeExecution {
			geminiTools = append(geminiTools, dto.GeminiChatTool{
				CodeExecution: make(map[string]string),
			})
		}
		if googleSearch {
			geminiTools = append(geminiTools, dto.GeminiChatTool{
				GoogleSearch: make(map[string]string),
			})
		}
		if urlContext {
			geminiTools = append(geminiTools, dto.GeminiChatTool{
				URLContext: make(map[string]string),
			})
		}
		if len(functions) > 0 {
			geminiTools = append(geminiTools, dto.GeminiChatTool{
				FunctionDeclarations: functions,
			})
		}
		geminiRequest.SetTools(geminiTools)
	}

	if textRequest.ResponseFormat != nil && (textRequest.ResponseFormat.Type == "json_schema" || textRequest.ResponseFormat.Type == "json_object") {
		geminiRequest.GenerationConfig.ResponseMimeType = "application/json"

		if len(textRequest.ResponseFormat.JsonSchema) > 0 {
			// 先将json.RawMessage解析
			var jsonSchema dto.FormatJsonSchema
			if err := common.Unmarshal(textRequest.ResponseFormat.JsonSchema, &jsonSchema); err == nil {
				cleanedSchema := removeAdditionalPropertiesWithDepth(jsonSchema.Schema, 0)
				geminiRequest.GenerationConfig.ResponseSchema = cleanedSchema
			}
		}
	}
	tool_call_ids := make(map[string]string)
	var system_content []string
	//shouldAddDummyModelMessage := false
	for _, message := range textRequest.Messages {
		if message.Role == "system" {
			system_content = append(system_content, message.StringContent())
			continue
		} else if message.Role == "tool" || message.Role == "function" {
			if len(geminiRequest.Contents) == 0 || geminiRequest.Contents[len(geminiRequest.Contents)-1].Role == "model" {
				geminiRequest.Contents = append(geminiRequest.Contents, dto.GeminiChatContent{
					Role: "user",
				})
			}
			var parts = &geminiRequest.Contents[len(geminiRequest.Contents)-1].Parts
			name := ""
			if message.Name != nil {
				name = *message.Name
			} else if val, exists := tool_call_ids[message.ToolCallId]; exists {
				name = val
			}
			var contentMap map[string]interface{}
			contentStr := message.StringContent()

			// 1. 尝试解析为 JSON 对象
			if err := json.Unmarshal([]byte(contentStr), &contentMap); err != nil {
				// 2. 如果失败，尝试解析为 JSON 数组
				var contentSlice []interface{}
				if err := json.Unmarshal([]byte(contentStr), &contentSlice); err == nil {
					// 如果是数组，包装成对象
					contentMap = map[string]interface{}{"result": contentSlice}
				} else {
					// 3. 如果再次失败，作为纯文本处理
					contentMap = map[string]interface{}{"content": contentStr}
				}
			}

			functionResp := &dto.GeminiFunctionResponse{
				Name:     name,
				Response: contentMap,
			}

			*parts = append(*parts, dto.GeminiPart{
				FunctionResponse: functionResp,
			})
			continue
		}
		var parts []dto.GeminiPart
		content := dto.GeminiChatContent{
			Role: message.Role,
		}
		// isToolCall := false
		if message.ToolCalls != nil {
			// message.Role = "model"
			// isToolCall = true
			for _, call := range message.ParseToolCalls() {
				args := map[string]interface{}{}
				if call.Function.Arguments != "" {
					if json.Unmarshal([]byte(call.Function.Arguments), &args) != nil {
						return nil, fmt.Errorf("invalid arguments for function %s, args: %s", call.Function.Name, call.Function.Arguments)
					}
				}
				toolCall := dto.GeminiPart{
					FunctionCall: &dto.FunctionCall{
						FunctionName: call.Function.Name,
						Arguments:    args,
					},
					ThoughtSignature: call.ThoughtSignature,
				}
				parts = append(parts, toolCall)
				tool_call_ids[call.ID] = call.Function.Name
			}
		}

		openaiContent := message.ParseContent()
		imageNum := 0
		for _, part := range openaiContent {
			if part.Type == dto.ContentTypeText {
				if part.Text == "" {
					continue
				}
				parts = append(parts, dto.GeminiPart{
					Text: part.Text,
				})
			} else if part.Type == dto.ContentTypeImageURL {
				imageNum += 1

				if constant.GeminiVisionMaxImageNum != -1 && imageNum > constant.GeminiVisionMaxImageNum {
					return nil, fmt.Errorf("too many images in the message, max allowed is %d", constant.GeminiVisionMaxImageNum)
				}
				// 判断是否是url
				if strings.HasPrefix(part.GetImageMedia().Url, "http") {
					// 是url，获取文件的类型和base64编码的数据
					fileData, err := service.GetFileBase64FromUrl(c, part.GetImageMedia().Url, "formatting image for Gemini")
					if err != nil {
						return nil, fmt.Errorf("get file base64 from url '%s' failed: %w", part.GetImageMedia().Url, err)
					}

					// 校验 MimeType 是否在 Gemini 支持的白名单中
					if _, ok := geminiSupportedMimeTypes[strings.ToLower(fileData.MimeType)]; !ok {
						url := part.GetImageMedia().Url
						return nil, fmt.Errorf("mime type is not supported by Gemini: '%s', url: '%s', supported types are: %v", fileData.MimeType, url, getSupportedMimeTypesList())
					}

					parts = append(parts, dto.GeminiPart{
						InlineData: &dto.GeminiInlineData{
							MimeType: fileData.MimeType, // 使用原始的 MimeType，因为大小写可能对API有意义
							Data:     fileData.Base64Data,
						},
					})
				} else {
					format, base64String, err := service.DecodeBase64FileData(part.GetImageMedia().Url)
					if err != nil {
						return nil, fmt.Errorf("decode base64 image data failed: %s", err.Error())
					}
					parts = append(parts, dto.GeminiPart{
						InlineData: &dto.GeminiInlineData{
							MimeType: format,
							Data:     base64String,
						},
					})
				}
			} else if part.Type == dto.ContentTypeFile {
				if part.GetFile().FileId != "" {
					return nil, fmt.Errorf("only base64 file is supported in gemini")
				}
				format, base64String, err := service.DecodeBase64FileData(part.GetFile().FileData)
				if err != nil {
					return nil, fmt.Errorf("decode base64 file data failed: %s", err.Error())
				}
				parts = append(parts, dto.GeminiPart{
					InlineData: &dto.GeminiInlineData{
						MimeType: format,
						Data:     base64String,
					},
				})
			} else if part.Type == dto.ContentTypeInputAudio {
				if part.GetInputAudio().Data == "" {
					return nil, fmt.Errorf("only base64 audio is supported in gemini")
				}
				base64String, err := service.DecodeBase64AudioData(part.GetInputAudio().Data)
				if err != nil {
					return nil, fmt.Errorf("decode base64 audio data failed: %s", err.Error())
				}
				parts = append(parts, dto.GeminiPart{
					InlineData: &dto.GeminiInlineData{
						MimeType: "audio/" + part.GetInputAudio().Format,
						Data:     base64String,
					},
				})
			}
		}

		content.Parts = parts

		// there's no assistant role in gemini and API shall vomit if Role is not user or model
		if content.Role == "assistant" {
			content.Role = "model"
		}
		if len(content.Parts) > 0 {
			geminiRequest.Contents = append(geminiRequest.Contents, content)
		}
	}

	// 上游要求 system_instruction 只能包含 text part，且空/纯空白的 system 指令可能触发 400。
	// 因此这里过滤掉空字符串/纯空白的 system 内容；若最终为空则不生成 systemInstruction。
	if len(system_content) > 0 {
		filtered := make([]string, 0, len(system_content))
		for _, s := range system_content {
			if strings.TrimSpace(s) == "" {
				continue
			}
			filtered = append(filtered, s)
		}
		system_content = filtered
	}
	if len(system_content) > 0 {
		geminiRequest.SystemInstructions = &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{
				{
					Text: strings.Join(system_content, "\n"),
				},
			},
		}
	}

	return &geminiRequest, nil
}

// Helper function to get a list of supported MIME types for error messages
func getSupportedMimeTypesList() []string {
	keys := make([]string, 0, len(geminiSupportedMimeTypes))
	for k := range geminiSupportedMimeTypes {
		keys = append(keys, k)
	}
	return keys
}

// cleanFunctionParameters recursively removes unsupported fields from Gemini function parameters.
func cleanFunctionParameters(params interface{}) interface{} {
	if params == nil {
		return nil
	}

	switch v := params.(type) {
	case map[string]interface{}:
		// Create a copy to avoid modifying the original
		cleanedMap := make(map[string]interface{})
		for k, val := range v {
			cleanedMap[k] = val
		}

		// Remove unsupported root-level fields
		delete(cleanedMap, "default")
		delete(cleanedMap, "exclusiveMaximum")
		delete(cleanedMap, "exclusiveMinimum")
		delete(cleanedMap, "$schema")
		delete(cleanedMap, "additionalProperties")

		// Vertex AI 要求 type 为单一字符串，不接受数组（如 ["string","null"]），需规范化为单值
		if typeVal, exists := cleanedMap["type"]; exists {
			if typeArr, ok := typeVal.([]interface{}); ok && len(typeArr) > 0 {
				for _, t := range typeArr {
					if s, ok := t.(string); ok && s != "null" {
						cleanedMap["type"] = s
						break
					}
				}
				if _, stillArray := cleanedMap["type"].([]interface{}); stillArray {
					cleanedMap["type"] = "string"
				}
			}
		}

		// Check and clean 'format' for string types
		if propType, typeExists := cleanedMap["type"].(string); typeExists && propType == "string" {
			if formatValue, formatExists := cleanedMap["format"].(string); formatExists {
				if formatValue != "enum" && formatValue != "date-time" {
					delete(cleanedMap, "format")
				}
			}
		}

		// Clean properties
		if props, ok := cleanedMap["properties"].(map[string]interface{}); ok && props != nil {
			cleanedProps := make(map[string]interface{})
			for propName, propValue := range props {
				cleanedProps[propName] = cleanFunctionParameters(propValue)
			}
			cleanedMap["properties"] = cleanedProps
		}

		// Recursively clean items in arrays
		if items, ok := cleanedMap["items"].(map[string]interface{}); ok && items != nil {
			cleanedMap["items"] = cleanFunctionParameters(items)
		}
		// Also handle items if it's an array of schemas
		if itemsArray, ok := cleanedMap["items"].([]interface{}); ok {
			cleanedItemsArray := make([]interface{}, len(itemsArray))
			for i, item := range itemsArray {
				cleanedItemsArray[i] = cleanFunctionParameters(item)
			}
			cleanedMap["items"] = cleanedItemsArray
		}

		// Recursively clean other schema composition keywords
		for _, field := range []string{"allOf", "anyOf", "oneOf"} {
			if nested, ok := cleanedMap[field].([]interface{}); ok {
				cleanedNested := make([]interface{}, len(nested))
				for i, item := range nested {
					cleanedNested[i] = cleanFunctionParameters(item)
				}
				cleanedMap[field] = cleanedNested
			}
		}

		// Recursively clean patternProperties
		if patternProps, ok := cleanedMap["patternProperties"].(map[string]interface{}); ok {
			cleanedPatternProps := make(map[string]interface{})
			for pattern, schema := range patternProps {
				cleanedPatternProps[pattern] = cleanFunctionParameters(schema)
			}
			cleanedMap["patternProperties"] = cleanedPatternProps
		}

		// Recursively clean definitions
		if definitions, ok := cleanedMap["definitions"].(map[string]interface{}); ok {
			cleanedDefinitions := make(map[string]interface{})
			for defName, defSchema := range definitions {
				cleanedDefinitions[defName] = cleanFunctionParameters(defSchema)
			}
			cleanedMap["definitions"] = cleanedDefinitions
		}

		// Recursively clean $defs (newer JSON Schema draft)
		if defs, ok := cleanedMap["$defs"].(map[string]interface{}); ok {
			cleanedDefs := make(map[string]interface{})
			for defName, defSchema := range defs {
				cleanedDefs[defName] = cleanFunctionParameters(defSchema)
			}
			cleanedMap["$defs"] = cleanedDefs
		}

		// Clean conditional keywords
		for _, field := range []string{"if", "then", "else", "not"} {
			if nested, ok := cleanedMap[field]; ok {
				cleanedMap[field] = cleanFunctionParameters(nested)
			}
		}

		return cleanedMap

	case []interface{}:
		// Handle arrays of schemas
		cleanedArray := make([]interface{}, len(v))
		for i, item := range v {
			cleanedArray[i] = cleanFunctionParameters(item)
		}
		return cleanedArray

	default:
		// Not a map or array, return as is (e.g., could be a primitive)
		return params
	}
}

func removeAdditionalPropertiesWithDepth(schema interface{}, depth int) interface{} {
	if depth >= 5 {
		return schema
	}

	v, ok := schema.(map[string]interface{})
	if !ok || len(v) == 0 {
		return schema
	}
	// 删除所有的title字段
	delete(v, "title")
	delete(v, "$schema")
	// 如果type不为object和array，则直接返回
	if typeVal, exists := v["type"]; !exists || (typeVal != "object" && typeVal != "array") {
		return schema
	}
	switch v["type"] {
	case "object":
		delete(v, "additionalProperties")
		// 处理 properties
		if properties, ok := v["properties"].(map[string]interface{}); ok {
			for key, value := range properties {
				properties[key] = removeAdditionalPropertiesWithDepth(value, depth+1)
			}
		}
		for _, field := range []string{"allOf", "anyOf", "oneOf"} {
			if nested, ok := v[field].([]interface{}); ok {
				for i, item := range nested {
					nested[i] = removeAdditionalPropertiesWithDepth(item, depth+1)
				}
			}
		}
	case "array":
		if items, ok := v["items"].(map[string]interface{}); ok {
			v["items"] = removeAdditionalPropertiesWithDepth(items, depth+1)
		}
	}

	return v
}

func unescapeString(s string) (string, error) {
	var result []rune
	escaped := false
	i := 0

	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:]) // 正确解码UTF-8字符
		if r == utf8.RuneError {
			return "", fmt.Errorf("invalid UTF-8 encoding")
		}

		if escaped {
			// 如果是转义符后的字符，检查其类型
			switch r {
			case '"':
				result = append(result, '"')
			case '\\':
				result = append(result, '\\')
			case '/':
				result = append(result, '/')
			case 'b':
				result = append(result, '\b')
			case 'f':
				result = append(result, '\f')
			case 'n':
				result = append(result, '\n')
			case 'r':
				result = append(result, '\r')
			case 't':
				result = append(result, '\t')
			case '\'':
				result = append(result, '\'')
			default:
				// 如果遇到一个非法的转义字符，直接按原样输出
				result = append(result, '\\', r)
			}
			escaped = false
		} else {
			if r == '\\' {
				escaped = true // 记录反斜杠作为转义符
			} else {
				result = append(result, r)
			}
		}
		i += size // 移动到下一个字符
	}

	return string(result), nil
}
func unescapeMapOrSlice(data interface{}) interface{} {
	switch v := data.(type) {
	case map[string]interface{}:
		for k, val := range v {
			v[k] = unescapeMapOrSlice(val)
		}
	case []interface{}:
		for i, val := range v {
			v[i] = unescapeMapOrSlice(val)
		}
	case string:
		if unescaped, err := unescapeString(v); err != nil {
			return v
		} else {
			return unescaped
		}
	}
	return data
}

func getResponseToolCall(item *dto.GeminiPart) *dto.ToolCallResponse {
	var argsBytes []byte
	var err error
	if result, ok := item.FunctionCall.Arguments.(map[string]interface{}); ok {
		argsBytes, err = json.Marshal(unescapeMapOrSlice(result))
	} else {
		argsBytes, err = json.Marshal(item.FunctionCall.Arguments)
	}

	if err != nil {
		return nil
	}
	resp := &dto.ToolCallResponse{
		ID:   fmt.Sprintf("call_%s", common.GetUUID()),
		Type: "function",
		Function: dto.FunctionResponse{
			Arguments: string(argsBytes),
			Name:      item.FunctionCall.FunctionName,
		},
	}
	if item.ThoughtSignature != "" {
		resp.ThoughtSignature = item.ThoughtSignature
	}
	return resp
}

// 检查是否为图像生成响应
func isImageGenerationResponse(response *dto.GeminiChatResponse) bool {
	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "image") {
				return true
			}
		}
	}
	//  谷歌问题,现在有时候只返回了text,但是没有图片,直接用版本来判断
	if strings.Contains(response.ModelVersion, "image") {
		return true
	}
	return false
}

// 将 Gemini 图像生成响应转换为 OpenAI ImageResponse 格式
func responseGeminiImageGeneration2OpenAI(response *dto.GeminiChatResponse) *dto.ImageResponse {
	imageResponse := &dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0),
	}

	for _, candidate := range response.Candidates {
		// 收集文本描述，用作 revised_prompt
		var textParts []string
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && strings.HasPrefix(part.InlineData.MimeType, "image") { // 处理图像数据
				imageData := dto.ImageData{
					B64Json: part.InlineData.Data,
				}
				// 如果有文本描述，添加到 revised_prompt
				if len(textParts) > 0 {
					imageData.RevisedPrompt = strings.Join(textParts, " ")
				}
				imageResponse.Data = append(imageResponse.Data, imageData)
			} else if part.Text != "" && part.Text != "\n" && !part.Thought {
				// 收集非空的文本内容（排除思考内容）
				textParts = append(textParts, part.Text)
			}
		}

		// 如果有图像但还没有设置 revised_prompt，为所有图像设置文本描述
		if len(textParts) > 0 {
			// 谷歌问题,现在有时候只返回了text,但是没有图片
			if len(imageResponse.Data) == 0 {
				imageData := dto.ImageData{
					B64Json: "",
				}
				imageResponse.Data = append(imageResponse.Data, imageData)
			}
			revisedPrompt := strings.Join(textParts, " ")
			for i := range imageResponse.Data {
				if imageResponse.Data[i].RevisedPrompt == "" {
					imageResponse.Data[i].RevisedPrompt = revisedPrompt
				}
			}
		}
	}

	return imageResponse
}

func responseGeminiChat2OpenAI(c *gin.Context, response *dto.GeminiChatResponse) *dto.OpenAITextResponse {
	fullTextResponse := dto.OpenAITextResponse{
		Id:      helper.GetResponseID(c),
		Object:  "chat.completion",
		Created: common.GetTimestamp(),
		Choices: make([]dto.OpenAITextResponseChoice, 0, len(response.Candidates)),
	}
	isToolCall := false
	for _, candidate := range response.Candidates {
		choice := dto.OpenAITextResponseChoice{
			Index: int(candidate.Index),
			Message: dto.Message{
				Role:    "assistant",
				Content: "",
			},
			FinishReason: constant.FinishReasonStop,
		}
		if len(candidate.Content.Parts) > 0 {
			var texts []string
			var toolCalls []dto.ToolCallResponse
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil {
					// 媒体内容
					if strings.HasPrefix(part.InlineData.MimeType, "image") {
						imgText := "![image](data:" + part.InlineData.MimeType + ";base64," + part.InlineData.Data + ")"
						texts = append(texts, imgText)
					} else {
						// 其他媒体类型，直接显示链接
						texts = append(texts, fmt.Sprintf("[media](data:%s;base64,%s)", part.InlineData.MimeType, part.InlineData.Data))
					}
				} else if part.FunctionCall != nil {
					choice.FinishReason = constant.FinishReasonToolCalls
					if call := getResponseToolCall(&part); call != nil {
						toolCalls = append(toolCalls, *call)
					}
				} else if part.Thought {
					choice.Message.ReasoningContent = part.Text
				} else {
					if part.ExecutableCode != nil {
						texts = append(texts, "```"+part.ExecutableCode.Language+"\n"+part.ExecutableCode.Code+"\n```")
					} else if part.CodeExecutionResult != nil {
						texts = append(texts, "```output\n"+part.CodeExecutionResult.Output+"\n```")
					} else {
						// 过滤掉空行
						if part.Text != "\n" {
							texts = append(texts, part.Text)
						}
					}
				}
			}
			if len(toolCalls) > 0 {
				choice.Message.SetToolCalls(toolCalls)
				isToolCall = true
			}
			choice.Message.SetStringContent(strings.Join(texts, "\n"))

		}
		if candidate.FinishReason != nil {
			switch *candidate.FinishReason {
			case "STOP":
				choice.FinishReason = constant.FinishReasonStop
			case "MAX_TOKENS":
				choice.FinishReason = constant.FinishReasonLength
			default:
				choice.FinishReason = constant.FinishReasonContentFilter
			}
		}
		if isToolCall {
			choice.FinishReason = constant.FinishReasonToolCalls
		}

		fullTextResponse.Choices = append(fullTextResponse.Choices, choice)
	}
	return &fullTextResponse
}

func streamResponseGeminiChat2OpenAI(geminiResponse *dto.GeminiChatResponse) (*dto.ChatCompletionsStreamResponse, bool) {
	choices := make([]dto.ChatCompletionsStreamResponseChoice, 0, len(geminiResponse.Candidates))
	isStop := false
	for _, candidate := range geminiResponse.Candidates {
		// 任何 finish reason 都表示流式响应结束
		if candidate.FinishReason != nil {
			isStop = true
			if *candidate.FinishReason == "STOP" {
				candidate.FinishReason = nil
			}
		}
		choice := dto.ChatCompletionsStreamResponseChoice{
			Index: int(candidate.Index),
			Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
				//Role: "assistant",
			},
		}
		var texts []string
		var thoughtTexts []string
		isTools := false
		hasThought := false
		hasContent := false
		if candidate.FinishReason != nil {
			// p := GeminiConvertFinishReason(*candidate.FinishReason)
			switch *candidate.FinishReason {
			case "STOP":
				choice.FinishReason = &constant.FinishReasonStop
			case "MAX_TOKENS":
				choice.FinishReason = &constant.FinishReasonLength
			default:
				choice.FinishReason = &constant.FinishReasonContentFilter
			}
		}
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				if strings.HasPrefix(part.InlineData.MimeType, "image") {
					imgText := "![image](data:" + part.InlineData.MimeType + ";base64," + part.InlineData.Data + ")"
					texts = append(texts, imgText)
					hasContent = true
				}
			} else if part.FunctionCall != nil {
				isTools = true
				if call := getResponseToolCall(&part); call != nil {
					call.SetIndex(len(choice.Delta.ToolCalls))
					choice.Delta.ToolCalls = append(choice.Delta.ToolCalls, *call)
				}

			} else if part.Thought {
				hasThought = true
				thoughtTexts = append(thoughtTexts, part.Text)
			} else {
				hasContent = true
				if part.ExecutableCode != nil {
					texts = append(texts, "```"+part.ExecutableCode.Language+"\n"+part.ExecutableCode.Code+"\n```\n")
				} else if part.CodeExecutionResult != nil {
					texts = append(texts, "```output\n"+part.CodeExecutionResult.Output+"\n```\n")
				} else {
					if part.Text != "\n" {
						texts = append(texts, part.Text)
					}
				}
			}
		}
		// 思考内容和普通内容可以同时存在
		if hasThought && len(thoughtTexts) > 0 {
			choice.Delta.SetReasoningContent(strings.Join(thoughtTexts, "\n"))
		}
		if hasContent && len(texts) > 0 {
			choice.Delta.SetContentString(strings.Join(texts, "\n"))
		} else if !hasThought && len(texts) > 0 {
			// 如果没有思考内容，但有待发送的文本，也设置 content（向后兼容）
			choice.Delta.SetContentString(strings.Join(texts, "\n"))
		}
		if isTools {
			choice.FinishReason = &constant.FinishReasonToolCalls
		}
		choices = append(choices, choice)
	}

	var response dto.ChatCompletionsStreamResponse
	response.Object = "chat.completion.chunk"
	response.Choices = choices
	return &response, isStop
}

func handleStream(c *gin.Context, info *relaycommon.RelayInfo, resp *dto.ChatCompletionsStreamResponse) error {
	streamData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal stream response: %w", err)
	}
	err = openai.HandleStreamFormat(c, info, string(streamData), info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
	if err != nil {
		return fmt.Errorf("failed to handle stream format: %w", err)
	}
	return nil
}

func handleFinalStream(c *gin.Context, info *relaycommon.RelayInfo, resp *dto.ChatCompletionsStreamResponse) error {
	streamData, err := common.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal stream response: %w", err)
	}
	openai.HandleFinalResponse(c, info, string(streamData), resp.Id, resp.Created, resp.Model, resp.GetSystemFingerprint(), resp.Usage, false)
	return nil
}

func GeminiChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	// responseText := ""
	id := helper.GetResponseID(c)
	createAt := common.GetTimestamp()
	responseText := strings.Builder{}
	rawRespBuilder := strings.Builder{}
	var usage = &dto.Usage{}
	var imageCount int
	finishReason := constant.FinishReasonStop
	var sentToolCallContent bool // 整次流是否发送过带 tool_calls 内容的 chunk

	// 累积所有思考内容（workaround for Google API issue）
	var accumulatedThoughts []string
	var hasThoughtsTokenCount bool
	var totalThoughtsTokenCount int
	var thoughtsSent bool // 是否已发送思考内容

	chunkIndex := 0
	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		chunkIndex++
		if rawRespBuilder.Len() < 2000 {
			rawRespBuilder.WriteString(data)
			rawRespBuilder.WriteByte('\n')
		}

		// 打印 Gemini 原始返回（完整 JSON）
		common.SysLog(fmt.Sprintf("[GeminiRawResponse] model=%s, chunk[%d], raw_data=%s",
			info.UpstreamModelName, chunkIndex, data))

		var geminiResponse dto.GeminiChatResponse
		err := common.UnmarshalJsonStr(data, &geminiResponse)
		if err != nil {
			logger.LogError(c, "error unmarshalling stream response: "+err.Error())
			return false
		}

		// 打印解析后的结构化数据（用于对比分析）
		geminiResponseJson, _ := common.Marshal(geminiResponse)
		common.SysLog(fmt.Sprintf("[GeminiParsedResponse] model=%s, chunk[%d], parsed_json=%s",
			info.UpstreamModelName, chunkIndex, string(geminiResponseJson)))

		// 记录每个 chunk 的完整原始数据（前500字符）用于调试
		dataPreview := data
		if len(dataPreview) > 500 {
			dataPreview = dataPreview[:500] + "..."
		}
		common.SysLog(fmt.Sprintf("[GeminiStreamDebug] model=%s, chunk[%d] RAW (first 500 chars): %s",
			info.UpstreamModelName, chunkIndex, dataPreview))

		// 记录每个 chunk 的基本信息
		common.SysLog(fmt.Sprintf("[GeminiStreamDebug] model=%s, chunk[%d]: hasCandidates=%v, candidatesCount=%d, hasUsageMetadata=%v, thoughtsTokenCount=%d",
			info.UpstreamModelName, chunkIndex, len(geminiResponse.Candidates) > 0, len(geminiResponse.Candidates),
			geminiResponse.UsageMetadata.TotalTokenCount > 0, geminiResponse.UsageMetadata.ThoughtsTokenCount))

		// 调试日志：检查思考内容相关的字段
		if len(geminiResponse.Candidates) > 0 {
			for _, candidate := range geminiResponse.Candidates {
				// 记录每个 candidate 的 parts 数量
				partsCount := len(candidate.Content.Parts)
				common.SysLog(fmt.Sprintf(
					"[GeminiStreamDebug] model=%s, candidate has %d parts",
					info.UpstreamModelName, partsCount,
				))

				for i, part := range candidate.Content.Parts {
					if part.Text != "" || part.Thought {
						textPreview := part.Text
						if len(textPreview) > 50 {
							textPreview = textPreview[:50] + "..."
						}
						common.SysLog(fmt.Sprintf(
							"[GeminiStreamDebug] model=%s, part[%d]: text_len=%d, thought=%v, hasFunctionCall=%v, text_preview=%s",
							info.UpstreamModelName, i, len(part.Text), part.Thought, part.FunctionCall != nil, textPreview,
						))
					}
				}
			}
			// 检查 usage metadata 中的思考 token 计数
			// 注意：需要累积所有 chunk 的 thoughtsTokenCount，因为有些 chunk 可能只有 metadata 没有 parts
			if geminiResponse.UsageMetadata.ThoughtsTokenCount > 0 {
				hasThoughtsTokenCount = true
				// 累积所有 chunk 的 thoughtsTokenCount，取最大值（因为最后一个 chunk 通常包含完整的计数）
				if geminiResponse.UsageMetadata.ThoughtsTokenCount > totalThoughtsTokenCount {
					totalThoughtsTokenCount = geminiResponse.UsageMetadata.ThoughtsTokenCount
				}
				common.SysLog(fmt.Sprintf(
					"[GeminiStreamDebug] model=%s chunk[%d] has thoughtsTokenCount=%d, totalThoughtsTokenCount=%d, checking for thought parts...",
					info.UpstreamModelName, chunkIndex, geminiResponse.UsageMetadata.ThoughtsTokenCount, totalThoughtsTokenCount,
				))
				// 检查是否有 Thought 为 true 的 part
				hasThoughtPart := false
				for _, candidate := range geminiResponse.Candidates {
					// 检查 parts 是否为 nil
					if candidate.Content.Parts == nil {
						common.SysLog(fmt.Sprintf(
							"[GeminiStreamDebug] model=%s chunk[%d] candidate.Content.Parts is nil, but thoughtsTokenCount=%d",
							info.UpstreamModelName, chunkIndex, geminiResponse.UsageMetadata.ThoughtsTokenCount,
						))
						continue
					}
					for _, part := range candidate.Content.Parts {
						if part.Thought {
							hasThoughtPart = true
							// 累积思考内容
							if part.Text != "" {
								accumulatedThoughts = append(accumulatedThoughts, part.Text)
							}
							textPreview := part.Text
							if len(textPreview) > 100 {
								textPreview = textPreview[:100] + "..."
							}
							common.SysLog(fmt.Sprintf(
								"[GeminiStreamDebug] Found thought part in chunk[%d]: text_len=%d, text_preview=%s",
								chunkIndex, len(part.Text), textPreview,
							))
						}
					}
				}
				if !hasThoughtPart {
					// 记录完整的原始响应数据（用于调试）
					rawDataPreview := data
					if len(rawDataPreview) > 1000 {
						rawDataPreview = rawDataPreview[:1000] + "..."
					}
					common.SysLog(fmt.Sprintf(
						"[GeminiStreamDebug] WARNING: model=%s chunk[%d] has thoughtsTokenCount=%d but no part.Thought=true detected! Raw response: %s",
						info.UpstreamModelName, chunkIndex, geminiResponse.UsageMetadata.ThoughtsTokenCount, rawDataPreview,
					))

					// 检查 candidate.Content 是否有其他字段
					if len(geminiResponse.Candidates) > 0 {
						candidate := geminiResponse.Candidates[0]
						contentJson, _ := common.Marshal(candidate.Content)
						contentPreview := string(contentJson)
						if len(contentPreview) > 500 {
							contentPreview = contentPreview[:500] + "..."
						}
						common.SysLog(fmt.Sprintf(
							"[GeminiStreamDebug] chunk[%d] candidate.Content structure: %s",
							chunkIndex, contentPreview,
						))
					}
				}
			}
		} else {
			// 如果没有 candidates，记录完整的响应
			rawDataPreview := data
			if len(rawDataPreview) > 500 {
				rawDataPreview = rawDataPreview[:500] + "..."
			}
			common.SysLog(fmt.Sprintf(
				"[GeminiStreamDebug] model=%s, chunk has no candidates. Raw response: %s",
				info.UpstreamModelName, rawDataPreview,
			))
		}

		for _, candidate := range geminiResponse.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.InlineData != nil && part.InlineData.MimeType != "" {
					imageCount++
				}
				if part.Text != "" {
					responseText.WriteString(part.Text)
				}
			}
		}

		// 分离思考内容和普通内容
		var thoughtParts []dto.GeminiPart
		var contentParts []dto.GeminiPart
		for _, candidate := range geminiResponse.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Thought {
					thoughtParts = append(thoughtParts, part)
				} else {
					contentParts = append(contentParts, part)
				}
			}
		}

		// 如果有思考内容且尚未发送，先发送思考内容
		if len(thoughtParts) > 0 && !thoughtsSent {
			var thoughtTexts []string
			for _, part := range thoughtParts {
				if part.Text != "" {
					thoughtTexts = append(thoughtTexts, part.Text)
				}
			}
			if len(thoughtTexts) > 0 {
				thoughtResponse := &dto.ChatCompletionsStreamResponse{
					Id:      id,
					Object:  "chat.completion.chunk",
					Created: createAt,
					Model:   info.UpstreamModelName,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{},
						},
					},
				}
				thoughtResponse.Choices[0].Delta.SetReasoningContent(strings.Join(thoughtTexts, "\n"))
				common.SysLog(fmt.Sprintf("[Gemini] 先发送思考内容，共 %d 条，总长度 %d 字符",
					len(thoughtTexts), len(strings.Join(thoughtTexts, "\n"))))
				// 在第一个响应之前发送思考内容
				if info.SendResponseCount == 0 {
					emptyResponse := helper.GenerateStartEmptyResponse(id, createAt, info.UpstreamModelName, nil)
					_ = handleStream(c, info, emptyResponse)
				}
				_ = handleStream(c, info, thoughtResponse)
				thoughtsSent = true
			}
		}

		// 创建只包含普通内容的响应（临时修改 geminiResponse）
		originalParts := make([][]dto.GeminiPart, len(geminiResponse.Candidates))
		for i := range geminiResponse.Candidates {
			originalParts[i] = make([]dto.GeminiPart, len(geminiResponse.Candidates[i].Content.Parts))
			copy(originalParts[i], geminiResponse.Candidates[i].Content.Parts)
			// 只保留非思考内容
			geminiResponse.Candidates[i].Content.Parts = contentParts
		}

		response, isStop := streamResponseGeminiChat2OpenAI(&geminiResponse)

		// 恢复原始 parts（用于后续处理）
		for i := range geminiResponse.Candidates {
			geminiResponse.Candidates[i].Content.Parts = originalParts[i]
		}

		response.Id = id
		response.Created = createAt
		response.Model = info.UpstreamModelName
		if geminiResponse.UsageMetadata.TotalTokenCount != 0 {
			usage.PromptTokens = geminiResponse.UsageMetadata.PromptTokenCount
			usage.CompletionTokens = geminiResponse.UsageMetadata.CandidatesTokenCount
			usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount
			usage.TotalTokens = geminiResponse.UsageMetadata.TotalTokenCount

			// 处理缓存 tokens（Context Caching）
			if geminiResponse.UsageMetadata.CachedContentTokenCount > 0 {
				usage.PromptTokensDetails.CachedTokens = geminiResponse.UsageMetadata.CachedContentTokenCount
			}

			for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
				if detail.Modality == "AUDIO" {
					usage.PromptTokensDetails.AudioTokens = detail.TokenCount
				} else if detail.Modality == "TEXT" {
					usage.PromptTokensDetails.TextTokens = detail.TokenCount
				}
			}
		}
		logger.LogDebug(c, fmt.Sprintf("info.SendResponseCount = %d", info.SendResponseCount))
		if info.SendResponseCount == 0 {
			// send first response
			emptyResponse := helper.GenerateStartEmptyResponse(id, createAt, info.UpstreamModelName, nil)
			if response.IsToolCall() {
				if len(emptyResponse.Choices) > 0 && len(response.Choices) > 0 {
					toolCalls := response.Choices[0].Delta.ToolCalls
					copiedToolCalls := make([]dto.ToolCallResponse, len(toolCalls))
					for idx := range toolCalls {
						copiedToolCalls[idx] = toolCalls[idx]
						copiedToolCalls[idx].Function.Arguments = ""
					}
					emptyResponse.Choices[0].Delta.ToolCalls = copiedToolCalls
				}
				finishReason = constant.FinishReasonToolCalls
				err = handleStream(c, info, emptyResponse)
				if err != nil {
					logger.LogError(c, err.Error())
				}

				response.ClearToolCalls()
				if response.IsFinished() {
					response.Choices[0].FinishReason = nil
				}
			} else {
				err = handleStream(c, info, emptyResponse)
				if err != nil {
					logger.LogError(c, err.Error())
				}
			}
		}

		// 仅在有内容时向客户端发送内容块和 stop 块，避免「仅有 usage、无 candidates」时仍 200+ 计费
		if len(response.Choices) > 0 {
			if response.IsToolCall() && len(response.Choices[0].Delta.ToolCalls) > 0 {
				sentToolCallContent = true
			}
			err = handleStream(c, info, response)
			if err != nil {
				logger.LogError(c, err.Error())
			}
		}
		if isStop {
			// 在最后一个响应块中提取图片和文本输出 tokens
			var imageOutputTokens int
			var textOutputTokens int
			for _, detail := range geminiResponse.UsageMetadata.CandidatesTokensDetails {
				if detail.Modality == "IMAGE" {
					imageOutputTokens += detail.TokenCount
				} else if detail.Modality == "TEXT" {
					textOutputTokens += detail.TokenCount
				}
			}
			// 将图片和文本输出 tokens 存储到 context，供计费逻辑使用
			if imageOutputTokens > 0 || textOutputTokens > 0 {
				c.Set("gemini_image_output_tokens", imageOutputTokens)
				c.Set("gemini_text_output_tokens", textOutputTokens)
			}

			// 只在真正有思考内容时才发送，如果没有思考内容就不显示
			if hasThoughtsTokenCount && totalThoughtsTokenCount > 0 && len(accumulatedThoughts) > 0 && !thoughtsSent {
				// 如果累积了思考内容但尚未发送，现在发送
				thoughtResponse := &dto.ChatCompletionsStreamResponse{
					Id:      id,
					Object:  "chat.completion.chunk",
					Created: createAt,
					Model:   info.UpstreamModelName,
					Choices: []dto.ChatCompletionsStreamResponseChoice{
						{
							Index: 0,
							Delta: dto.ChatCompletionsStreamResponseChoiceDelta{},
						},
					},
				}
				thoughtResponse.Choices[0].Delta.SetReasoningContent(strings.Join(accumulatedThoughts, "\n"))
				common.SysLog(fmt.Sprintf("[Gemini] 发送累积的思考内容，共 %d 条，总长度 %d 字符",
					len(accumulatedThoughts), len(strings.Join(accumulatedThoughts, "\n"))))
				_ = handleStream(c, info, thoughtResponse)
				thoughtsSent = true
			} else if hasThoughtsTokenCount && totalThoughtsTokenCount > 0 && len(accumulatedThoughts) == 0 && !thoughtsSent {
				// 如果有 thoughtsTokenCount 但没有累积到思考内容，记录详细日志用于分析
				common.SysLog(fmt.Sprintf(
					"[GeminiStreamDebug] model=%s 最终统计: hasThoughtsTokenCount=%v, totalThoughtsTokenCount=%d, accumulatedThoughts=%d, thoughtsSent=%v",
					info.UpstreamModelName, hasThoughtsTokenCount, totalThoughtsTokenCount, len(accumulatedThoughts), thoughtsSent,
				))
				common.SysLog(fmt.Sprintf(
					"[GeminiStreamDebug] model=%s Google API 进行了思考（thoughtsTokenCount=%d），但没有返回思考内容（parts:null）。这是 Google API 的已知问题。",
					info.UpstreamModelName, totalThoughtsTokenCount,
				))
			}

			if len(response.Choices) > 0 {
				_ = handleStream(c, info, helper.GenerateStopResponse(id, createAt, info.UpstreamModelName, finishReason))
			}
		}
		return true
	})

	isEmptyResponse := (info.SendResponseCount == 0) ||
		(responseText.Len() == 0 && info.SendResponseCount <= 1) ||
		(responseText.Len() == 0 && !sentToolCallContent)
	if isEmptyResponse {
		reqBody, _ := c.Get("gemini_request_body")
		truncatedReq := common.TruncateJsonValues(fmt.Sprintf("%v", reqBody))
		rawResp := rawRespBuilder.String()
		logger.LogWarn(c, fmt.Sprintf(
			"[GeminiEmptyResponse] upstream 200 but no content. model=%s",
			info.UpstreamModelName,
		))
		extraContent := fmt.Sprintf(
			"Gemini空响应(流式) requestBody=%s rawResponse=%s",
			truncatedReq, rawResp,
		)
		c.Set("gemini_empty_response_extra", extraContent)

		emptyUsage := &dto.Usage{}
		// 发送一个最终 usage 响应，保证客户端正常结束流
		response := helper.GenerateFinalUsageResponse(id, createAt, info.UpstreamModelName, *emptyUsage)
		if err := handleFinalStream(c, info, response); err != nil {
			common.SysLog("send final empty response failed: " + err.Error())
		}
		return emptyUsage, nil
	}

	if imageCount != 0 {
		if usage.CompletionTokens == 0 {
			usage.CompletionTokens = imageCount * 258
		}
	}

	usage.PromptTokensDetails.TextTokens = usage.PromptTokens
	usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens

	if usage.CompletionTokens == 0 {
		str := responseText.String()
		if len(str) > 0 {
			usage = service.ResponseText2Usage(responseText.String(), info.UpstreamModelName, info.PromptTokens)
		} else {
			// 空补全，不需要使用量
			usage = &dto.Usage{}
		}
	}

	response := helper.GenerateFinalUsageResponse(id, createAt, info.UpstreamModelName, *usage)
	err := handleFinalStream(c, info, response)
	if err != nil {
		common.SysLog("send final response failed: " + err.Error())
	}
	//if info.RelayFormat == relaycommon.RelayFormatOpenAI {
	//	helper.Done(c)
	//}
	//resp.Body.Close()
	return usage, nil
}

func GeminiChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	if common.DebugEnabled {
		// 使用截断函数处理base64内容，保留其他信息
		truncatedContent := common.TruncateBase64Content(string(responseBody))
		println("[Gemini-GeminiChatHandler] " + truncatedContent)
	}
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if len(geminiResponse.Candidates) == 0 {
		//return nil, types.NewOpenAIError(errors.New("no candidates returned"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			return nil, types.NewOpenAIError(errors.New("request blocked by Gemini API: "+*geminiResponse.PromptFeedback.BlockReason), types.ErrorCodePromptBlocked, http.StatusBadRequest)
		} else {
			// 空响应：不报错，记录日志并返回空 usage
			reqBody, _ := c.Get("gemini_request_body")
			truncatedReq := common.TruncateJsonValues(fmt.Sprintf("%v", reqBody))
			rawRespStr := common.TruncateBase64Content(string(responseBody))
			logger.LogWarn(c, fmt.Sprintf(
				"[GeminiEmptyResponse] upstream 200 but no candidates. model=%s",
				info.UpstreamModelName,
			))
			extraContent := fmt.Sprintf(
				"Gemini空响应(非流式) requestBody=%s rawResponse=%s",
				truncatedReq, rawRespStr,
			)
			c.Set("gemini_empty_response_extra", extraContent)

			emptyUsage := &dto.Usage{}

			// 构造一个合法的空 OpenAI 响应（空 choices）
			emptyResp := dto.OpenAITextResponse{
				Id:      helper.GetResponseID(c),
				Model:   info.UpstreamModelName,
				Object:  "chat.completion",
				Created: common.GetTimestamp(),
				Choices: []dto.OpenAITextResponseChoice{},
				Usage:   *emptyUsage,
			}
			emptyRespBytes, marshalErr := common.Marshal(emptyResp)
			if marshalErr != nil {
				logger.LogError(c, fmt.Sprintf("marshal empty OpenAI response failed: %s", marshalErr.Error()))
				// 即使 marshal 失败，也返回空 usage，避免再次抛错
				return emptyUsage, nil
			}

			service.IOCopyBytesGracefully(c, resp, emptyRespBytes)
			return emptyUsage, nil
		}
	}

	// 检查是否为图像生成响应
	if isImageGenerationResponse(&geminiResponse) {
		// 打印原始 Gemini 响应结构（截断 base64 等长字段）
		if rawBytes, marshalErr := json.Marshal(&geminiResponse); marshalErr == nil {
			logger.LogInfo(c, "Gemini image generation raw response: "+common.TruncateJsonValues(string(rawBytes)))
		}

		// 转换为图像响应格式
		imageResponse := responseGeminiImageGeneration2OpenAI(&geminiResponse)

		// 从 CandidatesTokensDetails 中提取图片和文本输出 tokens
		var imageOutputTokens int
		var textOutputTokens int
		for _, detail := range geminiResponse.UsageMetadata.CandidatesTokensDetails {
			if detail.Modality == "IMAGE" {
				imageOutputTokens += detail.TokenCount
			} else if detail.Modality == "TEXT" {
				textOutputTokens += detail.TokenCount
			}
		}

		// 将图片和文本输出 tokens 存储到 context，供计费逻辑使用
		if imageOutputTokens > 0 || textOutputTokens > 0 {
			c.Set("gemini_image_output_tokens", imageOutputTokens)
			c.Set("gemini_text_output_tokens", textOutputTokens)
		}

		// 使用API返回的真实token使用量（与对话接口一致）
		usage := dto.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.PromptTokenCount + geminiResponse.UsageMetadata.CandidatesTokenCount,
		}

		// 处理缓存 tokens（Context Caching）
		if geminiResponse.UsageMetadata.CachedContentTokenCount > 0 {
			usage.PromptTokensDetails.CachedTokens = geminiResponse.UsageMetadata.CachedContentTokenCount
		}

		imageResponse.Usage = &usage

		// 序列化图像响应
		responseBody, err = json.Marshal(imageResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}

		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.WriteHeader(resp.StatusCode)
		_, _ = c.Writer.Write(responseBody)

		return &usage, nil
	}

	// 处理普通文本响应
	fullTextResponse := responseGeminiChat2OpenAI(c, &geminiResponse)
	fullTextResponse.Model = info.UpstreamModelName
	usage := dto.Usage{
		PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
		TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
	}

	usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount
	usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens

	// 处理缓存 tokens（Context Caching）
	if geminiResponse.UsageMetadata.CachedContentTokenCount > 0 {
		usage.PromptTokensDetails.CachedTokens = geminiResponse.UsageMetadata.CachedContentTokenCount
	}

	for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
		if detail.Modality == "AUDIO" {
			usage.PromptTokensDetails.AudioTokens = detail.TokenCount
		} else if detail.Modality == "TEXT" {
			usage.PromptTokensDetails.TextTokens = detail.TokenCount
		}
	}

	fullTextResponse.Usage = usage

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		responseBody, err = common.Marshal(fullTextResponse)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(fullTextResponse, info)
		claudeRespStr, err := common.Marshal(claudeResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		break
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func GeminiEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var geminiResponse dto.GeminiBatchEmbeddingResponse
	if jsonErr := common.Unmarshal(responseBody, &geminiResponse); jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// convert to openai format response
	openAIResponse := dto.OpenAIEmbeddingResponse{
		Object: "list",
		Data:   make([]dto.OpenAIEmbeddingResponseItem, 0, len(geminiResponse.Embeddings)),
		Model:  info.UpstreamModelName,
	}

	for i, embedding := range geminiResponse.Embeddings {
		openAIResponse.Data = append(openAIResponse.Data, dto.OpenAIEmbeddingResponseItem{
			Object:    "embedding",
			Embedding: embedding.Values,
			Index:     i,
		})
	}

	// calculate usage
	// https://ai.google.dev/gemini-api/docs/pricing?hl=zh-cn#text-embedding-004
	// Google has not yet clarified how embedding models will be billed
	// refer to openai billing method to use input tokens billing
	// https://platform.openai.com/docs/guides/embeddings#what-are-embeddings
	usage := &dto.Usage{
		PromptTokens:     info.PromptTokens,
		CompletionTokens: 0,
		TotalTokens:      info.PromptTokens,
	}
	openAIResponse.Usage = *usage

	jsonResponse, jsonErr := common.Marshal(openAIResponse)
	if jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}

func GeminiImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	responseBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, types.NewOpenAIError(readErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var geminiResponse dto.GeminiImageResponse
	if jsonErr := common.Unmarshal(responseBody, &geminiResponse); jsonErr != nil {
		return nil, types.NewOpenAIError(jsonErr, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if len(geminiResponse.Predictions) == 0 {
		return nil, types.NewOpenAIError(errors.New("no images generated"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// convert to openai format response
	openAIResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0, len(geminiResponse.Predictions)),
	}

	for _, prediction := range geminiResponse.Predictions {
		if prediction.RaiFilteredReason != "" {
			continue // skip filtered image
		}
		openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{
			B64Json: prediction.BytesBase64Encoded,
		})
	}

	// https://github.com/google-gemini/cookbook/blob/719a27d752aac33f39de18a8d3cb42a70874917e/quickstarts/Counting_Tokens.ipynb
	// each image has fixed 258 tokens
	const imageTokens = 258
	generatedImages := len(openAIResponse.Data)
	usage := &dto.Usage{
		PromptTokens:     imageTokens * generatedImages,
		CompletionTokens: 0,
		TotalTokens:      imageTokens * generatedImages,
	}
	openAIResponse.Usage = usage

	jsonResponse, jsonErr := json.Marshal(openAIResponse)
	if jsonErr != nil {
		return nil, types.NewError(jsonErr, types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)

	return usage, nil
}
