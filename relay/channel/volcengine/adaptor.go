package volcengine

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	channelconstant "one-api/constant"
	"one-api/dto"
	"one-api/logger"
	"one-api/relay/channel"
	"one-api/relay/channel/openai"
	relaycommon "one-api/relay/common"
	"one-api/relay/constant"
	"one-api/service"
	"one-api/types"
	"path/filepath"
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
	adaptor := openai.Adaptor{}
	return adaptor.ConvertClaudeRequest(c, info, req)
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

// 豆包图片生成请求结构体
type DoubaoImageRequest struct {
	Model          string           `json:"model"`
	Prompt         string           `json:"prompt"`
	Image          interface{}      `json:"image,omitempty"` // 支持单张图片(string)或多张图片([]string)
	ImageData      *DoubaoImageData `json:"image_data,omitempty"`
	ResponseFormat string           `json:"response_format,omitempty"`
	Size           string           `json:"size,omitempty"`
	Watermark      *bool            `json:"watermark,omitempty"`
}

type DoubaoImageData struct {
	Data string `json:"data"`
}

// min 函数用于获取两个整数的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	switch info.RelayMode {
	case constant.RelayModeImagesGenerations:
		// 检查是否为图文生图模型
		if strings.Contains(info.UpstreamModelName, "seed") {
			// 处理图文生图请求
			req := &DoubaoImageRequest{
				Model:          info.UpstreamModelName,
				Prompt:         request.Prompt,
				Watermark:      request.Watermark,
				ResponseFormat: request.ResponseFormat,
				Size:           request.Size,
			}

			// 先判断extra是否存在
			if request.Extra != nil && len(request.Extra) > 0 {
				// 先检查是否有contents，有的话就解析一下
				if contentsData, exists := request.Extra["contents"]; exists {
					var contents []map[string]interface{}
					if err := json.Unmarshal(contentsData, &contents); err == nil {
						// 提取文本和图片
						var textParts []string
						var imageParts []string
						var hasTextInContents bool

						for _, content := range contents {
							if parts, ok := content["parts"].([]interface{}); ok {
								for _, part := range parts {
									if partMap, ok := part.(map[string]interface{}); ok {
										// 处理文本部分
										if text, exists := partMap["text"]; exists {
											if textStr, ok := text.(string); ok && textStr != "" {
												textParts = append(textParts, textStr)
												hasTextInContents = true
											}
										}
										// 处理图片URL - image格式
										if image, exists := partMap["image"]; exists {
											if imageStr, ok := image.(string); ok {
												imageParts = append(imageParts, imageStr)
											}
										}
									}
								}
							}
						}

						// 豆包图片生成逻辑调整：
						// 如果contents的part中包含text，则覆盖并忽略入参中的prompt字段
						// 如果contents中没有text，则使用prompt字段
						if hasTextInContents {
							req.Prompt = strings.Join(textParts, " ")
							//logger.LogInfo(c, fmt.Sprintf("使用contents中的text作为prompt: %s", req.Prompt))
						}

						// 处理图片 - 支持多张图片
						if len(imageParts) > 0 {
							if len(imageParts) == 1 {
								// 单张图片，使用字符串格式
								imageData := imageParts[0]
								if strings.HasPrefix(imageData, "http") {
									// URL格式
									req.Image = imageData
									//logger.LogInfo(c, fmt.Sprintf("使用单张图片URL: %s", imageData[:min(50, len(imageData))]+"..."))
								} else {
									// Base64格式
									req.Image = imageData
									//logger.LogInfo(c, fmt.Sprintf("使用单张Base64图片数据，长度: %d", len(imageData)))
								}
							} else {
								// 多张图片，使用数组格式
								req.Image = imageParts
								//logger.LogInfo(c, fmt.Sprintf("使用多张图片格式，共%d张图片", len(imageParts)))
							}
						}
					}
				}

				// 将extra参数直接合并到doubaoRequest的顶层
				reqBytes, _ := json.Marshal(req)
				var reqMap map[string]interface{}
				json.Unmarshal(reqBytes, &reqMap)

				for key, value := range request.Extra {
					var decoded interface{}
					if err := json.Unmarshal(value, &decoded); err == nil {
						reqMap[key] = decoded
					} else {
						reqMap[key] = value
					}
				}

				// 豆包 Seedream 4.0/4.5/5.0：确保 optimize_prompt_options 有默认值
				if info.UpstreamModelName == "doubao-seedream-4-0-250828" || info.UpstreamModelName == "doubao-seedream-4-5-251128" || info.UpstreamModelName == "doubao-seedream-5-0-260128" {
					if _, exists := reqMap["optimize_prompt_options"]; !exists {
						reqMap["optimize_prompt_options"] = map[string]interface{}{
							"mode": "standard",
						}
					}
				}

				// 重新构建doubaoRequest
				newBytes, _ := json.Marshal(reqMap)
				err := json.Unmarshal(newBytes, &req)
				if err != nil {
					return nil, err
				}
				delete(reqMap, "contents")
				return reqMap, nil
			} else {
				// 如果没有extra，使用原有逻辑
				doubaoRequest := DoubaoImageRequest{
					Model:          request.Model,
					Prompt:         request.Prompt,
					Watermark:      request.Watermark,
					ResponseFormat: request.ResponseFormat,
					Size:           request.Size,
				}

				return doubaoRequest, nil
			}
		} else {
			// 构建豆包文生图请求 - 统一文生图格式
			doubaoRequest := DoubaoImageRequest{
				Model:  request.Model,
				Prompt: request.Prompt,
				Size:   request.Size,
			}

			// 直接赋值整个Extra，支持所有火山引擎API参数
			if request.Extra != nil {
				// 将Extra中的内容直接合并到doubaoRequest中
				extraBytes, _ := json.Marshal(doubaoRequest)
				var doubaoMap map[string]interface{}
				json.Unmarshal(extraBytes, &doubaoMap)

				for key, value := range request.Extra {
					doubaoMap[key] = value
				}

				// 如果是 doubao-seedream-4-0-250828 模型，确保 optimize_prompt_options 有默认值
				if info.UpstreamModelName == "doubao-seedream-4-0-250828" || info.UpstreamModelName == "doubao-seedream-4-5-251128" {
					if _, exists := doubaoMap["optimize_prompt_options"]; !exists {
						doubaoMap["optimize_prompt_options"] = map[string]interface{}{
							"mode": "standard",
						}
					}
				}

				// 重新构建doubaoRequest
				newBytes, _ := json.Marshal(doubaoMap)
				json.Unmarshal(newBytes, &doubaoRequest)

				logger.LogInfo(c, fmt.Sprintf("传递额外参数: %+v", request.Extra))
			} else {
				// 如果是 doubao-seedream-4-0-250828 模型且没有 Extra，也要设置默认值
				if info.UpstreamModelName == "doubao-seedream-4-0-250828" || info.UpstreamModelName == "doubao-seedream-4-5-251128" {
					extraBytes, _ := json.Marshal(doubaoRequest)
					var doubaoMap map[string]interface{}
					json.Unmarshal(extraBytes, &doubaoMap)

					doubaoMap["optimize_prompt_options"] = map[string]interface{}{
						"mode": "standard",
					}

					newBytes, _ := json.Marshal(doubaoMap)
					json.Unmarshal(newBytes, &doubaoRequest)
				}
			}

			logger.LogInfo(c, fmt.Sprintf("文生图请求 - Model: %s, Prompt: %s", doubaoRequest.Model, doubaoRequest.Prompt))
			return doubaoRequest, nil
		}
	case constant.RelayModeImagesEdits:

		var requestBody bytes.Buffer
		writer := multipart.NewWriter(&requestBody)

		writer.WriteField("model", request.Model)
		// 获取所有表单字段
		formData := c.Request.PostForm
		// 遍历表单字段并打印输出
		for key, values := range formData {
			if key == "model" {
				continue
			}
			for _, value := range values {
				writer.WriteField(key, value)
			}
		}

		// Parse the multipart form to handle both single image and multiple images
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max memory
			return nil, errors.New("failed to parse multipart form")
		}

		if c.Request.MultipartForm != nil && c.Request.MultipartForm.File != nil {
			// Check if "image" field exists in any form, including array notation
			var imageFiles []*multipart.FileHeader
			var exists bool

			// First check for standard "image" field
			if imageFiles, exists = c.Request.MultipartForm.File["image"]; !exists || len(imageFiles) == 0 {
				// If not found, check for "image[]" field
				if imageFiles, exists = c.Request.MultipartForm.File["image[]"]; !exists || len(imageFiles) == 0 {
					// If still not found, iterate through all fields to find any that start with "image["
					foundArrayImages := false
					for fieldName, files := range c.Request.MultipartForm.File {
						if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
							foundArrayImages = true
							for _, file := range files {
								imageFiles = append(imageFiles, file)
							}
						}
					}

					// If no image fields found at all
					if !foundArrayImages && (len(imageFiles) == 0) {
						return nil, errors.New("image is required")
					}
				}
			}

			// Process all image files
			for i, fileHeader := range imageFiles {
				file, err := fileHeader.Open()
				if err != nil {
					return nil, fmt.Errorf("failed to open image file %d: %w", i, err)
				}
				defer file.Close()

				// If multiple images, use image[] as the field name
				fieldName := "image"
				if len(imageFiles) > 1 {
					fieldName = "image[]"
				}

				// Determine MIME type based on file extension
				mimeType := detectImageMimeType(fileHeader.Filename)

				// Create a form file with the appropriate content type
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fileHeader.Filename))
				h.Set("Content-Type", mimeType)

				part, err := writer.CreatePart(h)
				if err != nil {
					return nil, fmt.Errorf("create form part failed for image %d: %w", i, err)
				}

				if _, err := io.Copy(part, file); err != nil {
					return nil, fmt.Errorf("copy file failed for image %d: %w", i, err)
				}
			}

			// Handle mask file if present
			if maskFiles, exists := c.Request.MultipartForm.File["mask"]; exists && len(maskFiles) > 0 {
				maskFile, err := maskFiles[0].Open()
				if err != nil {
					return nil, errors.New("failed to open mask file")
				}
				defer maskFile.Close()

				// Determine MIME type for mask file
				mimeType := detectImageMimeType(maskFiles[0].Filename)

				// Create a form file with the appropriate content type
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="mask"; filename="%s"`, maskFiles[0].Filename))
				h.Set("Content-Type", mimeType)

				maskPart, err := writer.CreatePart(h)
				if err != nil {
					return nil, errors.New("create form file failed for mask")
				}

				if _, err := io.Copy(maskPart, maskFile); err != nil {
					return nil, errors.New("copy mask file failed")
				}
			}
		} else {
			return nil, errors.New("no multipart form data found")
		}

		// 关闭 multipart 编写器以设置分界线
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return bytes.NewReader(requestBody.Bytes()), nil

	default:
		return request, nil
	}
}

// processImageData 处理图像数据，支持base64和URL格式
func (a *Adaptor) processImageData(c *gin.Context, imageData string) (string, error) {
	// 处理base64图像
	if strings.HasPrefix(imageData, "data:image/") {
		return imageData, nil
	}

	// 处理URL图像（下载并转换为base64）
	if strings.HasPrefix(imageData, "http") {
		fileData, err := service.GetFileBase64FromUrl(c, imageData, "formatting image for Doubao")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("data:%s;base64,%s", fileData.MimeType, fileData.Base64Data), nil
	}

	return imageData, nil
}

// detectImageMimeType determines the MIME type based on the file extension
func detectImageMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	default:
		// Try to detect from extension if possible
		if strings.HasPrefix(ext, ".jp") {
			return "image/jpeg"
		}
		// Default to png as a fallback
		return "image/png"
	}
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	// 支持自定义域名，如果未设置则使用默认域名
	baseUrl := info.ChannelBaseUrl
	if baseUrl == "" {
		baseUrl = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine]
	}

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if strings.HasPrefix(info.UpstreamModelName, "bot") {
			return fmt.Sprintf("%s/api/v3/bots/chat/completions", baseUrl), nil
		}
		return fmt.Sprintf("%s/api/v3/chat/completions", baseUrl), nil
	default:
		switch info.RelayMode {
		case constant.RelayModeChatCompletions:
			if strings.HasPrefix(info.UpstreamModelName, "bot") {
				return fmt.Sprintf("%s/api/v3/bots/chat/completions", baseUrl), nil
			}
			return fmt.Sprintf("%s/api/v3/chat/completions", baseUrl), nil
		case constant.RelayModeEmbeddings:
			return fmt.Sprintf("%s/api/v3/embeddings", baseUrl), nil
		case constant.RelayModeImagesGenerations:
			return fmt.Sprintf("%s/api/v3/images/generations", baseUrl), nil
		case constant.RelayModeImagesEdits:
			return fmt.Sprintf("%s/api/v3/images/edits", baseUrl), nil
		case constant.RelayModeRerank:
			return fmt.Sprintf("%s/api/v3/rerank", baseUrl), nil
		default:
		}
	}
	return "", fmt.Errorf("unsupported relay mode: %d", info.RelayMode)
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	// 适配 方舟deepseek混合模型 的 thinking 后缀
	if strings.HasSuffix(info.UpstreamModelName, "-thinking") && strings.HasPrefix(info.UpstreamModelName, "deepseek") {
		info.UpstreamModelName = strings.TrimSuffix(info.UpstreamModelName, "-thinking")
		request.Model = info.UpstreamModelName
		request.THINKING = json.RawMessage(`{"type": "enabled"}`)
	}
	return request, nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

// doubaoImageHandler 处理豆包图片生成响应
func (a *Adaptor) doubaoImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	// 解析豆包响应
	var doubaoResponse map[string]interface{}
	err = json.Unmarshal(responseBody, &doubaoResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 转换为OpenAI格式
	imageResponse := dto.ImageResponse{
		Created: int64(doubaoResponse["created"].(float64)),
		Data:    make([]dto.ImageData, 0),
	}

	// 处理data字段
	if dataArray, ok := doubaoResponse["data"].([]interface{}); ok {
		for _, item := range dataArray {
			if dataItem, ok := item.(map[string]interface{}); ok {
				imageData := dto.ImageData{}

				// 处理URL字段
				if url, exists := dataItem["url"]; exists {
					imageData.Url = url.(string)
				}

				// 处理B64Json字段
				if b64Json, exists := dataItem["b64_json"]; exists {
					imageData.B64Json = b64Json.(string)
				}

				// 处理RevisedPrompt字段
				if revisedPrompt, exists := dataItem["size"]; exists {
					imageData.RevisedPrompt = revisedPrompt.(string)
				}

				imageResponse.Data = append(imageResponse.Data, imageData)
			}
		}
	}

	// 计算使用量并写入响应的 usage（与对话接口一致）
	usage := &dto.Usage{}
	if usageData, ok := doubaoResponse["usage"].(map[string]interface{}); ok {
		if generatedImages, exists := usageData["generated_images"]; exists {
			count := int(generatedImages.(float64))
			usage.TotalTokens = count
			usage.PromptTokens = count
			c.Set("generated_images_count", count)
		}
		if outputTokens, exists := usageData["output_tokens"]; exists {
			usage.CompletionTokens = int(outputTokens.(float64))
		}
		if totalTokens, exists := usageData["total_tokens"]; exists && usage.TotalTokens == 0 {
			usage.TotalTokens = int(totalTokens.(float64))
			usage.PromptTokens = usage.TotalTokens
		}
	} else if len(imageResponse.Data) > 0 {
		usage.TotalTokens = len(imageResponse.Data)
		usage.PromptTokens = len(imageResponse.Data)
	}
	imageResponse.Usage = usage

	// 序列化响应
	jsonResponse, err := json.Marshal(imageResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = c.Writer.Write(jsonResponse)

	return usage, nil
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	// 检查是否为图片生成接口
	if info.RelayMode == constant.RelayModeImagesGenerations {
		return a.doubaoImageHandler(c, resp, info)
	}

	adaptor := openai.Adaptor{}
	usage, err = adaptor.DoResponse(c, resp, info)
	return
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
