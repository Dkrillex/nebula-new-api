package ali

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"one-api/dto"
	"one-api/relay/channel"
	"one-api/relay/channel/claude"
	"one-api/relay/channel/openai"
	relaycommon "one-api/relay/common"
	"one-api/relay/constant"
	"one-api/service"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error) {
	openaiRequest, err := service.GeminiToOpenAIRequest(request, info)
	if err != nil {
		return nil, err
	}
	return a.ConvertOpenAIRequest(c, info, openaiRequest)
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return req, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var fullRequestURL string
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		fullRequestURL = fmt.Sprintf("%s/api/v2/apps/claude-code-proxy/v1/messages", info.ChannelBaseUrl)
	default:
		switch info.RelayMode {
		case constant.RelayModeEmbeddings:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/embeddings", info.ChannelBaseUrl)
		case constant.RelayModeRerank:
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/rerank/text-rerank/text-rerank", info.ChannelBaseUrl)
		case constant.RelayModeImagesGenerations:
			// qwen-image-plus 和 qwen-image-edit 系列使用新的 multimodal-generation API
			if info.OriginModelName == "qwen-image-plus" || strings.HasPrefix(info.OriginModelName, "qwen-image-edit") {
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
			} else {
				// 其他模型使用旧的 text2image API
				fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/text2image/image-synthesis", info.ChannelBaseUrl)
			}
		case constant.RelayModeImagesEdits:
			fullRequestURL = fmt.Sprintf("%s/api/v1/services/aigc/multimodal-generation/generation", info.ChannelBaseUrl)
		case constant.RelayModeCompletions:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/completions", info.ChannelBaseUrl)
		default:
			fullRequestURL = fmt.Sprintf("%s/compatible-mode/v1/chat/completions", info.ChannelBaseUrl)
		}
	}

	return fullRequestURL, nil
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	if info.IsStream {
		req.Set("X-DashScope-SSE", "enable")
	}
	if c.GetString("plugin") != "" {
		req.Set("X-DashScope-Plugin", c.GetString("plugin"))
	}
	if info.RelayMode == constant.RelayModeImagesGenerations {
		// qwen-image-plus 和 qwen-image-edit 系列使用新的 multimodal-generation API，不支持异步模式
		if info.OriginModelName != "qwen-image-plus" && !strings.HasPrefix(info.OriginModelName, "qwen-image-edit") {
			req.Set("X-DashScope-Async", "enable")
		}
	}
	if info.RelayMode == constant.RelayModeImagesEdits {
		req.Set("Content-Type", "application/json")
	}
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// docs: https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2712216
	// fix: InternalError.Algo.InvalidParameter: The value of the enable_thinking parameter is restricted to True.
	if strings.Contains(request.Model, "thinking") {
		request.EnableThinking = true
		request.Stream = true
		info.IsStream = true
	}
	// 若客户端显式声明 enable_thinking，则优先采用；开启后强制流式，避免上游报错
	// 兼容 request.EnableThinking 为任意类型（bool/字符串）
	if request.EnableThinking != nil {
		switch v := request.EnableThinking.(type) {
		case bool:
			if v {
				request.Stream = true
				info.IsStream = true
			}
		case string:
			if strings.EqualFold(v, "true") {
				request.EnableThinking = true
				request.Stream = true
				info.IsStream = true
			}
		}
	}
	// 非流式强制关闭 enable_thinking，避免上游报错
	if !request.Stream {
		request.EnableThinking = false
	}
	info.IsStream = request.Stream

	// 将“推理转内容”请求开关映射到通道设置，仅影响下行整形
	if request.NebulaThinkingToContent {
		info.ChannelSetting.ThinkingToContent = true
	}

	// 统一构造请求体，注入 Qwen 专有参数到 parameters/input，保持 OpenAI 兼容字段不变
	body := request.ToMap()
	// 先准备 parameters 基础对象
	var parameters map[string]any
	if exist, ok := body["parameters"].(map[string]any); ok && exist != nil {
		parameters = exist
	} else {
		parameters = make(map[string]any)
	}
	// 合并 request.QwenParameters
	if len(request.QwenParameters) > 0 {
		for k, v := range request.QwenParameters {
			parameters[k] = v
		}
	}
	// 合并 body.qwen_parameters，然后删除顶层 qwen_parameters，避免重复
	if qp, ok := body["qwen_parameters"].(map[string]any); ok && qp != nil {
		for k, v := range qp {
			parameters[k] = v
		}
		delete(body, "qwen_parameters")
	}
	// 若用户顶层直接给了 search_options / asr_options，移动到 parameters
	if so, ok := body["search_options"]; ok && so != nil {
		parameters["search_options"] = so
		delete(body, "search_options")
	}
	if ao, ok := body["asr_options"]; ok && ao != nil {
		parameters["asr_options"] = ao
		delete(body, "asr_options")
	}
	// 若用户顶层放了 enable_search / incremental_output，也归并到 parameters（避免歧义）
	if es, ok := body["enable_search"]; ok {
		parameters["enable_search"] = es
		delete(body, "enable_search")
	}
	if io2, ok := body["incremental_output"]; ok {
		parameters["incremental_output"] = io2
		delete(body, "incremental_output")
	}
	// 注：通义千问缓存通过messages中的cache_control字段实现，无需额外处理
	if len(parameters) > 0 {
		body["parameters"] = parameters
	}
	if len(request.QwenInput) > 0 {
		// 若 body 已存在 input 且为 map，尝试浅合并；否则直接赋值
		if exist, ok := body["input"].(map[string]any); ok && exist != nil {
			for k, v := range request.QwenInput {
				exist[k] = v
			}
			body["input"] = exist
		} else {
			body["input"] = request.QwenInput
		}
		// 删除顶层 qwen_input，避免重复
		delete(body, "qwen_input")
	}
	// 删除仅内部使用的开关，避免透传
	delete(body, "nebula_thinking_to_content")
	// 根据 Ali 的限制再次覆盖 enable_thinking 与 stream 字段
	// 按规则透传：流式 -> 保持 enable_thinking；非流式 -> 关闭 enable_thinking 并移除 stream
	if info.IsStream {
		body["enable_thinking"] = request.EnableThinking
		body["stream"] = true
	} else {
		body["enable_thinking"] = false
		delete(body, "stream")
	}

	switch info.RelayMode {
	default:
		// 仅对部分通用参数做范围修正（如 top_p），其余参数保持透传
		aliReq := requestOpenAI2Ali(*request)
		// 将修正后的 top_p 等回写到 body
		if aliReq != nil {
			body["top_p"] = aliReq.TopP
		}
		return body, nil
	}
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	if info.RelayMode == constant.RelayModeImagesGenerations {
		aliRequest, err := oaiImage2Ali(request)
		if err != nil {
			return nil, fmt.Errorf("convert image request failed: %w", err)
		}
		return aliRequest, nil
	} else if info.RelayMode == constant.RelayModeImagesEdits {
		// ali image edit https://bailian.console.aliyun.com/?tab=api#/api/?type=model&url=2976416
		// 如果用户使用表单，则需要解析表单数据
		if strings.Contains(c.Request.Header.Get("Content-Type"), "multipart/form-data") {
			aliRequest, err := oaiFormEdit2AliImageEdit(c, info, request)
			if err != nil {
				return nil, fmt.Errorf("convert image edit form request failed: %w", err)
			}
			return aliRequest, nil
		} else {
			aliRequest, err := oaiImage2Ali(request)
			if err != nil {
				return nil, fmt.Errorf("convert image request failed: %w", err)
			}
			return aliRequest, nil
		}
	}
	return nil, fmt.Errorf("unsupported image relay mode: %d", info.RelayMode)
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return ConvertRerankRequest(request), nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if info.IsStream {
			return claude.ClaudeStreamHandler(c, resp, info, claude.RequestModeMessage)
		} else {
			return claude.ClaudeHandler(c, resp, info, claude.RequestModeMessage)
		}
	default:
		switch info.RelayMode {
		case constant.RelayModeImagesGenerations:
			err, usage = aliImageHandler(c, resp, info)
		case constant.RelayModeImagesEdits:
			err, usage = aliImageEditHandler(c, resp, info)
		case constant.RelayModeRerank:
			err, usage = RerankHandler(c, resp, info)
		default:
			adaptor := openai.Adaptor{}
			usage, err = adaptor.DoResponse(c, resp, info)
		}
		return usage, err
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
