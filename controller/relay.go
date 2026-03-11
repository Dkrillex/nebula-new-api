package controller

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/middleware"
	"one-api/model"
	"one-api/relay"
	relaycommon "one-api/relay/common"
	relayconstant "one-api/relay/constant"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting"
	"one-api/types"
	"strings"
	"time"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeFiles:
		err = relay.FileHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	// 在函数开始就读取请求体，确保无论在哪里出错都能重复读取
	requestBody, _ := common.GetRequestBody(c)
	// 记录“用户调用接口的原始入参”（非透传与否都只记录这一份原始入参）
	// 注意：使用 SysLog 才会写入 log 文件（/logs），println 不会。
	if common.DebugEnabled {
		truncated := common.TruncateJsonValues(string(requestBody))
		common.SysLog(fmt.Sprintf("[Relay][UserRequest] %s %s | body=%s", c.Request.Method, c.Request.URL.Path, truncated))
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime || relayFormat == types.RelayFormatGeminiLive {
		// 检查 WebSocket 升级请求头
		connection := c.Request.Header.Get("Connection")
		upgrade := c.Request.Header.Get("Upgrade")
		common.SysLog(fmt.Sprintf("[Relay] WebSocket upgrade check: Connection=%s, Upgrade=%s", connection, upgrade))

		// 如果 Connection 头被代理修改为 close，尝试修复
		if strings.ToLower(connection) == "close" {
			common.SysLog(fmt.Sprintf("[Relay] Connection header was modified to 'close', fixing to 'Upgrade'"))
			c.Request.Header.Set("Connection", "Upgrade")
		}
		// 确保 Upgrade 头存在
		if upgrade == "" {
			c.Request.Header.Set("Upgrade", "websocket")
		}

		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			common.SysLog(fmt.Sprintf("[Relay] WebSocket upgrade failed: %v, Connection=%s, Upgrade=%s, Sec-WebSocket-Key=%s, Sec-WebSocket-Version=%s",
				err, c.Request.Header.Get("Connection"), c.Request.Header.Get("Upgrade"),
				c.Request.Header.Get("Sec-WebSocket-Key"), c.Request.Header.Get("Sec-WebSocket-Version")))
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			newAPIError.SetMessage(common.MessageWithRequestId(newAPIError.Error(), requestId))
			var errorResponse interface{}
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
				return
			case types.RelayFormatGeminiLive:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
				return
			case types.RelayFormatClaude:
				claudeError := newAPIError.ToClaudeError()
				errorResponse = gin.H{
					"type": "error",
					"error": gin.H{
						"type":    claudeError.Type,
						"message": claudeError.Message,
					},
					"request_id": c.GetString(common.RequestIdKey),
				}
			default:
				openAIErr := newAPIError.ToOpenAIError()
				errorResponse = gin.H{
					"error": openAIErr,
				}
				// 记录实际返回的错误响应内容，用于调试
				if newAPIError.StatusCode >= 400 {
					errorJson, _ := json.Marshal(errorResponse)
					logger.LogError(c, fmt.Sprintf("[Relay] 返回错误响应 (status: %d): %s", newAPIError.StatusCode, string(errorJson)))
				}
			}
			c.JSON(newAPIError.StatusCode, errorResponse)
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		return
	}

	// STEP 4: 检查如果是 Responses 格式的请求，需要特殊处理
	if _, ok := request.(*dto.OpenAIResponsesRequest); ok {
		// 检查是否需要响应转换（无论是 openai/ 前缀还是其他模型，只要请求体是 Responses 格式，都需要转换）
		if c.GetBool("is_cursor") && c.GetBool("convert_cursor_to_chat") {
			// 请求体是 Responses 格式：请求直接使用 Responses 格式发送到上游的 /v1/responses 接口
			// 切换到 Responses 格式，调用上游的 /v1/responses 接口
			relayFormat = types.RelayFormatOpenAIResponses
			c.Set("is_cursor", true)
			c.Set("convert_responses_to_chat", true) // 标记需要将响应从 Responses 转换为 Chat Completions
		} else {
			// 检查请求体是否包含 messages 字段（标准的 Chat Completions 格式）
			requestBody, _ := common.GetRequestBody(c)
			var bodyCheck struct {
				Model    string        `json:"model"`
				Messages []interface{} `json:"messages"`
			}
			if err := common.Unmarshal(requestBody, &bodyCheck); err == nil && len(bodyCheck.Messages) > 0 {
				// 请求包含 messages 字段，说明是标准的 Chat Completions 格式
				// 检查是否是 openai/ 前缀的模型
				if strings.HasPrefix(bodyCheck.Model, "openai/") {
					// openai/ 前缀的模型，但请求体是标准的 Chat Completions 格式
					// 应该使用 Chat Completions 格式，而不是 Responses 格式
					// 重新解析为标准的 Chat Completions 格式
					textRequest, err := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
					if err == nil {
						request = textRequest
						// 保持 Chat Completions 格式
						relayFormat = types.RelayFormatOpenAI
					} else {
						// 解析失败，使用 Responses 格式（保持原有逻辑）
						relayFormat = types.RelayFormatOpenAIResponses
					}
				} else {
					// 非 openai/ 前缀的模型，但请求被解析为 Responses 格式
					// 重新解析为标准的 Chat Completions 格式
					textRequest, err := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
					if err == nil {
						request = textRequest
						// 保持 Chat Completions 格式
						relayFormat = types.RelayFormatOpenAI
					} else {
						// 解析失败，使用 Responses 格式（保持原有逻辑）
						relayFormat = types.RelayFormatOpenAIResponses
					}
				}
			} else {
				// 请求不包含 messages 字段，使用 Responses 格式（保持原有逻辑）
				relayFormat = types.RelayFormatOpenAIResponses
			}
		}
		// 更新 originalModel 变量（如果 context 中已更新）
		if updatedModel := c.GetString("original_model"); updatedModel != "" {
			originalModel = updatedModel
		}
	} else {
		// 更新 originalModel 变量（如果 context 中已更新）
		if updatedModel := c.GetString("original_model"); updatedModel != "" {
			originalModel = updatedModel
		}
	}

	// 在转发前自动处理 chat 文件上传与消息改写
	if relayFormat == types.RelayFormatOpenAI {
		if chatReq, ok := request.(*dto.GeneralOpenAIRequest); ok {
			updatedReq, uploadErr := helper.AutoUploadChatFiles(c, relayconstant.Path2RelayMode(c.Request.URL.Path), chatReq)
			if uploadErr != nil {
				newAPIError = uploadErr
				return
			}
			request = updatedReq
		}
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	meta := request.GetTokenCountMeta()

	if setting.ShouldCheckPromptSensitive() {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.CountRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetPromptTokens(tokens)

	// 实时对话不进行预扣费，使用后扣费（通过 PostWssConsumeQuota）
	if relayFormat != types.RelayFormatOpenAIRealtime && relayFormat != types.RelayFormatGeminiLive {
		priceData, err := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
		if err != nil {
			newAPIError = types.NewError(err, types.ErrorCodeModelPriceError)
			return
		}

		// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

		newAPIError = service.PreConsumeQuota(c, priceData.ShouldPreConsumedQuota, relayInfo)
		if newAPIError != nil {
			return
		}

		defer func() {
			// Only return quota if downstream failed and quota was actually pre-consumed
			if newAPIError != nil && relayInfo.FinalPreConsumedQuota != 0 {
				service.ReturnPreConsumedQuota(c, relayInfo)
			}
		}()
	}

	for i := 0; i <= common.RetryTimes; i++ {
		channel, err := getChannel(c, group, originalModel, i)
		if err != nil {
			logger.LogError(c, err.Error())
			newAPIError = err
			break
		}

		addUsedChannel(c, channel.Id)
		// 请求体已在函数开始处打印，这里只需要重新设置 body
		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatGeminiLive:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			return
		}

		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)

		if !shouldRetry(c, newAPIError, common.RetryTimes-i) {
			break
		}

		// 图片生成重试前等待 3 秒
		if relayInfo.RelayMode == relayconstant.RelayModeImagesGenerations ||
			relayInfo.RelayMode == relayconstant.RelayModeImagesEdits {
			logger.LogInfo(c, "[图片重试] 开始等待 3 秒，等待结束后将切换渠道重试")
			time.Sleep(3 * time.Second)
			logger.LogInfo(c, "[图片重试] 等待 3 秒结束，开始下一轮请求")
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func getChannel(c *gin.Context, group, originalModel string, retryCount int) (*model.Channel, *types.NewAPIError) {
	if retryCount == 0 {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := model.CacheGetRandomSatisfiedChannel(c, group, originalModel, retryCount)
	if err != nil {
		return nil, types.NewError(fmt.Errorf("获取分组 %s 下模型 %s 的可用渠道失败（retry）: %s", selectGroup, originalModel, err.Error()), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	if channel == nil {
		return nil, types.NewError(fmt.Errorf("分组 %s 下模型 %s 的可用渠道不存在（数据库一致性已被破坏，retry）", selectGroup, originalModel), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, originalModel)
	if newAPIError != nil {
		return nil, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if openaiErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if openaiErr.StatusCode == 307 {
		return true
	}
	if openaiErr.StatusCode/100 == 5 {
		// 超时不重试
		if openaiErr.StatusCode == 504 || openaiErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if openaiErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if openaiErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if openaiErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError) {
	logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, err.Error()))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(channelError.ChannelId, err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.Error())
		})
	}

	// 429（Too Many Requests）不记录错误日志，避免大规模限流刷爆错误日志表
	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) && err.StatusCode != http.StatusTooManyRequests {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		userGroup := c.GetString("group")
		channelId := c.GetInt("channel_id")
		other := make(map[string]interface{})
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = c.GetString("channel_name")
		other["channel_type"] = c.GetInt("channel_type")
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		other["admin_info"] = adminInfo
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, err.MaskSensitiveError(), tokenId, 0, false, userGroup, other)
	}

}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "当前分组负载已饱和，请稍后再试，或升级账户以提升服务质量。"
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := dto.OpenAIError{
		Message: "API not implemented",
		Type:    "nebula_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := dto.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTask(c *gin.Context) {
	// 记录“用户调用接口的原始入参”（主要覆盖 /v1/video/generations 等任务接口）
	if common.DebugEnabled {
		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
	}

	retryTimes := common.RetryTimes
	channelId := c.GetInt("channel_id")
	group := c.GetString("group")
	originalModel := c.GetString("original_model")

	c.Set("use_channel", []string{fmt.Sprintf("%d", channelId)})
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		common.SysError(fmt.Sprintf("[RelayTask] GenRelayInfo失败: %v", err))
		return
	}
	taskErr := taskRelayHandler(c, relayInfo)
	if taskErr == nil {
		retryTimes = 0
	}
	for i := 0; shouldRetryTaskRelay(c, channelId, taskErr, retryTimes) && i < retryTimes; i++ {
		channel, newAPIError := getChannel(c, group, originalModel, i)
		if newAPIError != nil {
			logger.LogError(c, fmt.Sprintf("CacheGetRandomSatisfiedChannel failed: %s", newAPIError.Error()))
			taskErr = service.TaskErrorWrapperLocal(newAPIError.Err, "get_channel_failed", http.StatusInternalServerError)
			break
		}
		channelId = channel.Id
		useChannel := c.GetStringSlice("use_channel")
		useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
		c.Set("use_channel", useChannel)
		logger.LogInfo(c, fmt.Sprintf("using channel #%d to retry (remain times %d)", channel.Id, i))
		//middleware.SetupContextForSelectedChannel(c, channel, originalModel)

		requestBody, _ := common.GetRequestBody(c)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))
		taskErr = taskRelayHandler(c, relayInfo)
	}
	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if taskErr != nil {
		if taskErr.StatusCode == http.StatusTooManyRequests {
			taskErr.Message = "当前分组上游负载已饱和，请稍后再试"
		}
		c.JSON(taskErr.StatusCode, taskErr)
	}
}

func taskRelayHandler(c *gin.Context, relayInfo *relaycommon.RelayInfo) *dto.TaskError {
	var err *dto.TaskError
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeSunoFetch, relayconstant.RelayModeSunoFetchByID, relayconstant.RelayModeVideoFetchByID:
		//common.SysLog(fmt.Sprintf("[taskRelayHandler] 命中任务查询模式, RelayMode: %d", relayInfo.RelayMode))
		err = relay.RelayTaskFetch(c, relayInfo.RelayMode)
	default:
		//common.SysLog(fmt.Sprintf("[taskRelayHandler] 命中任务提交模式, RelayMode: %d", relayInfo.RelayMode))
		err = relay.RelayTaskSubmit(c, relayInfo)
	}
	if err != nil {
		common.SysError(fmt.Sprintf("[taskRelayHandler] 处理失败: %+v", err))
	}
	return err
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if taskErr.StatusCode == 504 || taskErr.StatusCode == 524 {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

// formatRequestHeadersForLog 格式化请求头用于日志打印
func formatRequestHeadersForLog(headers http.Header) string {
	if len(headers) == 0 {
		return "(empty)"
	}

	headerStrs := make([]string, 0, len(headers))
	for key, values := range headers {
		headerStrs = append(headerStrs, fmt.Sprintf("%s: %v", key, values))
	}
	return strings.Join(headerStrs, ", ")
}

// formatRequestBodyForLog 格式化请求体用于日志打印
// 如果是JSON，既显示结构（字段名和类型），也显示实际数据内容
// 返回结构信息和数据内容，分别打印在不同行
func formatRequestBodyForLog(requestBody []byte) (string, string) {
	bodyStr := string(requestBody)

	// 如果内容为空，直接返回
	if len(bodyStr) == 0 {
		return "(empty)", "(empty)"
	}

	// 尝试解析为JSON对象
	var jsonObj map[string]interface{}
	if err := json.Unmarshal(requestBody, &jsonObj); err == nil {
		// 成功解析为JSON，提取字段结构
		fields := make([]string, 0, len(jsonObj))
		for key, value := range jsonObj {
			fieldType := getValueType(value)
			fields = append(fields, fmt.Sprintf("%s:%s", key, fieldType))
		}
		structure := fmt.Sprintf("JSON结构: {%s}", strings.Join(fields, ", "))

		// 处理实际数据，对base64内容进行截断处理，但保留其他信息
		truncatedBody := common.TruncateBase64Content(bodyStr)
		// 不截断总长度，完整打印所有数据

		return structure, truncatedBody
	}

	// 尝试解析为JSON数组
	var jsonArray []interface{}
	if err := json.Unmarshal(requestBody, &jsonArray); err == nil {
		arrayType := "array"
		if len(jsonArray) > 0 {
			arrayType = fmt.Sprintf("array[%s]", getValueType(jsonArray[0]))
		}
		structure := fmt.Sprintf("JSON结构: %s[长度:%d]", arrayType, len(jsonArray))

		// 处理实际数据，对base64内容进行截断处理，但保留其他信息
		truncatedBody := common.TruncateBase64Content(bodyStr)
		// 不截断总长度，完整打印所有数据

		return structure, truncatedBody
	}

	// 不是JSON或解析失败，直接返回字符串（如果太长可以截断）
	const maxLength = 500
	if len(bodyStr) > maxLength {
		return bodyStr[:maxLength] + "...", bodyStr[:maxLength] + "..."
	}
	return bodyStr, bodyStr
}

// getValueType 获取值的类型描述
func getValueType(v interface{}) string {
	switch v := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		// 完整显示所有字段，不截断
		return fmt.Sprintf("object{%s}", strings.Join(keys, ","))
	case []interface{}:
		if len(v) > 0 {
			return fmt.Sprintf("array[%s]", getValueType(v[0]))
		}
		return "array"
	case string:
		if len(v) > 20 {
			return "string(长文本)"
		}
		return "string"
	case float64:
		// JSON数字默认解析为float64
		if v == float64(int64(v)) {
			return "number(int)"
		}
		return "number(float)"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func ClaudeCountTokens(c *gin.Context) {
	var request dto.ClaudeCountTokensRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		// 返回 Claude 格式错误
		c.JSON(http.StatusBadRequest, gin.H{
			"type": "error",
			"error": types.ClaudeError{
				Type:    "invalid_request_error",
				Message: err.Error(),
			},
		})
		return
	}

	// 构造 ClaudeRequest 复用现有计数逻辑
	claudeReq := dto.ClaudeRequest{
		Model:    request.Model,
		System:   request.System,
		Messages: request.Messages,
		Tools:    request.Tools,
	}

	tokens, err := service.CountTokenClaudeRequest(claudeReq, request.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"type": "error",
			"error": types.ClaudeError{
				Type:    "api_error",
				Message: err.Error(),
			},
		})
		return
	}

	c.JSON(http.StatusOK, dto.ClaudeCountTokensResponse{
		InputTokens: tokens,
	})
}
