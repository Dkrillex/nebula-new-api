package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"one-api/common"
	"one-api/constant"
	"one-api/dto"
	relaycommon "one-api/relay/common"
	"one-api/relay/helper"
	"one-api/service"
	"one-api/setting/model_setting"
	"one-api/types"
	"strings"

	"github.com/gin-gonic/gin"
)

// modelsNotSupportingContextManagement 不支持 context_management 的 Claude 模型（传入会触发 API 400: "Extra inputs are not permitted"）
var modelsNotSupportingContextManagement = map[string]bool{
	"claude-opus-4-6":    true,
	"claude-opus-4-6-v1": true,
}

// stripContextManagementForUnsupportedModels 当模型不支持 context_management 时从请求 JSON 中移除该字段，避免上游返回 400
func stripContextManagementForUnsupportedModels(jsonData []byte, model string) []byte {
	if !modelsNotSupportingContextManagement[model] {
		return jsonData
	}
	var data map[string]interface{}
	if err := common.Unmarshal(jsonData, &data); err != nil {
		return jsonData
	}
	delete(data, "context_management")
	out, err := common.Marshal(data)
	if err != nil {
		return jsonData
	}
	return out
}

func ClaudeHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {

	info.InitChannelMeta(c)

	claudeReq, ok := info.Request.(*dto.ClaudeRequest)

	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected *dto.ClaudeRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(claudeReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ClaudeRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
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

	if request.MaxTokens == 0 {
		request.MaxTokens = uint(model_setting.GetClaudeSettings().GetDefaultMaxTokens(request.Model))
	}

	if model_setting.GetClaudeSettings().ThinkingAdapterEnabled &&
		strings.HasSuffix(request.Model, "-thinking") {
		if request.Thinking == nil {
			// 因为BudgetTokens 必须大于1024
			if request.MaxTokens < 1280 {
				request.MaxTokens = 1280
			}

			// BudgetTokens 为 max_tokens 的 80%
			budgetTokens := int(float64(request.MaxTokens) * model_setting.GetClaudeSettings().ThinkingAdapterBudgetTokensPercentage)

			// AWS Bedrock 要求 max_tokens 必须严格大于 budget_tokens
			// 如果 budget_tokens >= max_tokens，则调整 max_tokens 使其至少比 budget_tokens 大 1
			if budgetTokens >= int(request.MaxTokens) {
				request.MaxTokens = uint(budgetTokens + 1)
			}

			request.Thinking = &dto.Thinking{
				Type:         "enabled",
				BudgetTokens: common.GetPointer[int](budgetTokens),
			}
			// TODO: 临时处理
			// https://docs.anthropic.com/en/docs/build-with-claude/extended-thinking#important-considerations-when-using-extended-thinking
			request.TopP = 0
			request.Temperature = common.GetPointer[float64](1.0)
		}
		request.Model = strings.TrimSuffix(request.Model, "-thinking")
		info.UpstreamModelName = request.Model
	}

	// 统一检查：无论 thinking 是从哪里来的（用户传入、-thinking 后缀等），都要确保 max_tokens > budget_tokens
	// 这个要求适用于所有 Claude API（Anthropic 官方 API 和 AWS Bedrock）
	// budget_tokens 是思考预算，必须小于 max_tokens（总输出限制）
	if request.Thinking != nil && request.Thinking.BudgetTokens != nil {
		budgetTokens := *request.Thinking.BudgetTokens
		// Claude API 要求 max_tokens 必须严格大于 budget_tokens
		// 如果 budget_tokens >= max_tokens，则调整 max_tokens 使其至少比 budget_tokens 大 1
		if budgetTokens >= int(request.MaxTokens) {
			request.MaxTokens = uint(budgetTokens + 1)
		}
	}

	if info.ChannelSetting.SystemPrompt != "" {
		if request.System == nil {
			request.SetStringSystem(info.ChannelSetting.SystemPrompt)
		} else if info.ChannelSetting.SystemPromptOverride {
			common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
			if request.IsStringSystem() {
				existing := strings.TrimSpace(request.GetStringSystem())
				if existing == "" {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt)
				} else {
					request.SetStringSystem(info.ChannelSetting.SystemPrompt + "\n" + existing)
				}
			} else {
				systemContents := request.ParseSystem()
				newSystem := dto.ClaudeMediaMessage{Type: dto.ContentTypeText}
				newSystem.SetText(info.ChannelSetting.SystemPrompt)
				if len(systemContents) == 0 {
					request.System = []dto.ClaudeMediaMessage{newSystem}
				} else {
					request.System = append([]dto.ClaudeMediaMessage{newSystem}, systemContents...)
				}
			}
		}
	}

	var requestBody io.Reader
	if model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled {
		body, err := common.GetRequestBody(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		body = stripContextManagementForUnsupportedModels(body, request.Model)
		requestBody = bytes.NewBuffer(body)
	} else {
		convertedRequest, err := adaptor.ConvertClaudeRequest(c, info, request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		jsonData, err := common.Marshal(convertedRequest)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}

		// remove disabled fields for Claude API
		jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings)
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

		// 最终校验：参数覆盖可能导致 max_tokens <= thinking.budget_tokens
		// 这个约束对 Anthropic 官方 API 和 AWS Bedrock 都成立（Bedrock 会直接 400）
		var finalReq dto.ClaudeRequest
		if err := common.Unmarshal(jsonData, &finalReq); err == nil {
			if finalReq.Thinking != nil && finalReq.Thinking.BudgetTokens != nil {
				budgetTokens := *finalReq.Thinking.BudgetTokens
				if finalReq.MaxTokens == 0 {
					finalReq.MaxTokens = uint(budgetTokens + 100)
				} else if budgetTokens >= int(finalReq.MaxTokens) {
					finalReq.MaxTokens = uint(budgetTokens + 1)
				}
				if patched, mErr := common.Marshal(finalReq); mErr == nil {
					jsonData = patched
				}
			}
		}

		jsonData = stripContextManagementForUnsupportedModels(jsonData, request.Model)

		if common.DebugEnabled {
			truncatedBody := common.TruncateJsonValues(string(jsonData))
			common.SysLog(fmt.Sprintf("requestBody: %s", truncatedBody))
		}
		requestBody = bytes.NewBuffer(jsonData)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")
	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}

	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
			// reset status code 重置状态码
			service.ResetStatusCode(newAPIError, statusCodeMappingStr)
			return newAPIError
		}
	}

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	//log.Printf("usage: %v", usage)
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return newAPIError
	}

	service.PostClaudeConsumeQuota(c, info, usage.(*dto.Usage))
	return nil
}
