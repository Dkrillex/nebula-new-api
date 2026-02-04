package ali

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/service"
	"one-api/types"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func oaiImage2Ali(request dto.ImageRequest) (*AliImageRequest, error) {
	ctx := context.Background()
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat

	common.SysLog("=== oaiImage2Ali V2 STARTED ===")

	// 截断 base64 数据以避免日志过长
	truncatedExtra := make(map[string]json.RawMessage)
	for k, v := range request.Extra {
		if len(v) > 200 {
			truncatedExtra[k] = json.RawMessage(fmt.Sprintf("\"%s...[truncated, length=%d]\"", string(v[:100]), len(v)))
		} else {
			truncatedExtra[k] = v
		}
	}
	logger.LogJson(ctx, "oaiImage2Ali request extra (truncated)", truncatedExtra)

	// 用于标记是否从 extra 中成功解析了参数
	hasExtraParams := false
	hasExtraInput := false

	if request.Extra != nil {
		common.SysLog(fmt.Sprintf("oaiImage2Ali: request.Extra is not nil, has %d keys", len(request.Extra)))

		// 检查是否有嵌套的 extra 字段（前端发送的格式）
		var extraData map[string]interface{}
		if nestedExtraRaw, ok := request.Extra["extra"]; ok {
			common.SysLog("oaiImage2Ali: Found nested 'extra' field")
			// 将 json.RawMessage 反序列化为 map
			var nestedExtra map[string]interface{}
			if err := common.Unmarshal(nestedExtraRaw, &nestedExtra); err == nil {
				extraData = nestedExtra
				common.SysLog("oaiImage2Ali: Successfully parsed nested extra")
			} else {
				common.SysLog(fmt.Sprintf("oaiImage2Ali: Failed to unmarshal nested extra: %v", err))
			}
		}

		// 如果没有嵌套的 extra 或解析失败，尝试直接从 request.Extra 解析
		if extraData == nil {
			common.SysLog("oaiImage2Ali: No nested 'extra' or unmarshal failed, parsing request.Extra directly")
			// 直接尝试从 request.Extra 中解析 parameters 和 input
			if parametersRaw, ok := request.Extra["parameters"]; ok {
				var params interface{}
				if err := common.Unmarshal(parametersRaw, &params); err == nil {
					imageRequest.Parameters = params
					hasExtraParams = true
					common.SysLog("oaiImage2Ali: Found parameters directly in request.Extra")
					logger.LogJson(ctx, "oaiImage2Ali parsed parameters", params)
				}
			}

			if inputRaw, ok := request.Extra["input"]; ok {
				var input interface{}
				if err := common.Unmarshal(inputRaw, &input); err == nil {
					imageRequest.Input = input
					hasExtraInput = true
					common.SysLog("oaiImage2Ali: Found input directly in request.Extra")
				}
			}
		} else {
			// 从嵌套的 extraData 中提取 parameters
			if val, ok := extraData["parameters"]; ok {
				imageRequest.Parameters = val
				hasExtraParams = true
				common.SysLog(fmt.Sprintf("oaiImage2Ali: Found parameters in nested extra, hasExtraParams=%v", hasExtraParams))
				logger.LogJson(ctx, "oaiImage2Ali parsed parameters", val)
			} else {
				common.SysLog("oaiImage2Ali: No parameters key in nested extraData")
			}

			// 从嵌套的 extraData 中提取 input
			if val, ok := extraData["input"]; ok {
				imageRequest.Input = val
				hasExtraInput = true
				common.SysLog(fmt.Sprintf("oaiImage2Ali: Found input in nested extra, hasExtraInput=%v", hasExtraInput))
			} else {
				common.SysLog("oaiImage2Ali: No input key in nested extraData")
			}
		}
	} else {
		common.SysLog("oaiImage2Ali: request.Extra is nil")
	}

	// 处理 contents 字段（qwen-image-edit 使用这种格式）
	if contentsRaw, ok := request.Extra["contents"]; ok && !hasExtraInput {
		common.SysLog("oaiImage2Ali: Found contents field in Extra")

		// 解析 contents 数组
		var contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Image string `json:"image,omitempty"`
				Text  string `json:"text,omitempty"`
			} `json:"parts"`
		}

		if err := common.Unmarshal(contentsRaw, &contents); err == nil && len(contents) > 0 {
			common.SysLog(fmt.Sprintf("oaiImage2Ali: Successfully parsed contents, length=%d", len(contents)))

			// 构造 messages 数组
			var messages []map[string]interface{}

			for _, content := range contents {
				message := map[string]interface{}{
					"role": content.Role,
				}

				// 构造 content 数组
				var contentArray []map[string]interface{}

				for _, part := range content.Parts {
					if part.Image != "" {
						// 截断图片数据的日志输出
						imagePreview := part.Image
						if len(imagePreview) > 100 {
							imagePreview = imagePreview[:100] + "...[truncated]"
						}
						common.SysLog(fmt.Sprintf("oaiImage2Ali: Found image in contents: %s", imagePreview))
						contentArray = append(contentArray, map[string]interface{}{
							"image": part.Image,
						})
					}
					if part.Text != "" {
						common.SysLog(fmt.Sprintf("oaiImage2Ali: Found text in contents: %s", part.Text))
						contentArray = append(contentArray, map[string]interface{}{
							"text": part.Text,
						})
					}
				}

				message["content"] = contentArray
				messages = append(messages, message)
			}

			// 设置 input
			imageRequest.Input = map[string]interface{}{
				"messages": messages,
			}
			hasExtraInput = true
			common.SysLog(fmt.Sprintf("oaiImage2Ali: Constructed input from contents, messages count=%d", len(messages)))
		} else {
			common.SysLog(fmt.Sprintf("oaiImage2Ali: Failed to parse contents or empty: err=%v", err))
		}
	}

	// 只有在 extra 中没有提供参数时才使用兜底逻辑
	if !hasExtraParams {
		common.SysLog("oaiImage2Ali: Using fallback parameters")
		imageRequest.Parameters = AliImageParameters{
			Size:      strings.Replace(request.Size, "x", "*", -1),
			N:         int(request.N),
			Watermark: request.Watermark,
		}
	}

	if !hasExtraInput {
		common.SysLog("oaiImage2Ali: Using fallback input")
		imageRequest.Input = AliImageInput{
			Prompt: request.Prompt,
		}
	}

	common.SysLog(fmt.Sprintf("=== oaiImage2Ali V2 COMPLETED: hasExtraParams=%v, hasExtraInput=%v ===", hasExtraParams, hasExtraInput))

	// 只输出摘要信息，不输出完整的 base64 数据
	summary := map[string]interface{}{
		"model":          imageRequest.Model,
		"responseFormat": imageRequest.ResponseFormat,
		"hasParameters":  imageRequest.Parameters != nil,
		"hasInput":       imageRequest.Input != nil,
	}

	// 统计图片数量
	if inputMap, ok := imageRequest.Input.(map[string]interface{}); ok {
		if messages, ok := inputMap["messages"].([]map[string]interface{}); ok {
			imageCount := 0
			textCount := 0
			for _, msg := range messages {
				if content, ok := msg["content"].([]map[string]interface{}); ok {
					for _, item := range content {
						if _, hasImage := item["image"]; hasImage {
							imageCount++
						}
						if _, hasText := item["text"]; hasText {
							textCount++
						}
					}
				}
			}
			summary["imageCount"] = imageCount
			summary["textCount"] = textCount
			summary["messagesCount"] = len(messages)
		}
	}

	logger.LogJson(ctx, "oaiImage2Ali final imageRequest summary", summary)

	return &imageRequest, nil
}

func oaiFormEdit2AliImageEdit(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*AliImageRequest, error) {
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat

	mf := c.Request.MultipartForm
	if mf == nil {
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("failed to parse image edit form request: %w", err)
		}
		mf = c.Request.MultipartForm
	}

	var imageFiles []*multipart.FileHeader
	var exists bool

	// First check for standard "image" field
	if imageFiles, exists = mf.File["image"]; !exists || len(imageFiles) == 0 {
		// If not found, check for "image[]" field
		if imageFiles, exists = mf.File["image[]"]; !exists || len(imageFiles) == 0 {
			// If still not found, iterate through all fields to find any that start with "image["
			foundArrayImages := false
			for fieldName, files := range mf.File {
				if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
					foundArrayImages = true
					imageFiles = append(imageFiles, files...)
				}
			}

			// If no image fields found at all
			if !foundArrayImages && (len(imageFiles) == 0) {
				return nil, errors.New("image is required")
			}
		}
	}

	if len(imageFiles) == 0 {
		return nil, errors.New("image is required")
	}

	if len(imageFiles) > 1 {
		return nil, errors.New("only one image is supported for qwen edit")
	}

	// 获取base64编码的图片
	var imageBase64s []string
	for _, file := range imageFiles {
		image, err := file.Open()
		if err != nil {
			return nil, errors.New("failed to open image file")
		}

		// 读取文件内容
		imageData, err := io.ReadAll(image)
		if err != nil {
			return nil, errors.New("failed to read image file")
		}

		// 获取MIME类型
		mimeType := http.DetectContentType(imageData)

		// 编码为base64
		base64Data := base64.StdEncoding.EncodeToString(imageData)

		// 构造data URL格式
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
		imageBase64s = append(imageBase64s, dataURL)
		image.Close()
	}

	//dto.MediaContent{}
	mediaContents := make([]AliMediaContent, len(imageBase64s))
	for i, b64 := range imageBase64s {
		mediaContents[i] = AliMediaContent{
			Image: b64,
		}
	}
	mediaContents = append(mediaContents, AliMediaContent{
		Text: request.Prompt,
	})
	imageRequest.Input = AliImageInput{
		Messages: []AliMessage{
			{
				Role:    "user",
				Content: mediaContents,
			},
		},
	}
	imageRequest.Parameters = AliImageParameters{
		Watermark: request.Watermark,
	}
	return &imageRequest, nil
}

func updateTask(info *relaycommon.RelayInfo, taskID string) (*AliResponse, error, []byte) {
	url := fmt.Sprintf("%s/api/v1/tasks/%s", info.ChannelBaseUrl, taskID)

	var aliResponse AliResponse

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &aliResponse, err, nil
	}

	req.Header.Set("Authorization", "Bearer "+info.ApiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		common.SysLog("updateTask client.Do err: " + err.Error())
		return &aliResponse, err, nil
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)

	var response AliResponse
	err = common.Unmarshal(responseBody, &response)
	if err != nil {
		common.SysLog("updateTask NewDecoder err: " + err.Error())
		return &aliResponse, err, nil
	}

	return &response, nil, responseBody
}

func asyncTaskWait(c *gin.Context, info *relaycommon.RelayInfo, taskID string) (*AliResponse, []byte, error) {
	waitSeconds := 10
	step := 0
	maxStep := 20

	var taskResponse AliResponse
	var responseBody []byte

	for {
		logger.LogDebug(c, fmt.Sprintf("asyncTaskWait step %d/%d, wait %d seconds", step, maxStep, waitSeconds))
		step++
		rsp, err, body := updateTask(info, taskID)
		responseBody = body
		if err != nil {
			logger.LogWarn(c, "asyncTaskWait UpdateTask err: "+err.Error())
			time.Sleep(time.Duration(waitSeconds) * time.Second)
			continue
		}

		if rsp.Output.TaskStatus == "" {
			return &taskResponse, responseBody, nil
		}

		switch rsp.Output.TaskStatus {
		case "FAILED":
			fallthrough
		case "CANCELED":
			fallthrough
		case "SUCCEEDED":
			fallthrough
		case "UNKNOWN":
			return rsp, responseBody, nil
		}
		if step >= maxStep {
			break
		}
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}

	return nil, nil, fmt.Errorf("aliAsyncTaskWait timeout")
}

func responseAli2OpenAIImage(c *gin.Context, response *AliResponse, originBody []byte, info *relaycommon.RelayInfo, responseFormat string) *dto.ImageResponse {
	// 初始化 Data 数组，确保不为 nil
	imageResponse := dto.ImageResponse{
		Created: info.StartTime.Unix(),
		Data:    make([]dto.ImageData, 0),
	}

	// qwen-image-plus 和 qwen-image-edit 系列使用新 API 格式：output.choices[].message.content[].image
	if (info.OriginModelName == "qwen-image-plus" || strings.HasPrefix(info.OriginModelName, "qwen-image-edit")) && len(response.Output.Choices) > 0 {
		common.SysLog(fmt.Sprintf("responseAli2OpenAIImage: Processing %s choices format", info.OriginModelName))
		logger.LogDebug(c, fmt.Sprintf("%s: Found %d choices", info.OriginModelName, len(response.Output.Choices)))
		for _, choice := range response.Output.Choices {
			if message, ok := choice["message"].(map[string]interface{}); ok {
				if content, ok := message["content"].([]interface{}); ok {
					logger.LogDebug(c, fmt.Sprintf("%s: Processing message with %d content items", info.OriginModelName, len(content)))
					for _, item := range content {
						if contentItem, ok := item.(map[string]interface{}); ok {
							if imageUrl, ok := contentItem["image"].(string); ok && imageUrl != "" {
								var b64Json string
								if responseFormat == "b64_json" {
									_, b64, err := service.GetImageFromUrl(imageUrl)
									if err != nil {
										logger.LogError(c, "get_image_data_failed: "+err.Error())
										continue
									}
									b64Json = b64
								}

								imageResponse.Data = append(imageResponse.Data, dto.ImageData{
									Url:           imageUrl,
									B64Json:       b64Json,
									RevisedPrompt: "",
								})
								logger.LogDebug(c, fmt.Sprintf("%s: Added image from choices: %s", info.OriginModelName, imageUrl))
							}
						}
					}
				}
			}
		}
		common.SysLog(fmt.Sprintf("responseAli2OpenAIImage: %s processed %d images", info.OriginModelName, len(imageResponse.Data)))
	} else {
		// 旧 API 格式：output.results[].url
		common.SysLog("responseAli2OpenAIImage: Processing legacy results format")
		for _, data := range response.Output.Results {
			var b64Json string
			if responseFormat == "b64_json" {
				_, b64, err := service.GetImageFromUrl(data.Url)
				if err != nil {
					logger.LogError(c, "get_image_data_failed: "+err.Error())
					continue
				}
				b64Json = b64
			} else {
				b64Json = data.B64Image
			}

			imageResponse.Data = append(imageResponse.Data, dto.ImageData{
				Url:           data.Url,
				B64Json:       b64Json,
				RevisedPrompt: "",
			})
		}
	}

	// 将原始响应放在 Metadata 字段中（厂家原始数据）
	var mapResponse map[string]any
	_ = common.Unmarshal(originBody, &mapResponse)
	imageResponse.Metadata = mapResponse

	// 记录最终响应信息
	if mapResponse != nil {
		keys := make([]string, 0, len(mapResponse))
		for k := range mapResponse {
			keys = append(keys, k)
		}
		logger.LogDebug(c, fmt.Sprintf("responseAli2OpenAIImage: Final response with %d images, Metadata keys: %v", len(imageResponse.Data), keys))
	} else {
		logger.LogDebug(c, fmt.Sprintf("responseAli2OpenAIImage: Final response with %d images, Metadata is nil", len(imageResponse.Data)))
	}

	return &imageResponse
}

func aliImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	responseFormat := c.GetString("response_format")

	var aliTaskResponse AliResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	service.CloseResponseBodyGracefully(resp)
	err = common.Unmarshal(responseBody, &aliTaskResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}

	if aliTaskResponse.Message != "" {
		logger.LogError(c, "ali_async_task_failed: "+aliTaskResponse.Message)
		return types.NewError(errors.New(aliTaskResponse.Message), types.ErrorCodeBadResponse), nil
	}

	var aliResponse *AliResponse
	var originRespBody []byte

	// qwen-image-plus 和 qwen-image-edit 系列使用新 API，同步返回结果，不需要异步轮询
	if info.OriginModelName == "qwen-image-plus" || strings.HasPrefix(info.OriginModelName, "qwen-image-edit") {
		common.SysLog(fmt.Sprintf("aliImageHandler: %s detected, using sync response", info.OriginModelName))
		logger.LogJson(c, fmt.Sprintf("%s raw response body", info.OriginModelName), string(responseBody))

		// 检查响应是否包含错误
		if aliTaskResponse.Code != "" || aliTaskResponse.Message != "" {
			common.SysLog(fmt.Sprintf("%s error response: code=%s, message=%s", info.OriginModelName, aliTaskResponse.Code, aliTaskResponse.Message))
			return types.NewError(fmt.Errorf("%s: %s", aliTaskResponse.Code, aliTaskResponse.Message), types.ErrorCodeBadResponse), nil
		}

		// 直接使用初始响应，不进行异步轮询
		aliResponse = &aliTaskResponse
		originRespBody = responseBody
		logger.LogJson(c, fmt.Sprintf("%s sync response output", info.OriginModelName), aliResponse.Output)
		logger.LogJson(c, fmt.Sprintf("%s sync response usage", info.OriginModelName), aliResponse.Usage)
	} else {
		// 其他模型使用异步模式
		common.SysLog("aliImageHandler: using async task wait")
		aliResponse, originRespBody, err = asyncTaskWait(c, info, aliTaskResponse.Output.TaskId)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponse), nil
		}

		if aliResponse.Output.TaskStatus != "SUCCEEDED" {
			return types.WithOpenAIError(types.OpenAIError{
				Message: aliResponse.Output.Message,
				Type:    "ali_error",
				Param:   "",
				Code:    aliResponse.Output.Code,
			}, resp.StatusCode), nil
		}
	}

	fullTextResponse := responseAli2OpenAIImage(c, aliResponse, originRespBody, info, responseFormat)

	// 提取图片数量用于计费并写入响应的 usage（与对话接口一致）
	usage := &dto.Usage{}
	if aliResponse.Usage.ImageCount > 0 {
		usage.TotalTokens = aliResponse.Usage.ImageCount
		usage.PromptTokens = aliResponse.Usage.ImageCount
		c.Set("generated_images_count", aliResponse.Usage.ImageCount)
		logger.LogDebug(c, fmt.Sprintf("[aliImageHandler] 提取图片数量: %d", aliResponse.Usage.ImageCount))
	} else {
		imageCount := len(fullTextResponse.Data)
		if imageCount > 0 {
			usage.TotalTokens = imageCount
			usage.PromptTokens = imageCount
			c.Set("generated_images_count", imageCount)
			logger.LogDebug(c, fmt.Sprintf("[aliImageHandler] 使用生成图片数量作为兜底: %d", imageCount))
		}
	}
	fullTextResponse.Usage = usage

	// 记录最终响应信息
	common.SysLog(fmt.Sprintf("aliImageHandler: Final ImageResponse - Created: %d, Data count: %d",
		fullTextResponse.Created, len(fullTextResponse.Data)))
	if len(fullTextResponse.Data) > 0 {
		logger.LogDebug(c, fmt.Sprintf("aliImageHandler: First image URL: %s", fullTextResponse.Data[0].Url))
	}

	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}

	logger.LogDebug(c, fmt.Sprintf("aliImageHandler: Response JSON length: %d bytes", len(jsonResponse)))
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return nil, usage
}

func aliImageEditHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NewAPIError, *dto.Usage) {
	var aliResponse AliResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}

	service.CloseResponseBodyGracefully(resp)
	err = common.Unmarshal(responseBody, &aliResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}

	if aliResponse.Message != "" {
		logger.LogError(c, "ali_task_failed: "+aliResponse.Message)
		return types.NewError(errors.New(aliResponse.Message), types.ErrorCodeBadResponse), nil
	}
	var fullTextResponse dto.ImageResponse
	if len(aliResponse.Output.Choices) > 0 {
		fullTextResponse = dto.ImageResponse{
			Created: info.StartTime.Unix(),
			Data: []dto.ImageData{
				{
					Url:     aliResponse.Output.Choices[0]["message"].(map[string]any)["content"].([]any)[0].(map[string]any)["image"].(string),
					B64Json: "",
				},
			},
		}
	}

	var mapResponse map[string]any
	_ = common.Unmarshal(responseBody, &mapResponse)
	fullTextResponse.Metadata = mapResponse

	// 提取图片数量用于计费并写入响应的 usage（与对话接口一致）
	usage := &dto.Usage{}
	if aliResponse.Usage.ImageCount > 0 {
		usage.TotalTokens = aliResponse.Usage.ImageCount
		usage.PromptTokens = aliResponse.Usage.ImageCount
		c.Set("generated_images_count", aliResponse.Usage.ImageCount)
		logger.LogDebug(c, fmt.Sprintf("[aliImageEditHandler] 提取图片数量: %d", aliResponse.Usage.ImageCount))
	} else {
		imageCount := len(fullTextResponse.Data)
		if imageCount > 0 {
			usage.TotalTokens = imageCount
			usage.PromptTokens = imageCount
			c.Set("generated_images_count", imageCount)
			logger.LogDebug(c, fmt.Sprintf("[aliImageEditHandler] 使用生成图片数量作为兜底: %d", imageCount))
		}
	}
	fullTextResponse.Usage = usage

	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return nil, usage
}
