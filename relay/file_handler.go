package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/relay/channel"
	relaycommon "one-api/relay/common"
	"one-api/service"
	"one-api/types"

	"github.com/gin-gonic/gin"
)

// FileHelper 透传 /v1/files 上传到上游（OpenAI/Azure）。
func FileHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	body, err := common.GetRequestBody(c)
	if err != nil {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	resp, err := channel.DoFormRequest(adaptor, c, info, bytes.NewBuffer(body))
	if err != nil {
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	if resp != nil {
		defer service.CloseResponseBodyGracefully(resp)

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), resp, false)
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}

		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
		}

		service.IOCopyBytesGracefully(c, resp, responseBody)
	}

	// 文件上传不计 token，usage 为空时会回落到 0 计费。
	postConsumeQuota(c, info, nil, "")
	return nil
}
