package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/relay/channel/openrouter"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"one-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

func sendStreamData(c *gin.Context, info *relaycommon.RelayInfo, data string, forceFormat bool, thinkToContent bool) error {
	if data == "" {
		return nil
	}

	type toolCallAcc struct {
		Key       string
		ID        string
		Index     *int
		Type      any
		Name      string
		Arguments string
	}

	var lastStreamResponse dto.ChatCompletionsStreamResponse
	if err := common.UnmarshalJsonStr(data, &lastStreamResponse); err != nil {
		// 如果解析失败，直接透传（可能是非 JSON 格式的数据）
		if !forceFormat && !thinkToContent {
			return helper.StringData(c, data)
		}
		return err
	}

	// 检查是否包含工具调用的增量参数（有 tool_calls 但没有 finish_reason: "tool_calls"）
	hasIncrementalToolCalls := false
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.ToolCalls) > 0 {
			// 检查是否有 finish_reason，如果没有或者是空，说明是增量参数
			if choice.FinishReason == nil || *choice.FinishReason == "" {
				hasIncrementalToolCalls = true
				break
			}
		}
	}

	// 如果有增量工具调用参数，累积但不发送
	if hasIncrementalToolCalls {
		// 从 gin.Context 获取工具调用累积状态
		toolCallsKey := "tool_calls_accumulator"
		toolCallsVal, exists := c.Get(toolCallsKey)
		var toolCallsAccumulator map[string]*toolCallAcc // map[key]*toolCallAcc

		if !exists {
			toolCallsAccumulator = make(map[string]*toolCallAcc)
		} else {
			toolCallsAccumulator, _ = toolCallsVal.(map[string]*toolCallAcc)
			if toolCallsAccumulator == nil {
				toolCallsAccumulator = make(map[string]*toolCallAcc)
			}
		}

		// 累积工具调用参数
		for _, choice := range lastStreamResponse.Choices {
			for _, toolCall := range choice.Delta.ToolCalls {
				key := ""
				// 以 index 为主键更稳定：同一个 tool_call 会被拆分成多段输出，但 index 保持一致；
				// id/name/arguments 可能分散在不同 chunk，甚至某些段里 id 为空。
				if toolCall.Index != nil {
					key = fmt.Sprintf("idx_%d", *toolCall.Index)
				} else if toolCall.ID != "" {
					key = toolCall.ID
				}
				if key == "" {
					continue
				}
				acc := toolCallsAccumulator[key]
				if acc == nil {
					acc = &toolCallAcc{Key: key}
					toolCallsAccumulator[key] = acc
				}
				// 记录基本信息（尽可能补全）
				if toolCall.ID != "" {
					acc.ID = toolCall.ID
				}
				if toolCall.Index != nil {
					acc.Index = toolCall.Index
				}
				if toolCall.Type != nil {
					acc.Type = toolCall.Type
				}
				if toolCall.Function.Name != "" {
					acc.Name = toolCall.Function.Name
				}
				if toolCall.Function.Arguments != "" {
					acc.Arguments += toolCall.Function.Arguments
				}
			}
		}

		// 保存累积状态
		c.Set(toolCallsKey, toolCallsAccumulator)

		// 不发送增量更新，返回空（等待完成事件）
		return nil
	}

	// 检查是否工具调用完成（finish_reason: "tool_calls"）
	hasCompletedToolCalls := false
	for _, choice := range lastStreamResponse.Choices {
		if choice.FinishReason != nil && *choice.FinishReason == "tool_calls" {
			hasCompletedToolCalls = true
			break
		}
	}

	// 如果工具调用完成，合并累积的参数
	if hasCompletedToolCalls {
		toolCallsKey := "tool_calls_accumulator"
		toolCallsVal, exists := c.Get(toolCallsKey)
		if exists {
			toolCallsAccumulator, ok := toolCallsVal.(map[string]*toolCallAcc)
			if ok && toolCallsAccumulator != nil {
				// 一些上游在 finish_reason: "tool_calls" 的 chunk 中不会再带 delta.tool_calls
				// 我们需要把之前累积的 tool_calls 合成并注入到当前 chunk，确保客户端能拿到完整 tool_calls。

				buildToolCallsFromAcc := func() []dto.ToolCallResponse {
					if len(toolCallsAccumulator) == 0 {
						return nil
					}
					keys := make([]string, 0, len(toolCallsAccumulator))
					for k := range toolCallsAccumulator {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					out := make([]dto.ToolCallResponse, 0, len(keys))
					for _, k := range keys {
						acc := toolCallsAccumulator[k]
						if acc == nil {
							continue
						}
						tcType := acc.Type
						if tcType == nil {
							tcType = "function"
						}
						tc := dto.ToolCallResponse{
							ID:   acc.ID,
							Type: tcType,
							Function: dto.FunctionResponse{
								Name:      acc.Name,
								Arguments: acc.Arguments,
							},
						}
						if acc.Index != nil {
							tc.SetIndex(*acc.Index)
						}
						out = append(out, tc)
					}
					return out
				}

				// 若当前 chunk 完全没有 tool_calls，则注入到第一个 choice（以及所有 choice，保持一致性）
				needInject := true
				for _, choice := range lastStreamResponse.Choices {
					if len(choice.Delta.ToolCalls) > 0 {
						needInject = false
						break
					}
				}
				if needInject {
					injected := buildToolCallsFromAcc()
					if len(injected) > 0 {
						for i := range lastStreamResponse.Choices {
							lastStreamResponse.Choices[i].Delta.ToolCalls = injected
						}
					}
				}

				// 合并累积的参数到响应中
				for i, choice := range lastStreamResponse.Choices {
					for j, toolCall := range choice.Delta.ToolCalls {
						key := ""
						if toolCall.Index != nil {
							key = fmt.Sprintf("idx_%d", *toolCall.Index)
						} else if toolCall.ID != "" {
							key = toolCall.ID
						}
						if key == "" {
							continue
						}
						if acc, ok := toolCallsAccumulator[key]; ok && acc != nil {
							// 使用累积的参数
							if acc.Arguments != "" {
								lastStreamResponse.Choices[i].Delta.ToolCalls[j].Function.Arguments = acc.Arguments
							}
							// 确保 name 也设置（如果之前没有）
							if toolCall.Function.Name == "" && acc.Name != "" {
								lastStreamResponse.Choices[i].Delta.ToolCalls[j].Function.Name = acc.Name
							}
							// 确保 type 也设置
							if lastStreamResponse.Choices[i].Delta.ToolCalls[j].Type == nil && acc.Type != nil {
								lastStreamResponse.Choices[i].Delta.ToolCalls[j].Type = acc.Type
							}
							// 确保 id 也设置（index-only 的情况）
							if lastStreamResponse.Choices[i].Delta.ToolCalls[j].ID == "" && acc.ID != "" {
								lastStreamResponse.Choices[i].Delta.ToolCalls[j].ID = acc.ID
							}
						}
					}
				}
				// 清理累积状态
				c.Set(toolCallsKey, nil)
			}
		}
	}

	if !forceFormat && !thinkToContent {
		// 序列化响应并发送
		responseBytes, err := common.Marshal(lastStreamResponse)
		if err != nil {
			return err
		}
		return helper.StringData(c, string(responseBytes))
	}

	if !thinkToContent {
		return helper.ObjectData(c, lastStreamResponse)
	}

	hasThinkingContent := false
	hasContent := false
	var thinkingContent strings.Builder
	for _, choice := range lastStreamResponse.Choices {
		if len(choice.Delta.GetReasoningContent()) > 0 {
			hasThinkingContent = true
			thinkingContent.WriteString(choice.Delta.GetReasoningContent())
		}
		if len(choice.Delta.GetContentString()) > 0 {
			hasContent = true
		}
	}

	// Handle think to content conversion
	if info.ThinkingContentInfo.IsFirstThinkingContent {
		if hasThinkingContent {
			response := lastStreamResponse.Copy()
			for i := range response.Choices {
				// send `think` tag with thinking content
				response.Choices[i].Delta.SetContentString("<think>\n" + thinkingContent.String())
				response.Choices[i].Delta.ReasoningContent = nil
				response.Choices[i].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.IsFirstThinkingContent = false
			info.ThinkingContentInfo.HasSentThinkingContent = true
			return helper.ObjectData(c, response)
		}
	}

	if len(lastStreamResponse.Choices) == 0 {
		return helper.ObjectData(c, lastStreamResponse)
	}

	// Process each choice
	for i, choice := range lastStreamResponse.Choices {
		// Handle transition from thinking to content
		// only send `</think>` tag when previous thinking content has been sent
		if hasContent && !info.ThinkingContentInfo.SendLastThinkingContent && info.ThinkingContentInfo.HasSentThinkingContent {
			response := lastStreamResponse.Copy()
			for j := range response.Choices {
				response.Choices[j].Delta.SetContentString("\n</think>\n")
				response.Choices[j].Delta.ReasoningContent = nil
				response.Choices[j].Delta.Reasoning = nil
			}
			info.ThinkingContentInfo.SendLastThinkingContent = true
			helper.ObjectData(c, response)
		}

		// Convert reasoning content to regular content if any
		if len(choice.Delta.GetReasoningContent()) > 0 {
			lastStreamResponse.Choices[i].Delta.SetContentString(choice.Delta.GetReasoningContent())
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		} else if !hasThinkingContent && !hasContent {
			// flush thinking content
			lastStreamResponse.Choices[i].Delta.ReasoningContent = nil
			lastStreamResponse.Choices[i].Delta.Reasoning = nil
		}
	}

	return helper.ObjectData(c, lastStreamResponse)
}

func OaiStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	model := info.UpstreamModelName
	var responseId string
	var createAt int64 = 0
	var systemFingerprint string
	var containStreamUsage bool
	var responseTextBuilder strings.Builder
	var toolCount int
	var usage = &dto.Usage{}
	var streamItems []string // store stream items
	var lastStreamData string

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		if lastStreamData != "" {
			err := HandleStreamFormat(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
			if err != nil {
				common.SysLog("error handling stream format: " + err.Error())
			}
		}
		if len(data) > 0 {
			lastStreamData = data
			streamItems = append(streamItems, data)
		}
		return true
	})

	// 处理最后的响应
	shouldSendLastResp := true
	if err := handleLastResponse(lastStreamData, &responseId, &createAt, &systemFingerprint, &model, &usage,
		&containStreamUsage, info, &shouldSendLastResp); err != nil {
		logger.LogError(c, fmt.Sprintf("error handling last response: %s, lastStreamData: [%s]", err.Error(), lastStreamData))
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		if shouldSendLastResp {
			_ = sendStreamData(c, info, lastStreamData, info.ChannelSetting.ForceFormat, info.ChannelSetting.ThinkingToContent)
		}
	}

	// 处理token计算
	if err := processTokens(info.RelayMode, streamItems, &responseTextBuilder, &toolCount); err != nil {
		logger.LogError(c, "error processing tokens: "+err.Error())
	}

	if !containStreamUsage {
		usage = service.ResponseText2Usage(responseTextBuilder.String(), info.UpstreamModelName, info.PromptTokens)
		usage.CompletionTokens += toolCount * 7
	}

	applyUsagePostProcessing(info, usage, nil)

	HandleFinalResponse(c, info, lastStreamData, responseId, createAt, model, systemFingerprint, usage, containStreamUsage)

	return usage, nil
}

func OpenaiHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	var simpleResponse dto.OpenAITextResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	if common.DebugEnabled {
		println("upstream response body:", string(responseBody))
	}
	// Unmarshal to simpleResponse
	if info.ChannelType == constant.ChannelTypeOpenRouter && info.ChannelOtherSettings.IsOpenRouterEnterprise() {
		// 尝试解析为 openrouter enterprise
		var enterpriseResponse openrouter.OpenRouterEnterpriseResponse
		err = common.Unmarshal(responseBody, &enterpriseResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if enterpriseResponse.Success {
			responseBody = enterpriseResponse.Data
		} else {
			logger.LogError(c, fmt.Sprintf("openrouter enterprise response success=false, data: %s", enterpriseResponse.Data))
			return nil, types.NewOpenAIError(fmt.Errorf("openrouter response success=false"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	err = common.Unmarshal(responseBody, &simpleResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := simpleResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// openai/ 前缀的模型响应转换在 relay_responses.go 的 OaiResponsesHandler 中处理

	forceFormat := false
	if info.ChannelSetting.ForceFormat {
		forceFormat = true
	}

	usageModified := false
	if simpleResponse.Usage.PromptTokens == 0 {
		completionTokens := simpleResponse.Usage.CompletionTokens
		if completionTokens == 0 {
			for _, choice := range simpleResponse.Choices {
				ctkm := service.CountTextToken(choice.Message.StringContent()+choice.Message.ReasoningContent+choice.Message.Reasoning, info.UpstreamModelName)
				completionTokens += ctkm
			}
		}
		simpleResponse.Usage = dto.Usage{
			PromptTokens:     info.PromptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      info.PromptTokens + completionTokens,
		}
		usageModified = true
	}

	applyUsagePostProcessing(info, &simpleResponse.Usage, responseBody)

	switch info.RelayFormat {
	case types.RelayFormatOpenAI:
		if usageModified {
			var bodyMap map[string]interface{}
			err = common.Unmarshal(responseBody, &bodyMap)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
			bodyMap["usage"] = simpleResponse.Usage
			responseBody, _ = common.Marshal(bodyMap)
		}
		if forceFormat {
			responseBody, err = common.Marshal(simpleResponse)
			if err != nil {
				return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
			}
		} else {
			break
		}
	case types.RelayFormatClaude:
		claudeResp := service.ResponseOpenAI2Claude(&simpleResponse, info)
		claudeRespStr, err := common.Marshal(claudeResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = claudeRespStr
	case types.RelayFormatGemini:
		geminiResp := service.ResponseOpenAI2Gemini(&simpleResponse, info)
		geminiRespStr, err := common.Marshal(geminiResp)
		if err != nil {
			return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
		}
		responseBody = geminiRespStr
	}

	// 注意：Cursor 调用 /v1/chat/completions 时，期望标准的 OpenAI Chat Completions 响应格式
	// 不需要转换为 Cursor 格式，直接使用标准格式即可
	// Cursor 格式转换仅用于 Cursor 自己的 /v1/responses 接口

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &simpleResponse.Usage, nil
}

func OpenaiTTSHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) *dto.Usage {
	// the status code has been judged before, if there is a body reading failure,
	// it should be regarded as a non-recoverable error, so it should not return err for external retry.
	// Analogous to nginx's load balancing, it will only retry if it can't be requested or
	// if the upstream returns a specific status code, once the upstream has already written the header,
	// the subsequent failure of the response body should be regarded as a non-recoverable error,
	// and can be terminated directly.
	defer service.CloseResponseBodyGracefully(resp)
	usage := &dto.Usage{}
	usage.PromptTokens = info.PromptTokens
	usage.TotalTokens = info.PromptTokens
	for k, v := range resp.Header {
		c.Writer.Header().Set(k, v[0])
	}
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	_, err := io.Copy(c.Writer, resp.Body)
	if err != nil {
		logger.LogError(c, err.Error())
	}
	return usage
}

func OpenaiSTTHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo, responseFormat string) (*types.NewAPIError, *dto.Usage) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)

	var responseData struct {
		Usage *dto.Usage `json:"usage"`
	}
	if err := json.Unmarshal(responseBody, &responseData); err == nil && responseData.Usage != nil {
		if responseData.Usage.TotalTokens > 0 {
			usage := responseData.Usage
			if usage.PromptTokens == 0 {
				usage.PromptTokens = usage.InputTokens
			}
			if usage.CompletionTokens == 0 {
				usage.CompletionTokens = usage.OutputTokens
			}
			return nil, usage
		}
	}

	audioTokens, err := countAudioTokens(c)
	if err != nil {
		return types.NewError(err, types.ErrorCodeCountTokenFailed), nil
	}
	usage := &dto.Usage{}
	usage.PromptTokens = audioTokens
	usage.CompletionTokens = 0
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return nil, usage
}

func countAudioTokens(c *gin.Context) (int, error) {
	body, err := common.GetRequestBody(c)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	var reqBody struct {
		File *multipart.FileHeader `form:"file" binding:"required"`
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err = c.ShouldBind(&reqBody); err != nil {
		return 0, errors.WithStack(err)
	}
	ext := filepath.Ext(reqBody.File.Filename) // 获取文件扩展名
	reqFp, err := reqBody.File.Open()
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer reqFp.Close()

	tmpFp, err := os.CreateTemp("", "audio-*"+ext)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	defer os.Remove(tmpFp.Name())

	_, err = io.Copy(tmpFp, reqFp)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	if err = tmpFp.Close(); err != nil {
		return 0, errors.WithStack(err)
	}

	duration, err := common.GetAudioDuration(c.Request.Context(), tmpFp.Name(), ext)
	if err != nil {
		return 0, errors.WithStack(err)
	}

	return int(math.Round(math.Ceil(duration) / 60.0 * 1000)), nil // 1 minute 相当于 1k tokens
}

func OpenaiRealtimeHandler(c *gin.Context, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.RealtimeUsage) {
	if info == nil || info.ClientWs == nil || info.TargetWs == nil {
		return types.NewError(fmt.Errorf("invalid websocket connection"), types.ErrorCodeBadResponse), nil
	}

	info.IsStream = true
	clientConn := info.ClientWs
	targetConn := info.TargetWs

	clientClosed := make(chan struct{})
	targetClosed := make(chan struct{})
	sendChan := make(chan []byte, 100)
	receiveChan := make(chan []byte, 100)
	errChan := make(chan error, 2)

	usage := &dto.RealtimeUsage{}
	localUsage := &dto.RealtimeUsage{}
	sumUsage := &dto.RealtimeUsage{}

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
						common.SysLog(fmt.Sprintf("[Realtime][Client->Upstream] client closed: %v", err))
					} else {
						errChan <- fmt.Errorf("error reading from client: %v", err)
					}
					close(clientClosed)
					return
				}

				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdate {
					if realtimeEvent.Session != nil {
						if realtimeEvent.Session.Tools != nil {
							info.RealtimeTools = realtimeEvent.Session.Tools
						}
					}
				}

				textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
				if err != nil {
					errChan <- fmt.Errorf("error counting text token: %v", err)
					return
				}
				logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
				localUsage.TotalTokens += textToken + audioToken
				localUsage.InputTokens += textToken + audioToken
				localUsage.InputTokenDetails.TextTokens += textToken
				localUsage.InputTokenDetails.AudioTokens += audioToken

				err = helper.WssString(c, targetConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to target: %v", err)
					return
				}

				select {
				case sendChan <- message:
				default:
				}
			}
		}
	})

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
						common.SysLog(fmt.Sprintf("[Realtime][Upstream->Client] upstream closed: %v", err))
					} else {
						errChan <- fmt.Errorf("error reading from target: %v", err)
					}
					close(targetClosed)
					return
				}
				info.SetFirstResponseTime()
				realtimeEvent := &dto.RealtimeEvent{}
				err = common.Unmarshal(message, realtimeEvent)
				if err != nil {
					errChan <- fmt.Errorf("error unmarshalling message: %v", err)
					return
				}

				if realtimeEvent.Type == dto.RealtimeEventTypeResponseDone {
					realtimeUsage := realtimeEvent.Response.Usage
					if realtimeUsage != nil {
						usage.TotalTokens += realtimeUsage.TotalTokens
						usage.InputTokens += realtimeUsage.InputTokens
						usage.OutputTokens += realtimeUsage.OutputTokens
						usage.InputTokenDetails.AudioTokens += realtimeUsage.InputTokenDetails.AudioTokens
						usage.InputTokenDetails.CachedTokens += realtimeUsage.InputTokenDetails.CachedTokens
						usage.InputTokenDetails.TextTokens += realtimeUsage.InputTokenDetails.TextTokens
						usage.OutputTokenDetails.AudioTokens += realtimeUsage.OutputTokenDetails.AudioTokens
						usage.OutputTokenDetails.TextTokens += realtimeUsage.OutputTokenDetails.TextTokens
						err := preConsumeUsage(c, info, usage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						usage = &dto.RealtimeUsage{}

						localUsage = &dto.RealtimeUsage{}
					} else {
						textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
						if err != nil {
							errChan <- fmt.Errorf("error counting text token: %v", err)
							return
						}
						logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
						localUsage.TotalTokens += textToken + audioToken
						info.IsFirstRequest = false
						localUsage.InputTokens += textToken + audioToken
						localUsage.InputTokenDetails.TextTokens += textToken
						localUsage.InputTokenDetails.AudioTokens += audioToken
						err = preConsumeUsage(c, info, localUsage, sumUsage)
						if err != nil {
							errChan <- fmt.Errorf("error consume usage: %v", err)
							return
						}
						// 本次计费完成，清除
						localUsage = &dto.RealtimeUsage{}
						// print now usage
					}
					logger.LogInfo(c, fmt.Sprintf("realtime streaming sumUsage: %v", sumUsage))
					logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))
					logger.LogInfo(c, fmt.Sprintf("realtime streaming localUsage: %v", localUsage))

				} else if realtimeEvent.Type == dto.RealtimeEventTypeSessionUpdated || realtimeEvent.Type == dto.RealtimeEventTypeSessionCreated {
					realtimeSession := realtimeEvent.Session
					if realtimeSession != nil {
						// update audio format
						info.InputAudioFormat = common.GetStringIfEmpty(realtimeSession.InputAudioFormat, info.InputAudioFormat)
						info.OutputAudioFormat = common.GetStringIfEmpty(realtimeSession.OutputAudioFormat, info.OutputAudioFormat)
					}
				} else {
					textToken, audioToken, err := service.CountTokenRealtime(info, *realtimeEvent, info.UpstreamModelName)
					if err != nil {
						errChan <- fmt.Errorf("error counting text token: %v", err)
						return
					}
					logger.LogInfo(c, fmt.Sprintf("type: %s, textToken: %d, audioToken: %d", realtimeEvent.Type, textToken, audioToken))
					localUsage.TotalTokens += textToken + audioToken
					localUsage.OutputTokens += textToken + audioToken
					localUsage.OutputTokenDetails.TextTokens += textToken
					localUsage.OutputTokenDetails.AudioTokens += audioToken
				}

				err = helper.WssString(c, clientConn, string(message))
				if err != nil {
					errChan <- fmt.Errorf("error writing to client: %v", err)
					return
				}

				select {
				case receiveChan <- message:
				default:
				}
			}
		}
	})

	select {
	case <-clientClosed:
		common.SysLog("[Realtime] exit select: clientClosed")
	case <-targetClosed:
		common.SysLog("[Realtime] exit select: targetClosed (upstream)")
	case err := <-errChan:
		//return service.OpenAIErrorWrapper(err, "realtime_error", http.StatusInternalServerError), nil
		logger.LogError(c, "realtime error: "+err.Error())
	case <-c.Done():
		common.SysLog("[Realtime] exit select: context done")
	}

	if usage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, usage, sumUsage)
	}

	if localUsage.TotalTokens != 0 {
		_ = preConsumeUsage(c, info, localUsage, sumUsage)
	}

	// check usage total tokens, if 0, use local usage

	return nil, sumUsage
}

func preConsumeUsage(ctx *gin.Context, info *relaycommon.RelayInfo, usage *dto.RealtimeUsage, totalUsage *dto.RealtimeUsage) error {
	if usage == nil || totalUsage == nil {
		return fmt.Errorf("invalid usage pointer")
	}

	totalUsage.TotalTokens += usage.TotalTokens
	totalUsage.InputTokens += usage.InputTokens
	totalUsage.OutputTokens += usage.OutputTokens
	totalUsage.InputTokenDetails.CachedTokens += usage.InputTokenDetails.CachedTokens
	totalUsage.InputTokenDetails.TextTokens += usage.InputTokenDetails.TextTokens
	totalUsage.InputTokenDetails.AudioTokens += usage.InputTokenDetails.AudioTokens
	totalUsage.OutputTokenDetails.TextTokens += usage.OutputTokenDetails.TextTokens
	totalUsage.OutputTokenDetails.AudioTokens += usage.OutputTokenDetails.AudioTokens
	// clear usage
	err := service.PreWssConsumeQuota(ctx, info, usage)
	return err
}

func OpenaiHandlerWithUsage(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	// 打印响应体（base64 截断）
	if common.DebugEnabled {
		truncated := common.TruncateBase64Content(string(responseBody))
		logger.LogDebug(c, fmt.Sprintf("[OpenaiHandlerWithUsage] 响应体: %s", truncated))
	}

	var usageResp dto.SimpleResponse
	err = common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 暂不写入，等 usage 合并后再决定是否注入 usage 到图片响应
	writeBody := responseBody

	// Once we've written to the client, we should not return errors anymore
	// because the upstream has already consumed resources and returned content
	// We should still perform billing even if parsing fails
	// format

	// 添加调试日志：打印解析后的响应数据
	if common.DebugEnabled {
		logger.LogDebug(c, fmt.Sprintf("[OpenaiHandlerWithUsage] 解析后的响应: InputTokens=%d, OutputTokens=%d, CompletionTokens=%d, PromptTokens=%d",
			usageResp.InputTokens, usageResp.OutputTokens, usageResp.CompletionTokens, usageResp.PromptTokens))
		logger.LogDebug(c, fmt.Sprintf("[OpenaiHandlerWithUsage] 模型名称: UpstreamModelName=%s, OriginModelName=%s",
			info.UpstreamModelName, info.OriginModelName))
	}

	// 记录合并前的值
	originalCompletionTokens := usageResp.CompletionTokens
	originalOutputTokens := usageResp.OutputTokens

	if usageResp.InputTokens > 0 {
		usageResp.PromptTokens += usageResp.InputTokens
	}
	if usageResp.OutputTokens > 0 {
		usageResp.CompletionTokens += usageResp.OutputTokens
	}

	// 添加调试日志：打印合并后的值
	if common.DebugEnabled {
		logger.LogDebug(c, fmt.Sprintf("[OpenaiHandlerWithUsage] 合并后的值: CompletionTokens=%d (原值=%d + OutputTokens=%d)",
			usageResp.CompletionTokens, originalCompletionTokens, originalOutputTokens))
	}
	if usageResp.InputTokensDetails != nil {
		usageResp.PromptTokensDetails.ImageTokens += usageResp.InputTokensDetails.ImageTokens
		usageResp.PromptTokensDetails.TextTokens += usageResp.InputTokensDetails.TextTokens
	}

	// 对于 gpt-image-1，特殊处理 tokens 提取
	if strings.HasPrefix(info.UpstreamModelName, "gpt-image-1") {
		if common.DebugEnabled {
			logger.LogDebug(c, "[OpenaiHandlerWithUsage] 检测到 gpt-image-1 模型，开始特殊处理")
		}

		// 如果没有 InputTokensDetails，需要估算
		if usageResp.InputTokensDetails == nil && usageResp.PromptTokens > 0 {
			// 估算：假设文本 tokens 为 80，其余为图像 tokens
			estimatedTextTokens := 80
			estimatedImageTokens := usageResp.PromptTokens - estimatedTextTokens
			if estimatedImageTokens > 0 {
				if usageResp.PromptTokensDetails.ImageTokens == 0 {
					usageResp.PromptTokensDetails.ImageTokens = estimatedImageTokens
				}
				if usageResp.PromptTokensDetails.TextTokens == 0 {
					usageResp.PromptTokensDetails.TextTokens = estimatedTextTokens
				}
			}
			if common.DebugEnabled {
				logger.LogDebug(c, fmt.Sprintf("[OpenaiHandlerWithUsage] gpt-image-1 输入tokens估算: TextTokens=%d, ImageTokens=%d",
					estimatedTextTokens, estimatedImageTokens))
			}
		}

		// 对于 gpt-image-1，completionTokens 就是图像输出 tokens
		if usageResp.CompletionTokens > 0 {
			// 设置到 context，供计费逻辑使用
			c.Set("gpt_image_output_tokens", usageResp.CompletionTokens)

			// 验证设置是否成功
			if setValue, exists := c.Get("gpt_image_output_tokens"); exists {
				if common.DebugEnabled {
					logger.LogDebug(c, fmt.Sprintf("[OpenaiHandlerWithUsage] gpt-image-1 输出图像 tokens: %d, 已设置到context: %v",
						usageResp.CompletionTokens, setValue))
				}
			} else {
				logger.LogError(c, fmt.Sprintf("[OpenaiHandlerWithUsage] gpt-image-1 设置 gpt_image_output_tokens 失败! CompletionTokens=%d",
					usageResp.CompletionTokens))
			}
		} else {
			if common.DebugEnabled {
				logger.LogDebug(c, "[OpenaiHandlerWithUsage] gpt-image-1 CompletionTokens 为 0，未设置 gpt_image_output_tokens")
			}
		}
	} else {
		if common.DebugEnabled {
			logger.LogDebug(c, "[OpenaiHandlerWithUsage] 非 gpt-image-1 模型，跳过特殊处理")
		}
	}

	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)

	// 图片生成/编辑：向响应体注入 usage（与对话接口一致的 tokens 数）
	if info.RelayMode == relayconstant.RelayModeImagesGenerations || info.RelayMode == relayconstant.RelayModeImagesEdits {
		var imageResp dto.ImageResponse
		if err := common.Unmarshal(responseBody, &imageResp); err == nil && imageResp.Data != nil {
			imageResp.Usage = &usageResp.Usage
			if injected, err := common.Marshal(&imageResp); err == nil {
				writeBody = injected
			}
		}
	}
	service.IOCopyBytesGracefully(c, resp, writeBody)
	return &usageResp.Usage, nil
}

func applyUsagePostProcessing(info *relaycommon.RelayInfo, usage *dto.Usage, responseBody []byte) {
	if info == nil || usage == nil {
		return
	}

	switch info.ChannelType {
	case constant.ChannelTypeDeepSeek:
		if usage.PromptTokensDetails.CachedTokens == 0 && usage.PromptCacheHitTokens != 0 {
			usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
		}
	case constant.ChannelTypeZhipu_v4:
		if usage.PromptTokensDetails.CachedTokens == 0 {
			if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.InputTokensDetails.CachedTokens
			} else if cachedTokens, ok := extractCachedTokensFromBody(responseBody); ok {
				usage.PromptTokensDetails.CachedTokens = cachedTokens
			} else if usage.PromptCacheHitTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = usage.PromptCacheHitTokens
			}
		}
	}
	// 处理阿里云通义千问缓存
	if info.ChannelType == constant.ChannelTypeAli {
		// 从响应体中提取缓存使用情况
		var aliResponse struct {
			Usage struct {
				PromptTokensDetails *struct {
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
					CachedTokens             int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
			} `json:"usage"`
		}

		if err := common.Unmarshal(responseBody, &aliResponse); err == nil && aliResponse.Usage.PromptTokensDetails != nil {
			// 设置缓存token详情
			if aliResponse.Usage.PromptTokensDetails.CacheCreationInputTokens > 0 {
				usage.PromptTokensDetails.CachedCreationTokens = aliResponse.Usage.PromptTokensDetails.CacheCreationInputTokens
			}
			if aliResponse.Usage.PromptTokensDetails.CachedTokens > 0 {
				usage.PromptTokensDetails.CachedTokens = aliResponse.Usage.PromptTokensDetails.CachedTokens
			}
		}
	}
}

func extractCachedTokensFromBody(body []byte) (int, bool) {
	if len(body) == 0 {
		return 0, false
	}

	var payload struct {
		Usage struct {
			PromptTokensDetails struct {
				CachedTokens *int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
			CachedTokens         *int `json:"cached_tokens"`
			PromptCacheHitTokens *int `json:"prompt_cache_hit_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, false
	}

	if payload.Usage.PromptTokensDetails.CachedTokens != nil {
		return *payload.Usage.PromptTokensDetails.CachedTokens, true
	}
	if payload.Usage.CachedTokens != nil {
		return *payload.Usage.CachedTokens, true
	}
	if payload.Usage.PromptCacheHitTokens != nil {
		return *payload.Usage.PromptCacheHitTokens, true
	}
	return 0, false
}
