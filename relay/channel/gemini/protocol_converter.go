package gemini

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/dto"
	"strings"
)

// DetectProtocolMode 根据首条消息判断协议模式
func DetectProtocolMode(message []byte) dto.ProtocolMode {
	var rawMsg map[string]interface{}
	if err := json.Unmarshal(message, &rawMsg); err != nil {
		return dto.ProtocolModeOpenAI // 默认为 OpenAI
	}

	// 检查是否为 OpenAI Realtime 格式
	if msgType, ok := rawMsg["type"].(string); ok {
		if strings.HasPrefix(msgType, "session.") ||
			strings.HasPrefix(msgType, "input_audio_buffer.") ||
			strings.HasPrefix(msgType, "conversation.") ||
			strings.HasPrefix(msgType, "response.") {
			return dto.ProtocolModeOpenAI
		}
	}

	// 检查是否为 Gemini Live 格式
	if _, hasSetup := rawMsg["setup"]; hasSetup {
		return dto.ProtocolModeGemini
	}
	if _, hasClientContent := rawMsg["clientContent"]; hasClientContent {
		return dto.ProtocolModeGemini
	}
	if _, hasRealtimeInput := rawMsg["realtimeInput"]; hasRealtimeInput {
		return dto.ProtocolModeGemini
	}

	// 默认为 OpenAI 格式
	return dto.ProtocolModeOpenAI
}

// ConvertOpenAIToGeminiLive 将 OpenAI Realtime 事件转换为 Gemini Live 消息
// modelName 应该是完整的模型资源路径，例如：projects/{project}/locations/{location}/publishers/google/models/{model}
// config 包含从 URL 参数解析的配置（可选）
func ConvertOpenAIToGeminiLive(realtimeEvent *dto.RealtimeEvent, modelName string, config *GeminiLiveConfig) (*dto.GeminiLiveMessage, error) {
	geminiMsg := &dto.GeminiLiveMessage{}

	// #region agent log
	common.SysLog(fmt.Sprintf("[GeminiLive][H1][DEBUG] ConvertOpenAIToGeminiLive - EventType: %s, ModelName: %s", realtimeEvent.Type, modelName))
	// #endregion

	switch realtimeEvent.Type {
	case dto.RealtimeEventTypeSessionUpdate:
		// 转换为 setup 消息
		setup := &dto.GeminiLiveSetup{
			Model: modelName, // 使用短模型名称（Python SDK 模式）
		}

		// #region agent log
		common.SysLog(fmt.Sprintf("[GeminiLive][H1][DEBUG] Creating setup message with Model: %s", modelName))
		// #endregion

		if realtimeEvent.Session != nil {
			// 生成配置
			genConfig := &dto.GeminiLiveGenerationConfig{}
			if realtimeEvent.Session.Temperature != 0 {
				temp := realtimeEvent.Session.Temperature
				genConfig.Temperature = &temp
			}

			// 响应模态
			// 注意：Gemini Live API 只允许一个响应模态
			// 如果同时需要文本和音频，应该只指定 AUDIO，然后启用转录功能
			if len(realtimeEvent.Session.Modalities) > 0 {
				genConfig.ResponseModalities = make([]string, 0)
				hasAudio := false
				hasText := false

				for _, mod := range realtimeEvent.Session.Modalities {
					if mod == "audio" {
						hasAudio = true
					} else if mod == "text" {
						hasText = true
					}
				}

				// 优先使用 AUDIO（如果请求了音频）
				if hasAudio {
					genConfig.ResponseModalities = append(genConfig.ResponseModalities, "AUDIO")
					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H17][INFO] Using AUDIO response modality (hasText=%t, hasAudio=%t)", hasText, hasAudio))
					// #endregion
				} else if hasText {
					genConfig.ResponseModalities = append(genConfig.ResponseModalities, "TEXT")
					// #region agent log
					common.SysLog("[GeminiLive][H17][INFO] Using TEXT response modality")
					// #endregion
				}
			}

			// 语音配置
			if realtimeEvent.Session.Voice != "" {
				voiceName := MapOpenAIVoiceToGemini(realtimeEvent.Session.Voice)
				genConfig.SpeechConfig = &dto.GeminiLiveSpeechConfig{
					VoiceConfig: &dto.GeminiLiveVoiceConfig{
						PrebuiltVoiceConfig: &dto.GeminiLivePrebuiltVoiceConfig{
							VoiceName: voiceName,
						},
					},
				}
			}

			setup.GenerationConfig = genConfig

			// 系统指令
			if realtimeEvent.Session.Instructions != "" {
				setup.SystemInstruction = &dto.GeminiLiveContent{
					Parts: []dto.GeminiLivePartData{
						{Text: realtimeEvent.Session.Instructions},
					},
				}
			}

			// 工具
			if len(realtimeEvent.Session.Tools) > 0 {
				setup.Tools = ConvertOpenAIToolsToGemini(realtimeEvent.Session.Tools)
			}
		}

		// 应用配置（如果提供）
		if config != nil {
			config.ApplyToSetup(setup)
		}

		// 移除不支持的字段（Gemini Live API 目前不支持这些字段）
		if setup.GenerationConfig != nil {
			setup.GenerationConfig.InputContextWindowThreshold = nil
			setup.GenerationConfig.TargetContextSize = nil
		}

		geminiMsg.Setup = setup

	case dto.RealtimeEventInputAudioBufferAppend:
		// 转换为 realtimeInput 消息
		if realtimeEvent.Audio != "" {
			geminiMsg.RealtimeInput = &dto.GeminiLiveRealtimeInput{
				MediaChunks: []dto.GeminiLiveMediaChunk{
					{
						MimeType: "audio/pcm;rate=16000", // Gemini 需要 16kHz
						Data:     realtimeEvent.Audio,
					},
				},
			}
		}

	case dto.RealtimeEventTypeConversationCreate:
		// 转换为 clientContent 消息
		// #region agent log
		common.SysLog(fmt.Sprintf("[GeminiLive][H20][DEBUG] conversation.item.create - Item is nil: %t", realtimeEvent.Item == nil))
		if realtimeEvent.Item != nil {
			common.SysLog(fmt.Sprintf("[GeminiLive][H20][DEBUG] Item details - Role: %s, ContentLen: %d", realtimeEvent.Item.Role, len(realtimeEvent.Item.Content)))
		}
		// #endregion

		if realtimeEvent.Item != nil {
			clientContent := &dto.GeminiLiveClientContent{
				Turns: []dto.GeminiLiveContent{},
			}

			turn := dto.GeminiLiveContent{
				Role:  realtimeEvent.Item.Role,
				Parts: []dto.GeminiLivePartData{},
			}

			for i, content := range realtimeEvent.Item.Content {
				// #region agent log
				common.SysLog(fmt.Sprintf("[GeminiLive][H20][DEBUG] Content[%d] - Type: %s, Text: '%s', AudioLen: %d", i, content.Type, content.Text, len(content.Audio)))
				// #endregion

				part := dto.GeminiLivePartData{}
				// OpenAI 使用 "input_text" 和 "input_audio" 作为类型
				if (content.Type == "text" || content.Type == "input_text") && content.Text != "" {
					part.Text = content.Text
					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H20][DEBUG] Added text part: '%s'", content.Text))
					// #endregion
				} else if (content.Type == "audio" || content.Type == "input_audio") && content.Audio != "" {
					part.InlineData = &dto.GeminiLiveInlineData{
						MimeType: "audio/pcm;rate=16000",
						Data:     content.Audio,
					}
					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H20][DEBUG] Added audio part, len: %d", len(content.Audio)))
					// #endregion
				}
				if part.Text != "" || part.InlineData != nil {
					turn.Parts = append(turn.Parts, part)
				}
			}

			// #region agent log
			common.SysLog(fmt.Sprintf("[GeminiLive][H20][DEBUG] Final turn.Parts length: %d", len(turn.Parts)))
			// #endregion

			if len(turn.Parts) > 0 {
				clientContent.Turns = append(clientContent.Turns, turn)
				clientContent.TurnComplete = true // 标记回合完成
			}
			geminiMsg.ClientContent = clientContent
		}

	case dto.RealtimeEventTypeResponseCreate:
		// Gemini 不需要显式的 response.create，它会自动响应
		// 返回空消息，调用方应跳过发送
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported OpenAI Realtime event type: %s", realtimeEvent.Type)
	}

	return geminiMsg, nil
}

// ConvertGeminiLiveToOpenAI 将 Gemini Live 消息转换为 OpenAI Realtime 事件
func ConvertGeminiLiveToOpenAI(geminiMsg *dto.GeminiLiveMessage) (*dto.RealtimeEvent, error) {
	realtimeEvent := &dto.RealtimeEvent{
		EventId: common.GetUUID(),
	}

	// 处理 setupComplete
	if geminiMsg.SetupComplete != nil {
		realtimeEvent.Type = dto.RealtimeEventTypeSessionCreated
		realtimeEvent.Session = &dto.RealtimeSession{}
		return realtimeEvent, nil
	}

	// 处理 serverContent
	if geminiMsg.ServerContent != nil {
		if geminiMsg.ServerContent.TurnComplete {
			realtimeEvent.Type = dto.RealtimeEventTypeResponseDone
			realtimeEvent.Response = &dto.RealtimeResponse{
				Usage: &dto.RealtimeUsage{},
			}
			return realtimeEvent, nil
		}

		if geminiMsg.ServerContent.Interrupted {
			// 可以映射为特殊事件或忽略
			return nil, nil
		}

		// 处理输出转录（音频转文本）
		if geminiMsg.ServerContent.OutputTranscription != nil && geminiMsg.ServerContent.OutputTranscription.Text != "" {
			// 输出音频的文本转录
			event := &dto.RealtimeEvent{
				EventId: common.GetUUID(),
				Type:    "response.audio_transcript.delta",
				Delta:   geminiMsg.ServerContent.OutputTranscription.Text,
			}
			return event, nil
		}

		// 处理输入转录（用户音频转文本）
		if geminiMsg.ServerContent.InputTranscription != nil && geminiMsg.ServerContent.InputTranscription.Text != "" {
			// 输入音频的文本转录
			event := &dto.RealtimeEvent{
				EventId: common.GetUUID(),
				Type:    "conversation.item.input_audio_transcription.completed",
				Delta:   geminiMsg.ServerContent.InputTranscription.Text,
			}
			return event, nil
		}

		if geminiMsg.ServerContent.ModelTurn != nil && len(geminiMsg.ServerContent.ModelTurn.Parts) > 0 {
			// 处理所有的 parts（Gemini 可能在一个消息中返回多个 part）
			for i, part := range geminiMsg.ServerContent.ModelTurn.Parts {
				// #region agent log
				common.SysLog(fmt.Sprintf("[GeminiLive][H19][DEBUG] ModelTurn part[%d] - hasText=%t, hasInlineData=%t", i, part.Text != "", part.InlineData != nil))
				if part.InlineData != nil {
					common.SysLog(fmt.Sprintf("[GeminiLive][H19][DEBUG] InlineData - MimeType='%s', DataLen=%d", part.InlineData.MimeType, len(part.InlineData.Data)))
				}
				// #endregion

				// 优先检查音频（因为我们主要处理音频流）
				if part.InlineData != nil && strings.Contains(part.InlineData.MimeType, "audio") {
					// 音频增量
					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H19][DEBUG] Converting audio to OpenAI format, mimeType=%s, dataLen=%d", part.InlineData.MimeType, len(part.InlineData.Data)))
					// #endregion
					event := &dto.RealtimeEvent{
						EventId: common.GetUUID(),
						Type:    dto.RealtimeEventResponseAudioDelta,
						Audio:   part.InlineData.Data,
					}
					return event, nil
				}

				if part.Text != "" {
					// 文本增量
					event := &dto.RealtimeEvent{
						EventId: common.GetUUID(),
						Type:    "response.text.delta",
						Delta:   part.Text,
					}
					return event, nil
				}
			}
		}

		// 如果 serverContent 存在但没有可转换的内容，返回 nil（不是错误）
		return nil, nil
	}

	// 处理 toolCall
	if geminiMsg.ToolCall != nil {
		realtimeEvent.Type = "response.function_call_arguments.done"
		// 需要进一步映射工具调用
		return realtimeEvent, nil
	}

	// 其他未处理的消息类型，返回 nil（不是错误）
	return nil, nil
}

// MapOpenAIVoiceToGemini 映射 OpenAI 语音到 Gemini 语音
func MapOpenAIVoiceToGemini(openaiVoice string) string {
	// OpenAI voices: alloy, echo, fable, onyx, nova, shimmer
	// Gemini voices: Puck, Charon, Kore, Fenrir, Aoede
	voiceMap := map[string]string{
		"alloy":   "Puck",
		"echo":    "Charon",
		"fable":   "Kore",
		"onyx":    "Fenrir",
		"nova":    "Aoede",
		"shimmer": "Puck",
	}

	if geminiVoice, ok := voiceMap[openaiVoice]; ok {
		return geminiVoice
	}
	return "Puck" // 默认语音
}

// ConvertOpenAIToolsToGemini 转换 OpenAI 工具定义到 Gemini 格式
func ConvertOpenAIToolsToGemini(openaiTools []dto.RealTimeTool) *dto.GeminiLiveTool {
	if len(openaiTools) == 0 {
		return nil
	}

	functionDeclarations := []dto.GeminiLiveFunctionDeclaration{}
	for _, tool := range openaiTools {
		if tool.Type == "function" {
			funcDecl := dto.GeminiLiveFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
			}

			if tool.Parameters != nil {
				if params, ok := tool.Parameters.(map[string]interface{}); ok {
					funcDecl.Parameters = params
				}
			}

			functionDeclarations = append(functionDeclarations, funcDecl)
		}
	}

	if len(functionDeclarations) > 0 {
		return &dto.GeminiLiveTool{
			FunctionDeclarations: functionDeclarations,
		}
	}

	return nil
}

// ResampleAudio 音频重采样（简单的线性插值）
// 从 24kHz 转换到 16kHz 或反之
func ResampleAudio(audioData string, fromRate, toRate int) (string, error) {
	if fromRate == toRate {
		return audioData, nil
	}

	// 解码 base64
	pcmData, err := base64.StdEncoding.DecodeString(audioData)
	if err != nil {
		return "", err
	}

	// 简单的降采样/升采样（线性插值）
	ratio := float64(toRate) / float64(fromRate)
	samples := len(pcmData) / 2 // 16-bit = 2 bytes per sample
	newSamples := int(float64(samples) * ratio)

	resampled := make([]byte, newSamples*2)

	for i := 0; i < newSamples; i++ {
		srcIdx := float64(i) / ratio
		srcIdxInt := int(srcIdx)

		if srcIdxInt >= samples-1 {
			srcIdxInt = samples - 2
		}

		// 获取两个相邻样本
		sample1 := int16(pcmData[srcIdxInt*2]) | int16(pcmData[srcIdxInt*2+1])<<8
		sample2 := int16(pcmData[(srcIdxInt+1)*2]) | int16(pcmData[(srcIdxInt+1)*2+1])<<8

		// 线性插值
		frac := srcIdx - float64(srcIdxInt)
		interpolated := int16(float64(sample1)*(1-frac) + float64(sample2)*frac)

		// 写入重采样数据
		resampled[i*2] = byte(interpolated & 0xFF)
		resampled[i*2+1] = byte((interpolated >> 8) & 0xFF)
	}

	return base64.StdEncoding.EncodeToString(resampled), nil
}

// IsGeminiLiveModel 判断是否为 Gemini Live API 模型
func IsGeminiLiveModel(modelName string) bool {
	return strings.HasPrefix(modelName, "gemini-live-") ||
		strings.Contains(modelName, "native-audio")
}
