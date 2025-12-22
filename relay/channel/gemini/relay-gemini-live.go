package gemini

import (
	"fmt"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strings"
	"sync"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// GeminiLiveHandler 处理 Gemini Live API 的 WebSocket 通信
func GeminiLiveHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	// 验证 URL 参数
	if err := ValidateGeminiLiveParams(c); err != nil {
		return types.NewError(err, types.ErrorCodeInvalidRequest), nil
	}

	// 解析配置
	config := ParseGeminiLiveConfig(c)

	// #region agent log
	common.SysLog(fmt.Sprintf("[GeminiLive][DEBUG] Handler started - UpstreamModelName: %s, ChannelBaseUrl: %s, TargetWs RemoteAddr: %v", info.UpstreamModelName, info.ChannelBaseUrl, info.TargetWs.RemoteAddr()))
	common.SysLog(fmt.Sprintf("[GeminiLive][Config] Voice: %s, Language: %s, GoogleSearch: %t, OutputMode: %s",
		config.Voice, config.Language, config.GoogleSearch, config.OutputMode))
	// #endregion

	// 构建完整的模型资源路径
	// 格式: projects/{project_id}/locations/{location}/publishers/google/models/{model_name}
	fullModelPath := BuildGeminiLiveModelPath(info)

	// #region agent log
	common.SysLog(fmt.Sprintf("[GeminiLive][DEBUG] Full model path: %s", fullModelPath))
	// #endregion

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

	var protocolMode dto.ProtocolMode
	var protocolDetected bool
	var mu sync.Mutex

	// 客户端 -> 上游
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in client reader: %v", r)
			}
		}()

		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := clientConn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						common.SysLog(fmt.Sprintf("[GeminiLive][Client->Upstream] client closed: %v", err))
					} else {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				// 检测协议模式（仅第一次）
				mu.Lock()
				if !protocolDetected {
					protocolMode = DetectProtocolMode(message)
					protocolDetected = true
					logger.LogInfo(c, fmt.Sprintf("[GeminiLive] Detected protocol mode: %s", protocolMode))
				}
				currentMode := protocolMode
				mu.Unlock()

				var messageToSend []byte

				if currentMode == dto.ProtocolModeOpenAI {
					// OpenAI 格式 -> Gemini 格式
					realtimeEvent := &dto.RealtimeEvent{}
					err = common.Unmarshal(message, realtimeEvent)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error unmarshalling OpenAI event: %v", err))
						continue
					}

					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][DEBUG] OpenAI event type: %s, UpstreamModelName: %s", realtimeEvent.Type, info.UpstreamModelName))
					// #endregion

					// 转换为 Gemini Live 格式（使用完整的模型路径和配置）
					geminiMsg, err := ConvertOpenAIToGeminiLive(realtimeEvent, fullModelPath, config)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error converting to Gemini format: %v", err))
						continue
					}

					// 如果转换结果为 nil，跳过发送（某些事件不需要转发）
					if geminiMsg == nil {
						continue
					}

					// #region agent log
					geminiMsgJSON, _ := common.Marshal(geminiMsg)
					truncatedGeminiMsg := common.TruncateBase64Content(string(geminiMsgJSON))
					common.SysLog(fmt.Sprintf("[GeminiLive][H1][DEBUG] Converted Gemini message: %s", truncatedGeminiMsg))
					// #endregion

					// 统计 token（基于 OpenAI 事件）
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err == nil {
						logger.LogInfo(c, fmt.Sprintf("[GeminiLive] Input - type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
					}

					messageToSend, err = common.Marshal(geminiMsg)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error marshalling Gemini message: %v", err))
						continue
					}

					// #region agent log
					truncatedUpstreamMsg := common.TruncateBase64Content(string(messageToSend))
					common.SysLog(fmt.Sprintf("[GeminiLive][H4][DEBUG] Sending to upstream: %s", truncatedUpstreamMsg))
					// #endregion
				} else {
					// Gemini 原生格式，需要替换 setup.model 为完整路径

					geminiMsg := &dto.GeminiLiveMessage{}
					if err := common.Unmarshal(message, geminiMsg); err != nil {
						// 显示前500字符帮助调试
						rawPreview := string(message)
						if len(rawPreview) > 500 {
							rawPreview = rawPreview[:500] + "...[truncated]"
						}
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error unmarshalling Gemini message: %v, raw: %s", err, rawPreview))
						continue
					}

					// #region agent log
					if geminiMsg.ClientContent != nil {
						common.SysLog(fmt.Sprintf("[GeminiLive][H26][DEBUG] Parsed ClientContent - TurnComplete=%t, TurnsLen=%d", geminiMsg.ClientContent.TurnComplete, len(geminiMsg.ClientContent.Turns)))
					}
					if geminiMsg.RealtimeInput != nil {
						common.SysLog(fmt.Sprintf("[GeminiLive][H26][DEBUG] Parsed RealtimeInput - MediaChunksLen=%d", len(geminiMsg.RealtimeInput.MediaChunks)))
					}
					if geminiMsg.Setup == nil && geminiMsg.ClientContent == nil && geminiMsg.RealtimeInput == nil && geminiMsg.ToolResponse == nil {
						// 所有字段都是 nil，打印原始消息帮助调试
						rawPreview := string(message)
						if len(rawPreview) > 300 {
							rawPreview = rawPreview[:300] + "...[truncated]"
						}
						common.SysLog(fmt.Sprintf("[GeminiLive][H26][WARN] All fields nil after unmarshal, raw: %s", rawPreview))
					}
					// #endregion

					// 如果是 setup 消息，应用配置并替换模型路径
					if geminiMsg.Setup != nil {
						// #region agent log
						common.SysLog(fmt.Sprintf("[GeminiLive][H23][DEBUG] Original setup model: %s", geminiMsg.Setup.Model))
						// #endregion

						// 应用 URL 参数配置（如果客户端未指定）
						config.ApplyToSetup(geminiMsg.Setup)

						// 移除不支持的字段（Gemini Live API 目前不支持这些字段）
						if geminiMsg.Setup.GenerationConfig != nil {
							geminiMsg.Setup.GenerationConfig.InputContextWindowThreshold = nil
							geminiMsg.Setup.GenerationConfig.TargetContextSize = nil
						}

						// 替换为完整路径
						geminiMsg.Setup.Model = fullModelPath

						// #region agent log
						common.SysLog(fmt.Sprintf("[GeminiLive][H23][DEBUG] Applied config and replaced with full path: %s", fullModelPath))
						// #endregion
					}

					// 重新序列化
					messageToSend, err = common.Marshal(geminiMsg)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error marshalling Gemini message: %v", err))
						continue
					}

					// 统计 token（Gemini 格式）
					textTokens, audioTokens := countGeminiLiveTokens(geminiMsg, true)
					logger.LogInfo(c, fmt.Sprintf("[GeminiLive] Input - textToken: %d, audioToken: %d", textTokens, audioTokens))
					localUsage.TotalTokens += textTokens + audioTokens
					localUsage.InputTokens += textTokens + audioTokens
					localUsage.InputTokenDetails.TextTokens += textTokens
					localUsage.InputTokenDetails.AudioTokens += audioTokens

					// #region agent log
					// 添加发送日志
					truncatedMsg := common.TruncateBase64Content(string(messageToSend))
					common.SysLog(fmt.Sprintf("[GeminiLive][H24][DEBUG] Sending to upstream (Gemini native): %s", truncatedMsg))
					// #endregion
				}

				// 发送到上游
				err = helper.WssString(c, targetConn, string(messageToSend))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}
			}
		}
	})

	// 上游 -> 客户端
	gopool.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("panic in target reader: %v", r)
			}
		}()

		for {
			select {
			case <-c.Done():
				return
			default:
				_, message, err := targetConn.ReadMessage()
				if err != nil {
					if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						common.SysLog(fmt.Sprintf("[GeminiLive][Upstream->Client] upstream closed: %v", err))
					} else {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}

				// #region agent log
				truncatedMsg := common.TruncateBase64Content(string(message))
				common.SysLog(fmt.Sprintf("[GeminiLive][H5][DEBUG] Received from upstream: %s", truncatedMsg))
				// #endregion

				// 解析 Gemini Live 消息
				geminiMsg := &dto.GeminiLiveMessage{}
				err = common.Unmarshal(message, geminiMsg)
				if err != nil {
					logger.LogError(c, fmt.Sprintf("[GeminiLive] Error unmarshalling Gemini message: %v", err))
					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H22][ERROR] Raw message (first 500 chars): %s", string(message)[:min(500, len(message))]))
					// #endregion
					continue
				}

				// #region agent log
				// 检查解析后的结构
				if geminiMsg.ServerContent != nil {
					common.SysLog(fmt.Sprintf("[GeminiLive][H22][DEBUG] Parsed ServerContent - ModelTurn!=nil: %t, TurnComplete: %t",
						geminiMsg.ServerContent.ModelTurn != nil,
						geminiMsg.ServerContent.TurnComplete))
				}
				if geminiMsg.UsageMetadata != nil {
					common.SysLog(fmt.Sprintf("[GeminiLive][H27][DEBUG] UsageMetadata parsed successfully - Total: %d, Prompt: %d, Candidates: %d",
						geminiMsg.UsageMetadata.TotalTokenCount,
						geminiMsg.UsageMetadata.PromptTokenCount,
						geminiMsg.UsageMetadata.CandidatesTokenCount))
				} else {
					common.SysLog(fmt.Sprintf("[GeminiLive][H27][DEBUG] UsageMetadata is nil"))
				}
				// #endregion

				// 如果上游返回了 UsageMetadata，使用精确值而不是估算值
				if geminiMsg.UsageMetadata != nil {
					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H27][INFO] Received UsageMetadata - Prompt: %d, Candidates: %d, Total: %d",
						geminiMsg.UsageMetadata.PromptTokenCount,
						geminiMsg.UsageMetadata.CandidatesTokenCount,
						geminiMsg.UsageMetadata.TotalTokenCount))
					// #endregion

					// 解析详细的 token 计数（按模态分类）
					var promptTextTokens, promptAudioTokens, candidatesTextTokens, candidatesAudioTokens int
					for _, detail := range geminiMsg.UsageMetadata.PromptTokensDetails {
						if detail.Modality == "TEXT" {
							promptTextTokens += detail.TokenCount
						} else if detail.Modality == "AUDIO" {
							promptAudioTokens += detail.TokenCount
						}
					}
					for _, detail := range geminiMsg.UsageMetadata.CandidatesTokensDetails {
						if detail.Modality == "TEXT" {
							candidatesTextTokens += detail.TokenCount
						} else if detail.Modality == "AUDIO" {
							candidatesAudioTokens += detail.TokenCount
						}
					}

					// 更新本地使用量（当前回合）
					localUsage.InputTokens = promptTextTokens + promptAudioTokens
					localUsage.InputTokenDetails.TextTokens = promptTextTokens
					localUsage.InputTokenDetails.AudioTokens = promptAudioTokens
					localUsage.OutputTokens = candidatesTextTokens + candidatesAudioTokens
					localUsage.OutputTokenDetails.TextTokens = candidatesTextTokens
					localUsage.OutputTokenDetails.AudioTokens = candidatesAudioTokens
					localUsage.TotalTokens = geminiMsg.UsageMetadata.TotalTokenCount

					// #region agent log
					common.SysLog(fmt.Sprintf("[GeminiLive][H27][INFO] Updated localUsage - Input(text:%d, audio:%d), Output(text:%d, audio:%d), Total:%d",
						promptTextTokens, promptAudioTokens,
						candidatesTextTokens, candidatesAudioTokens,
						localUsage.TotalTokens))
					// #endregion

					// UsageMetadata 的出现表示回合已完成，立即累加到总使用量
					usage.TotalTokens += localUsage.TotalTokens
					usage.InputTokens += localUsage.InputTokens
					usage.OutputTokens += localUsage.OutputTokens
					usage.InputTokenDetails.TextTokens += localUsage.InputTokenDetails.TextTokens
					usage.InputTokenDetails.AudioTokens += localUsage.InputTokenDetails.AudioTokens
					usage.OutputTokenDetails.TextTokens += localUsage.OutputTokenDetails.TextTokens
					usage.OutputTokenDetails.AudioTokens += localUsage.OutputTokenDetails.AudioTokens

					sumUsage.TotalTokens += localUsage.TotalTokens
					sumUsage.InputTokens += localUsage.InputTokens
					sumUsage.OutputTokens += localUsage.OutputTokens
					sumUsage.InputTokenDetails.TextTokens += localUsage.InputTokenDetails.TextTokens
					sumUsage.InputTokenDetails.AudioTokens += localUsage.InputTokenDetails.AudioTokens
					sumUsage.OutputTokenDetails.TextTokens += localUsage.OutputTokenDetails.TextTokens
					sumUsage.OutputTokenDetails.AudioTokens += localUsage.OutputTokenDetails.AudioTokens

					logger.LogInfo(c, fmt.Sprintf("[GeminiLive] UsageMetadata received - accumulated sumUsage: Total=%d, Input=%d, Output=%d",
						sumUsage.TotalTokens, sumUsage.InputTokens, sumUsage.OutputTokens))

					// 预消费配额
					err := preConsumeUsageGemini(c, info, localUsage, sumUsage)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error pre-consuming quota: %v", err))
					}

					// 重置本地使用量
					localUsage = &dto.RealtimeUsage{}
				} else {
					// 没有 UsageMetadata，使用估算值（向后兼容）
					textTokens, audioTokens := countGeminiLiveTokens(geminiMsg, false)
					if textTokens > 0 || audioTokens > 0 {
						logger.LogInfo(c, fmt.Sprintf("[GeminiLive] Output - textToken: %d, audioToken: %d", textTokens, audioTokens))
						localUsage.OutputTokens += textTokens + audioTokens
						localUsage.OutputTokenDetails.TextTokens += textTokens
						localUsage.OutputTokenDetails.AudioTokens += audioTokens
					}
				}

				// 检查是否为回合完成
				if geminiMsg.ServerContent != nil && geminiMsg.ServerContent.TurnComplete {
					// 累积使用量
					usage.TotalTokens += localUsage.TotalTokens
					usage.InputTokens += localUsage.InputTokens
					usage.OutputTokens += localUsage.OutputTokens
					usage.InputTokenDetails.TextTokens += localUsage.InputTokenDetails.TextTokens
					usage.InputTokenDetails.AudioTokens += localUsage.InputTokenDetails.AudioTokens
					usage.OutputTokenDetails.TextTokens += localUsage.OutputTokenDetails.TextTokens
					usage.OutputTokenDetails.AudioTokens += localUsage.OutputTokenDetails.AudioTokens

					sumUsage.TotalTokens += localUsage.TotalTokens
					sumUsage.InputTokens += localUsage.InputTokens
					sumUsage.OutputTokens += localUsage.OutputTokens
					sumUsage.InputTokenDetails.TextTokens += localUsage.InputTokenDetails.TextTokens
					sumUsage.InputTokenDetails.AudioTokens += localUsage.InputTokenDetails.AudioTokens
					sumUsage.OutputTokenDetails.TextTokens += localUsage.OutputTokenDetails.TextTokens
					sumUsage.OutputTokenDetails.AudioTokens += localUsage.OutputTokenDetails.AudioTokens

					logger.LogInfo(c, fmt.Sprintf("[GeminiLive] Turn complete - sumUsage: %+v", sumUsage))

					// 预消费配额
					err := preConsumeUsageGemini(c, info, localUsage, sumUsage)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error pre-consuming quota: %v", err))
					}

					// 重置本地使用量
					localUsage = &dto.RealtimeUsage{}
				}

				var messageToSend []byte

				mu.Lock()
				currentMode := protocolMode
				mu.Unlock()

				if currentMode == dto.ProtocolModeOpenAI {
					// #region agent log
					// 在转换之前记录消息类型
					msgType := "unknown"
					if geminiMsg.SetupComplete != nil {
						msgType = "setupComplete"
					} else if geminiMsg.ServerContent != nil {
						hasModelTurn := geminiMsg.ServerContent.ModelTurn != nil
						hasTurnComplete := geminiMsg.ServerContent.TurnComplete

						if hasModelTurn {
							msgType = fmt.Sprintf("modelTurn(parts=%d)", len(geminiMsg.ServerContent.ModelTurn.Parts))
						}
						if hasTurnComplete {
							msgType += "+turnComplete"
						}
						if msgType == "" {
							msgType = "serverContent(empty)"
						}
					}
					common.SysLog(fmt.Sprintf("[GeminiLive][H21][DEBUG] Converting message type: %s", msgType))
					// #endregion

					// Gemini 格式 -> OpenAI 格式
					realtimeEvent, err := ConvertGeminiLiveToOpenAI(geminiMsg)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error converting to OpenAI format: %v", err))
						continue
					}

					// 如果转换结果为 nil，跳过发送（这是正常的，某些消息不需要转发）
					if realtimeEvent == nil {
						// #region agent log
						common.SysLog(fmt.Sprintf("[GeminiLive][H18][DEBUG] Skipping message (no OpenAI equivalent)"))
						// #endregion
						continue
					}

					messageToSend, err = common.Marshal(realtimeEvent)
					if err != nil {
						logger.LogError(c, fmt.Sprintf("[GeminiLive] Error marshalling OpenAI event: %v", err))
						continue
					}

					// #region agent log
					truncatedClientMsg := common.TruncateBase64Content(string(messageToSend))
					common.SysLog(fmt.Sprintf("[GeminiLive][H18][DEBUG] Sending to client: %s", truncatedClientMsg))
					// #endregion
				} else {
					// Gemini 原生格式，直接转发
					messageToSend = message
				}

				// 发送到客户端
				err = helper.WssString(c, clientConn, string(messageToSend))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				info.SendResponseCount++
			}
		}
	})

	// 等待连接关闭或错误
	select {
	case <-clientClosed:
		common.SysLog("[GeminiLive] exit select: clientClosed")
	case <-targetClosed:
		common.SysLog("[GeminiLive] exit select: targetClosed (upstream)")
	case err := <-errChan:
		logger.LogError(c, "[GeminiLive] error: "+err.Error())
		return types.NewError(err, types.ErrorCodeBadResponse), nil
	case <-c.Done():
		common.SysLog("[GeminiLive] exit select: context done")
	}

	// 最终使用量统计
	usage.TotalTokens = sumUsage.TotalTokens
	usage.InputTokens = sumUsage.InputTokens
	usage.OutputTokens = sumUsage.OutputTokens
	usage.InputTokenDetails = sumUsage.InputTokenDetails
	usage.OutputTokenDetails = sumUsage.OutputTokenDetails

	logger.LogInfo(c, fmt.Sprintf("[GeminiLive] Final usage: %+v", usage))

	return nil, usage
}

// countGeminiLiveTokens 统计 Gemini Live 消息的 token 数量
func countGeminiLiveTokens(msg *dto.GeminiLiveMessage, isInput bool) (textTokens int, audioTokens int) {
	if isInput {
		// 客户端消息
		if msg.ClientContent != nil {
			for _, turn := range msg.ClientContent.Turns {
				for _, part := range turn.Parts {
					if part.Text != "" {
						textTokens += service.CountTokenInput(part.Text, "")
					}
					if part.InlineData != nil && part.InlineData.MimeType == "audio/pcm" {
						// 估算音频 token（16kHz PCM，每秒约 100 tokens）
						audioTokens += len(part.InlineData.Data) / 320 // 粗略估算
					}
				}
			}
		}

		if msg.RealtimeInput != nil {
			for _, chunk := range msg.RealtimeInput.MediaChunks {
				if chunk.MimeType == "audio/pcm;rate=16000" {
					audioTokens += len(chunk.Data) / 320
				}
			}
		}
	} else {
		// 服务器消息
		if msg.ServerContent != nil && msg.ServerContent.ModelTurn != nil {
			for _, part := range msg.ServerContent.ModelTurn.Parts {
				if part.Text != "" {
					textTokens += service.CountTokenInput(part.Text, "")
				}
				if part.InlineData != nil && part.InlineData.MimeType == "audio/pcm" {
					// 24kHz PCM，每秒约 150 tokens
					audioTokens += len(part.InlineData.Data) / 320
				}
			}
		}
	}

	return textTokens, audioTokens
}

// preConsumeUsageGemini 预消费配额
func preConsumeUsageGemini(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage.TotalTokens == 0 {
		return nil
	}

	err := service.PreWssConsumeQuota(ctx, info, usage)
	if err != nil {
		return err
	}

	logger.LogInfo(ctx, fmt.Sprintf("[GeminiLive] Pre-consumed quota: %d tokens", usage.TotalTokens))
	return nil
}

// BuildGeminiLiveModelPath 构建完整的 Gemini Live API 模型资源路径
// 格式: projects/{project_id}/locations/{location}/publishers/google/models/{model_name}
func BuildGeminiLiveModelPath(info *relaycommon.RelayInfo) string {
	modelName := info.UpstreamModelName

	// #region agent log
	common.SysLog(fmt.Sprintf("[GeminiLive][H6][INFO] BuildGeminiLiveModelPath called with model: %s, baseUrl: %s", modelName, info.ChannelBaseUrl))
	// #endregion

	// 对于 Google AI Studio，直接使用短模型名称
	if strings.Contains(info.ChannelBaseUrl, "generativelanguage.googleapis.com") {
		// #region agent log
		common.SysLog(fmt.Sprintf("[GeminiLive][H6][INFO] Google AI Studio detected, using short model name: %s", modelName))
		// #endregion
		return modelName
	}

	// 对于 Vertex AI，虽然 Python SDK 的 connect() 方法接受短模型名称，
	// 但 SDK 内部会自动构建完整的资源路径
	// 参考 Python SDK 初始化：genai.Client(vertexai=True, project=PROJECT_ID, location=LOCATION)
	// SDK 内部将 "gemini-live-2.5-flash-native-audio" 转换为
	// "projects/{project}/locations/{location}/publishers/google/models/{model}"

	// 从 Service Account JSON 中解析 project_id
	var projectID string
	var serviceAccountData map[string]interface{}
	if err := common.Unmarshal([]byte(info.ApiKey), &serviceAccountData); err == nil {
		if pid, ok := serviceAccountData["project_id"].(string); ok {
			projectID = pid
		}
	}

	if projectID == "" {
		// #region agent log
		common.SysLog(fmt.Sprintf("[GeminiLive][H6][WARN] No project_id found in credentials, using short model name: %s", modelName))
		// #endregion
		return modelName
	}

	// H16: 现在 URL 使用区域特定域名但不包含项目路径
	// setup 消息中的 model 字段需要包含完整的资源路径

	location := "us-central1"

	// 构建完整的资源路径
	fullPath := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
		projectID, location, modelName)

	// #region agent log
	common.SysLog(fmt.Sprintf("[GeminiLive][H16][INFO] Built full model path for setup message: %s", fullPath))
	// #endregion

	return fullPath
}
