package controller

import (
	"bytes"
	"io"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

// RealtimeWebRTCDirect 直接代理 /v1/realtime（HTTP SDP），用于本地上游无该路由时转发到真实上游
func RealtimeWebRTCDirect(c *gin.Context) {
	// 读取原始 body（SDP）
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil || len(bodyBytes) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid sdp"})
		return
	}
	model := c.Query("model")
	if model == "" {
		model = "gpt-realtime"
	}

	upstream := os.Getenv("UPSTREAM_REALTIME_URL")
	if upstream == "" {
		upstream = "https://api.openai.com/v1/realtime"
	}
	apiToken := os.Getenv("UPSTREAM_REALTIME_API_TOKEN")
	if apiToken == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "missing UPSTREAM_REALTIME_API_TOKEN"})
		return
	}

	// 保留 query model
	url := upstream
	if url == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upstream url empty"})
		return
	}
	if len(c.Request.URL.RawQuery) > 0 {
		url += "?" + c.Request.URL.RawQuery
	} else {
		url += "?model=" + model
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	req.Header.Set("Content-Type", "application/sdp")
	req.Header.Set("Authorization", "Bearer "+apiToken)
	req.Header.Set("OpenAI-Beta", "realtime=v1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, gin.H{"error": "upstream error", "status": resp.StatusCode, "body": string(respBody)})
		return
	}
	// 透传 text/plain
	c.Data(http.StatusOK, "text/plain", respBody)
}
