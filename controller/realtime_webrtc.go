package controller

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"

	"one-api/model"
	relaycommon "one-api/relay/common"
	"one-api/service"

	"github.com/gin-gonic/gin"
)

// RealtimeWebRTCOffer 处理 /realtime/webrtc/offer（lab 内部），使用 SystemAccessTokenAuth，按 user_id 选渠道/扣费
func RealtimeWebRTCOffer(c *gin.Context) {
	type OfferReq struct {
		Sdp   string `json:"sdp"`
		Model string `json:"model"`
	}
	var req OfferReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Sdp == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sdp"})
		return
	}
	modelName := req.Model
	if modelName == "" {
		modelName = "gpt-realtime"
	}

	// 从 query 获取 nebula_api_id（API 系统 userId），回落 user_id
	userIDStr := c.Query("nebula_api_id")
	if userIDStr == "" {
		userIDStr = c.Query("user_id")
	}
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing nebula_api_id"})
		return
	}
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// 选渠道（默认 group 为空，retry=0）
	channel, err := model.GetRandomSatisfiedChannel("", modelName, 0)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "no channel available: " + err.Error()})
		return
	}

	// 构造上游 URL 和 key
	info := &relaycommon.RelayInfo{
		UserId: int(userID),
	}
	baseURL := channel.GetBaseURL()
	if baseURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "channel base_url empty"})
		return
	}
	// WebRTC SDP 需走 HTTPS/HTTP，上游若配置为 wss/ws 则切换协议
	baseURL = strings.TrimSpace(baseURL)
	if strings.HasPrefix(baseURL, "wss://") {
		baseURL = "https://" + strings.TrimPrefix(baseURL, "wss://")
	} else if strings.HasPrefix(baseURL, "ws://") {
		baseURL = "http://" + strings.TrimPrefix(baseURL, "ws://")
	}
	url := baseURL
	// 避免重复追加 /v1/realtime
	if strings.HasSuffix(url, "/v1/realtime") {
		// 保持不变
	} else if strings.HasSuffix(url, "/v1/realtime/") {
		url = strings.TrimSuffix(url, "/") // 去掉最后一个斜杠
	} else {
		if url[len(url)-1] != '/' {
			url += "/v1/realtime"
		} else {
			url += "v1/realtime"
		}
	}
	url += "?model=" + modelName

	// 组装请求
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewBufferString(req.Sdp))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	httpReq.Header.Set("Content-Type", "application/sdp")
	httpReq.Header.Set("Accept", "application/sdp")
	// 使用渠道 key
	key, _, apiErr := channel.GetNextEnabledKey()
	if apiErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": apiErr.Error()})
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+key)
	httpReq.Header.Set("OpenAI-Beta", "realtime=v1")

	// 记录上游请求信息便于排查
	c.Set("upstream_url", url)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// 预扣一次请求额度（按 1 单位）
	if err := service.PreConsumeQuota(c, 1, info); err != nil {
		c.JSON(err.StatusCode, gin.H{"error": err.Error()})
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{"error": "upstream error", "status": resp.StatusCode, "body": string(body)})
		return
	}
	c.Data(http.StatusOK, "text/plain", body)
}
