package relay

import (
	"fmt"
	"one-api/common"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func WssHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	// 初始化 PriceData（必须在处理请求之前）
	meta := &types.TokenCountMeta{MaxTokens: 0}
	_, err := helper.ModelPriceHelper(c, info, 1, meta)
	if err != nil {
		common.SysLog(fmt.Sprintf("[Realtime] ModelPriceHelper failed: %v, using default values", err))
		// 不返回错误，继续处理，使用代码中的默认值保护
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)
	//var requestBody io.Reader
	//firstWssRequest, _ := c.Get("first_wss_request")
	//requestBody = bytes.NewBuffer(firstWssRequest.([]byte))

	statusCodeMappingStr := c.GetString("status_code_mapping")
	resp, err := adaptor.DoRequest(c, info, nil)
	if err != nil {
		common.SysLog(fmt.Sprintf("[Realtime][DoRequest] upstream connect failed: %v, url=%s%s", err, info.ChannelBaseUrl, info.RequestURLPath))
		return types.NewError(err, types.ErrorCodeDoRequestFailed)
	}

	if resp != nil {
		info.TargetWs = resp.(*websocket.Conn)
		defer info.TargetWs.Close()
	}

	usage, newAPIError := adaptor.DoResponse(c, nil, info)
	if newAPIError != nil {
		common.SysLog(fmt.Sprintf("[Realtime][DoResponse] upstream closed with error: %v", newAPIError.Error()))
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}
	service.PostWssConsumeQuota(c, info, info.UpstreamModelName, usage.(*dto.RealtimeUsage), "")
	return nil
}
