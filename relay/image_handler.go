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
	const maxBase64Length = 50

	var result strings.Builder
	startIndex := 0

	for {
		// 查找base64前缀
		base64Index := strings.Index(content[startIndex:], base64Prefix)
		if base64Index == -1 {
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

		// 找到下一个引号、空格、逗号或大括号作为结束位置
		endIndex := markerIndex + len(base64Marker)
		actualEnd := len(content)

		for _, delimiter := range []string{"\"", " ", ",", "}"} {
			if pos := strings.Index(content[endIndex:], delimiter); pos != -1 {
				pos += endIndex
				if pos < actualEnd {
					actualEnd = pos
				}
			}
		}

		// 如果base64数据长度超过指定长度，则截断
		if actualEnd-endIndex > maxBase64Length {
			result.WriteString(content[base64Index:endIndex])
			result.WriteString("[base64数据已截断]")
			startIndex = actualEnd
		} else {
			// 短数据保持原样
			result.WriteString(content[base64Index:actualEnd])
			startIndex = actualEnd
		}
	}

	// 添加剩余内容
	result.WriteString(content[startIndex:])
	return result.String()
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
