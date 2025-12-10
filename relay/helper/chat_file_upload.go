package helper

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relayconstant "one-api/relay/constant"
	"one-api/service"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxUploadFileSize = 50 * 1024 * 1024 // 50MB

// AutoUploadChatFiles 检测 messages 中的 base64 文件并在转发前上传，改写为 file_id。
// 触发条件：
// - 路径为 /v1/chat/completions（即 RelayModeChatCompletions）
// - 模型名包含 "gpt"
// - content.type=file 且 file_data 非空、file_id 为空
func AutoUploadChatFiles(c *gin.Context, relayMode int, req *dto.GeneralOpenAIRequest) (*dto.GeneralOpenAIRequest, *types.NewAPIError) {
	if req == nil {
		return req, nil
	}
	if relayMode != relayconstant.RelayModeChatCompletions {
		log.Printf("[AutoUploadChatFiles] relayMode != RelayModeChatCompletions, relayMode=%d", relayMode)
		return req, nil
	}
	if !strings.Contains(strings.ToLower(req.Model), "gpt") {
		log.Printf("[AutoUploadChatFiles] model does not contain 'gpt', model=%s", req.Model)
		return req, nil
	}

	var changed bool

	for mi := range req.Messages {
		contents := req.Messages[mi].ParseContent()
		var updated bool
		for ci := range contents {
			if contents[ci].Type != dto.ContentTypeFile {
				continue
			}
			file := contents[ci].GetFile()
			if file == nil || file.FileId != "" || file.FileData == "" {
				continue
			}

			fileID, err := uploadFileDataToChannel(c, file.FileName, file.FileData)
			if err != nil {
				return nil, err
			}

			contents[ci].File = &dto.MessageFile{
				FileId: fileID,
			}
			updated = true
			changed = true
		}

		if updated {
			// 将修改后的内容写回 message
			req.Messages[mi].SetMediaContent(contents)
		}
	}

	if !changed {
		return req, nil
	}

	// 将改写后的请求体重新写回 gin 上下文，后续计费/转发使用更新后的 body
	body, err := common.Marshal(req)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeJsonMarshalFailed, types.ErrOptionWithSkipRetry())
	}
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
	c.Set(common.KeyRequestBody, body)

	// 文件与渠道绑定，避免重试切换渠道导致 file_id 不可用
	c.Set("specific_channel_id", true)

	return req, nil
}

func uploadFileDataToChannel(c *gin.Context, fileName, fileData string) (string, *types.NewAPIError) {
	decoded, err := base64.StdEncoding.DecodeString(fileData)
	if err != nil {
		return "", types.NewErrorWithStatusCode(fmt.Errorf("file_data base64 解码失败: %v", err), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if len(decoded) > maxUploadFileSize {
		return "", types.NewErrorWithStatusCode(fmt.Errorf("文件过大: %d bytes, 最大允许 %d bytes", len(decoded), maxUploadFileSize), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	if fileName == "" {
		fileName = "upload.bin"
	}

	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	baseURL := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
	if baseURL == "" {
		return "", types.NewError(fmt.Errorf("渠道基础地址为空"), types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
	}
	apiVersion := c.GetString("api_version")
	uploadURL := buildFileUploadURL(baseURL, channelType, apiVersion)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("purpose", "assistants"); err != nil {
		return "", types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	if _, err := part.Write(decoded); err != nil {
		return "", types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}
	writer.Close()

	req, err := http.NewRequest(http.MethodPost, uploadURL, &buf)
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeDoRequestFailed, types.ErrOptionWithSkipRetry())
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if channelType == constant.ChannelTypeAzure {
		req.Header.Set("api-key", apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if org := common.GetContextKeyString(c, constant.ContextKeyChannelOrganization); org != "" {
		req.Header.Set("OpenAI-Organization", org)
	}

	resp, err := service.GetHttpClient().Do(req)
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeDoRequestFailed)
	}
	defer service.CloseResponseBodyGracefully(resp)

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		msg := string(bodyBytes)
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		return "", types.NewErrorWithStatusCode(fmt.Errorf("文件上传失败，status=%d, body=%s", resp.StatusCode, msg), types.ErrorCodeBadResponse, resp.StatusCode, types.ErrOptionWithSkipRetry())
	}

	var uploadResp struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal(bodyBytes, &uploadResp); err != nil {
		return "", types.NewError(err, types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}
	if uploadResp.Id == "" {
		return "", types.NewError(fmt.Errorf("上游未返回 file_id"), types.ErrorCodeBadResponse, types.ErrOptionWithSkipRetry())
	}
	return uploadResp.Id, nil
}

func buildFileUploadURL(baseURL string, channelType int, apiVersion string) string {
	base := strings.TrimRight(baseURL, "/")
	if channelType == constant.ChannelTypeAzure {
		// Azure文件上传不需要api-version参数
		return base + "/openai/v1/files"
	}
	return base + "/v1/files"
}
