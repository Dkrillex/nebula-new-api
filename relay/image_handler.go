package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	// 初始化 PriceData（必须在处理请求之前）
	meta := &types.TokenCountMeta{MaxTokens: 0}
	priceData, err := helper.ModelPriceHelper(c, info, 1, meta)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeModelPriceError, http.StatusInternalServerError)
	}
	info.PriceData = priceData

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader

	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverride(jsonData, info.ParamOverride)
				if err != nil {
					return types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
				}
			}

			if common.DebugEnabled {
				// 使用截断函数处理base64内容，保留其他信息
				truncatedContent := common.TruncateBase64Content(string(jsonData))
				logger.LogDebug(c, fmt.Sprintf("[image_Handler]image request body: %s", truncatedContent))
			}
			requestBody = bytes.NewBuffer(jsonData)
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	if usage.(*dto.Usage).TotalTokens == 0 {
		usage.(*dto.Usage).TotalTokens = int(request.N)
	}
	if usage.(*dto.Usage).PromptTokens == 0 {
		usage.(*dto.Usage).PromptTokens = int(request.N)
	}

	// 提取并传递 input_tokens_details（用于图像Token计费）
	if usage != nil {
		usageData := usage.(*dto.Usage)
		if usageData.InputTokensDetails != nil {
			// 将 InputTokenDetails 转换为 map 存入 context
			inputDetails := make(map[string]interface{})
			inputDetails["image_tokens"] = usageData.InputTokensDetails.ImageTokens
			inputDetails["text_tokens"] = usageData.InputTokensDetails.TextTokens
			c.Set("input_tokens_details", inputDetails)

			if common.DebugEnabled {
				logger.LogDebug(c, fmt.Sprintf("[ImageHelper] InputTokensDetails: image_tokens=%d, text_tokens=%d",
					usageData.InputTokensDetails.ImageTokens, usageData.InputTokensDetails.TextTokens))
			}
		}
	}

	// 设置生成图片数量到 context，用于按次计费乘数计算
	if _, exists := c.Get("generated_images_count"); !exists {
		c.Set("generated_images_count", int(request.N))
	}

	// 设置图像质量和尺寸到 context（用于 ImageTokenPricing 计费）
	c.Set("image_quality", request.Quality)
	c.Set("image_size", request.Size)

	// 设置输入图片数量（用于 ImageTokenPricing 计费）
	if request.Extra != nil {
		if imagesData, ok := request.Extra["images"]; ok {
			var images []string
			if err := json.Unmarshal(imagesData, &images); err == nil {
				c.Set("input_images_count", len(images))
				if common.DebugEnabled {
					logger.LogDebug(c, fmt.Sprintf("[ImageHelper] 输入图片数量: %d", len(images)))
				}
			}
		} else if _, ok := request.Extra["image"]; ok {
			c.Set("input_images_count", 1)
			if common.DebugEnabled {
				logger.LogDebug(c, "[ImageHelper] 输入图片数量: 1")
			}
		}
	}

	quality := "standard"
	if request.Quality == "hd" || request.Quality == "high" {
		quality = "hd"
	}

	var logContent string

	if len(request.Size) > 0 {
		logContent = fmt.Sprintf("大小 %s, 品质 %s, 张数 %d", request.Size, quality, request.N)
	}

	// 调试日志：打印计费信息
	if common.DebugEnabled {
		logger.LogDebug(c, fmt.Sprintf("[ImageHelper] 开始计费: %s", logContent))
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), logContent)
	return nil
}
