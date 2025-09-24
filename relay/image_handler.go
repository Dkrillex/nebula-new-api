package relay

import (
	"bytes"
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

// truncateBase64Content 截断JSON字符串中的base64内容，保留其他信息
func truncateBase64Content(content string) string {
	const base64Prefix = "data:image/"
	const base64Marker = ";base64,"
	const maxBase64Length = 50 // 增加一些长度以保留更多信息

	var result strings.Builder
	startIndex := 0

	for {
		// 查找base64前缀
		base64Index := strings.Index(content[startIndex:], base64Prefix)
		if base64Index == -1 {
			// 没有更多base64内容，添加剩余部分
			result.WriteString(content[startIndex:])
			break
		}
		base64Index += startIndex

		// 添加base64前的内容
		result.WriteString(content[startIndex:base64Index])

		// 查找base64标记
		markerIndex := strings.Index(content[base64Index:], base64Marker)
		if markerIndex == -1 {
			// 没找到base64标记，保持原样
			result.WriteString(content[base64Index:])
			break
		}
		markerIndex += base64Index
		base64StartIndex := markerIndex + len(base64Marker)

		// 查找base64数据的结束位置（下一个双引号）
		base64EndIndex := strings.Index(content[base64StartIndex:], "\"")
		if base64EndIndex == -1 {
			// 没找到结束引号，保持原样
			result.WriteString(content[base64Index:])
			break
		}
		base64EndIndex += base64StartIndex

		// 计算base64数据长度
		base64DataLength := base64EndIndex - base64StartIndex

		// 如果base64数据长度超过指定长度，则截断
		if base64DataLength > maxBase64Length {
			// 保留前缀和部分base64数据
			result.WriteString(content[base64Index:base64StartIndex])
			result.WriteString(content[base64StartIndex : base64StartIndex+maxBase64Length])
			result.WriteString("...[base64数据已截断，长度:")
			result.WriteString(fmt.Sprintf("%d", base64DataLength))
			result.WriteString("]\"")
			startIndex = base64EndIndex + 1
		} else {
			// 短数据保持原样
			result.WriteString(content[base64Index : base64EndIndex+1])
			startIndex = base64EndIndex + 1
		}
	}

	// 处理没有前缀的base64数据
	return truncateRawBase64Content(result.String())
}

// truncateRawBase64Content 处理没有前缀的纯base64数据
func truncateRawBase64Content(content string) string {
	const maxBase64Length = 100
	const minBase64Length = 200 // 只有超过这个长度的才认为是需要截断的base64数据

	var result strings.Builder
	startIndex := 0

	for {
		// 查找可能的base64数据开始位置（以双引号开始的长字符串）
		quoteIndex := strings.Index(content[startIndex:], "\"")
		if quoteIndex == -1 {
			// 没有更多引号，添加剩余部分
			result.WriteString(content[startIndex:])
			break
		}
		quoteIndex += startIndex

		// 添加引号前的内容
		result.WriteString(content[startIndex : quoteIndex+1])

		// 查找下一个引号
		nextQuoteIndex := strings.Index(content[quoteIndex+1:], "\"")
		if nextQuoteIndex == -1 {
			// 没找到结束引号，保持原样
			result.WriteString(content[quoteIndex+1:])
			break
		}
		nextQuoteIndex += quoteIndex + 1

		// 获取引号内的内容
		quotedContent := content[quoteIndex+1 : nextQuoteIndex]

		// 检查是否是base64数据（长度足够且包含base64字符）
		if len(quotedContent) > minBase64Length && isBase64String(quotedContent) {
			// 这是base64数据，需要截断
			if len(quotedContent) > maxBase64Length {
				result.WriteString(quotedContent[:maxBase64Length])
				result.WriteString("...[base64数据已截断，长度:")
				result.WriteString(fmt.Sprintf("%d", len(quotedContent)))
				result.WriteString("]")
			} else {
				result.WriteString(quotedContent)
			}
		} else {
			// 不是base64数据，保持原样
			result.WriteString(quotedContent)
		}

		startIndex = nextQuoteIndex + 1
	}

	return result.String()
}

// isBase64String 检查字符串是否可能是base64数据
func isBase64String(s string) bool {
	if len(s) == 0 {
		return false
	}

	// 检查是否只包含base64字符
	base64Chars := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="
	base64CharCount := 0

	for _, char := range s {
		if strings.ContainsRune(base64Chars, char) {
			base64CharCount++
		}
	}

	// 如果超过80%的字符是base64字符，且长度足够，则认为是base64
	return float64(base64CharCount)/float64(len(s)) > 0.8
}

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

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
				truncatedContent := truncateBase64Content(string(jsonData))
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
			newAPIError = service.RelayErrorHandler(httpResp, false)
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

	// 设置生成图片数量到 context，用于按次计费乘数计算
	if _, exists := c.Get("generated_images_count"); !exists {
		c.Set("generated_images_count", int(request.N))
	}

	quality := "standard"
	if request.Quality == "hd" {
		quality = "hd"
	}

	var logContent string

	if len(request.Size) > 0 {
		logContent = fmt.Sprintf("大小 %s, 品质 %s", request.Size, quality)
	}

	postConsumeQuota(c, info, usage.(*dto.Usage), logContent)
	return nil
}
