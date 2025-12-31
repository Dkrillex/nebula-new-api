package gemini

import (
	"io"
	"net/http"
	"one-api/common"
	"one-api/dto"
	"one-api/logger"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/types"
	"strings"

	"github.com/pkg/errors"

	"github.com/gin-gonic/gin"
)

func GeminiTextGenerationHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	// 解析为 Gemini 原生响应格式
	var geminiResponse dto.GeminiChatResponse
	err = common.Unmarshal(responseBody, &geminiResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 从 CandidatesTokensDetails 中提取图片和文本输出 tokens
	var imageOutputTokens int
	var textOutputTokens int
	for _, detail := range geminiResponse.UsageMetadata.CandidatesTokensDetails {
		if detail.Modality == "IMAGE" {
			imageOutputTokens += detail.TokenCount
		} else if detail.Modality == "TEXT" {
			textOutputTokens += detail.TokenCount
		}
	}

	// 如果没有 CandidatesTokensDetails，尝试从内容中统计图片数量（作为后备）
	var imageCount int
	if imageOutputTokens == 0 {
		for _, candidate := range geminiResponse.Candidates {
			if len(candidate.Content.Parts) > 0 {
				for _, part := range candidate.Content.Parts {
					if part.InlineData != nil && part.InlineData.MimeType != "" {
						imageCount++
					}
				}
			}
		}
	}

	// 将图片信息存储到 context，供计费逻辑使用
	if imageOutputTokens > 0 || imageCount > 0 {
		c.Set("gemini_image_output_tokens", imageOutputTokens)
		c.Set("gemini_text_output_tokens", textOutputTokens)
		if imageCount > 0 {
			c.Set("gemini_image_output_count", imageCount)
		}
	}

	// 计算使用量（基于 UsageMetadata）
	usage := dto.Usage{
		PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
		CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount + geminiResponse.UsageMetadata.ThoughtsTokenCount,
		TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
	}

	usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount

	// 处理缓存 tokens（Context Caching）
	if geminiResponse.UsageMetadata.CachedContentTokenCount > 0 {
		usage.PromptTokensDetails.CachedTokens = geminiResponse.UsageMetadata.CachedContentTokenCount
	}

	for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
		if detail.Modality == "AUDIO" {
			usage.PromptTokensDetails.AudioTokens = detail.TokenCount
		} else if detail.Modality == "TEXT" {
			usage.PromptTokensDetails.TextTokens = detail.TokenCount
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return &usage, nil
}

func NativeGeminiEmbeddingHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if common.DebugEnabled {
		println(string(responseBody))
	}

	usage := &dto.Usage{
		PromptTokens: info.PromptTokens,
		TotalTokens:  info.PromptTokens,
	}

	if info.IsGeminiBatchEmbedding {
		var geminiResponse dto.GeminiBatchEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	} else {
		var geminiResponse dto.GeminiEmbeddingResponse
		err = common.Unmarshal(responseBody, &geminiResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)

	return usage, nil
}

func GeminiTextGenerationStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	var usage = &dto.Usage{}
	var imageCount int

	helper.SetEventStreamHeaders(c)

	responseText := strings.Builder{}

	helper.StreamScannerHandler(c, resp, info, func(data string) bool {
		var geminiResponse dto.GeminiChatResponse
		err := common.UnmarshalJsonStr(data, &geminiResponse)
		if err != nil {
			logger.LogError(c, "error unmarshalling stream response: "+err.Error())
			return false
		}

		// 统计图片数量和文本 tokens
		for _, candidate := range geminiResponse.Candidates {
			if len(candidate.Content.Parts) > 0 {
				for _, part := range candidate.Content.Parts {
					if part.InlineData != nil && part.InlineData.MimeType != "" {
						imageCount++
					}
					if part.Text != "" {
						responseText.WriteString(part.Text)
					}
				}
			}
		}

		// 更新使用量统计
		if geminiResponse.UsageMetadata.TotalTokenCount != 0 {
			usage.PromptTokens = geminiResponse.UsageMetadata.PromptTokenCount
			usage.CompletionTokens = geminiResponse.UsageMetadata.CandidatesTokenCount + geminiResponse.UsageMetadata.ThoughtsTokenCount
			usage.TotalTokens = geminiResponse.UsageMetadata.TotalTokenCount
			usage.CompletionTokenDetails.ReasoningTokens = geminiResponse.UsageMetadata.ThoughtsTokenCount

			// 处理缓存 tokens（Context Caching）
			if geminiResponse.UsageMetadata.CachedContentTokenCount > 0 {
				usage.PromptTokensDetails.CachedTokens = geminiResponse.UsageMetadata.CachedContentTokenCount
			}

			for _, detail := range geminiResponse.UsageMetadata.PromptTokensDetails {
				if detail.Modality == "AUDIO" {
					usage.PromptTokensDetails.AudioTokens = detail.TokenCount
				} else if detail.Modality == "TEXT" {
					usage.PromptTokensDetails.TextTokens = detail.TokenCount
				}
			}

			// 从 CandidatesTokensDetails 中提取图片和文本输出 tokens
			var imageOutputTokens int
			var textOutputTokens int
			for _, detail := range geminiResponse.UsageMetadata.CandidatesTokensDetails {
				if detail.Modality == "IMAGE" {
					imageOutputTokens += detail.TokenCount
				} else if detail.Modality == "TEXT" {
					textOutputTokens += detail.TokenCount
				}
			}

			// 更新全局变量（用于最终计算）
			if imageOutputTokens > 0 {
				c.Set("gemini_image_output_tokens", imageOutputTokens)
			}
			if textOutputTokens > 0 {
				c.Set("gemini_text_output_tokens", textOutputTokens)
			}
		}

		// 直接发送 GeminiChatResponse 响应
		err = helper.StringData(c, data)
		if err != nil {
			logger.LogError(c, err.Error())
		}
		info.SendResponseCount++
		return true
	})

	if info.SendResponseCount == 0 {
		return nil, types.NewOpenAIError(errors.New("no response received from Gemini API"), types.ErrorCodeEmptyResponse, http.StatusInternalServerError)
	}

	// 从 context 中获取已设置的图片和文本输出 tokens（在流式处理中已设置）
	imageOutputTokens := c.GetInt("gemini_image_output_tokens")
	textOutputTokens := c.GetInt("gemini_text_output_tokens")

	// 如果没有从 CandidatesTokensDetails 获取到，尝试从内容统计（作为后备）
	if imageOutputTokens == 0 && imageCount > 0 {
		// 后备方案：使用图片数量估算（但应该优先使用 API 返回的实际值）
		imageOutputTokens = imageCount * 258
	}

	if textOutputTokens == 0 && usage.CompletionTokens > 0 {
		// 如果没有单独的文本 tokens，从总 completion tokens 中减去图片 tokens
		if imageOutputTokens > 0 && usage.CompletionTokens >= imageOutputTokens {
			textOutputTokens = usage.CompletionTokens - imageOutputTokens
		} else {
			textOutputTokens = usage.CompletionTokens
		}
	}

	// 如果仍然没有，使用本地统计
	if textOutputTokens == 0 {
		str := responseText.String()
		if len(str) > 0 {
			textOutputTokens = service.CountTokenInput(str, info.UpstreamModelName)
		}
	}

	// 确保 context 中有正确的值
	if imageOutputTokens > 0 {
		c.Set("gemini_image_output_tokens", imageOutputTokens)
	}
	if textOutputTokens > 0 {
		c.Set("gemini_text_output_tokens", textOutputTokens)
	}

	// 如果usage.CompletionTokens为0，则使用本地统计的completion tokens
	if usage.CompletionTokens == 0 {
		str := responseText.String()
		if len(str) > 0 {
			usage = service.ResponseText2Usage(responseText.String(), info.UpstreamModelName, info.PromptTokens)
		} else {
			// 空补全，不需要使用量
			usage = &dto.Usage{}
		}
	}

	// 移除流式响应结尾的[Done]，因为Gemini API没有发送Done的行为
	//helper.Done(c)

	return usage, nil
}
