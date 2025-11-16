package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"time"
)

const (
	ossUploadEnvKey       = "OSS_BASE64_ENDPOINT"
	defaultOSSHTTPTimeout = 120 * time.Second
)

var ossHTTPClient = &http.Client{
	Timeout: defaultOSSHTTPTimeout,
}

// OSSUploadRequest 定义 Go 服务上传 Base64 至 OSS 所需的请求体。
type OSSUploadRequest struct {
	Base64Content string `json:"base64Content"`
	FileName      string `json:"fileName,omitempty"`
	ExtensionType string `json:"extensionType,omitempty"`
}

// OSSUploadResponse 封装 ruoyi OSS 上传接口返回结构。
type OSSUploadResponse struct {
	Code int               `json:"code"`
	Msg  string            `json:"msg"`
	Data *OSSUploadPayload `json:"data"`
}

// OSSUploadPayload 为 ruoyi 的 `SysOssUploadVo`。
type OSSUploadPayload struct {
	OssID    string `json:"ossId"`
	OssURL   string `json:"url"`
	FileName string `json:"fileName"`
}

// GetOSSBase64Endpoint 返回配置的 OSS Base64 上传接口地址。
func GetOSSBase64Endpoint() string {
	return common.GetEnvOrDefaultString(ossUploadEnvKey, "")
}

// UploadBase64ToOSS 调用外部 Java 服务，将 Base64 内容上传到 OSS，返回 OSS 对象信息。
func UploadBase64ToOSS(ctx context.Context, endpoint string, payload *OSSUploadRequest) (*OSSUploadPayload, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("OSS Base64 endpoint 未配置，请设置环境变量 %s", ossUploadEnvKey)
	}
	if payload == nil || payload.Base64Content == "" {
		return nil, fmt.Errorf("上传内容不能为空")
	}

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化 OSS 上传请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("构造 OSS 上传请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := ossHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("调用 OSS 上传接口失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("OSS 上传接口返回异常状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取 OSS 上传响应失败: %w", err)
	}

	var ossResp OSSUploadResponse
	if err = json.Unmarshal(respBody, &ossResp); err != nil {
		return nil, fmt.Errorf("解析 OSS 上传响应失败: %w", err)
	}

	if ossResp.Code != 200 {
		return nil, fmt.Errorf("OSS 上传失败，code=%d, msg=%s", ossResp.Code, ossResp.Msg)
	}
	if ossResp.Data == nil || ossResp.Data.OssURL == "" {
		return nil, fmt.Errorf("OSS 上传成功但返回数据为空")
	}

	return ossResp.Data, nil
}
