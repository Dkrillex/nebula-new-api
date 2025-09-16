package dto

import "encoding/json"

type PlayGroundRequest struct {
	Model string `json:"model,omitempty"`
	Group string `json:"group,omitempty"`
}

// SyncPlaygroundRequest 外部系统操练场请求结构体
type SyncPlaygroundRequest struct {
	UserId           int                      `json:"user_id" binding:"required"`
	Model            string                   `json:"model" binding:"required"`
	Group            string                   `json:"group,omitempty"`
	Messages         []map[string]interface{} `json:"messages" binding:"required"`
	Stream           bool                     `json:"stream,omitempty"`
	Temperature      float64                  `json:"temperature,omitempty"`
	TopP             float64                  `json:"top_p,omitempty"`
	FrequencyPenalty float64                  `json:"frequency_penalty,omitempty"`
	PresencePenalty  float64                  `json:"presence_penalty,omitempty"`
	MaxTokens        int                      `json:"max_tokens,omitempty"`
}

// SyncImageGenerationRequest 外部系统图片生成请求结构体
type SyncImageGenerationRequest struct {
	UserId         int    `json:"user_id" binding:"required"`
	Model          string `json:"model" binding:"required"`
	Group          string `json:"group"`
	Prompt         string `json:"prompt" binding:"required"`
	N              uint   `json:"n,omitempty"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	ResponseFormat string `json:"response_format,omitempty"`
	Style          string `json:"style,omitempty"`
	// 用匿名参数接收额外参数，支持大模型私有参数（包括contents等）
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON 自定义JSON解析，提取额外字段到Extra中
func (r *SyncImageGenerationRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]json.RawMessage
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 定义已知字段
	knownFields := map[string]struct{}{
		"user_id":         {},
		"model":           {},
		"group":           {},
		"prompt":          {},
		"n":               {},
		"size":            {},
		"quality":         {},
		"response_format": {},
		"style":           {},
	}

	// 正常解析已定义字段
	type Alias SyncImageGenerationRequest
	var known Alias
	if err := json.Unmarshal(data, &known); err != nil {
		return err
	}
	*r = SyncImageGenerationRequest(known)

	// 提取多余字段到Extra
	r.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			r.Extra[k] = v
		}
	}
	return nil
}
