package dto

import (
	"encoding/json"
	"fmt"
	"one-api/common"
)

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
	InputFidelity  string `json:"input_fidelity,omitempty"` // gpt-image-1 图生图特有参数
	// 用匿名参数接收额外参数，支持大模型私有参数（包括contents、image等）
	Extra map[string]json.RawMessage `json:"-"`
}

// SyncVideoGenerationRequest 外部系统视频生成请求结构体
type SyncVideoGenerationRequest struct {
	UserId         int     `json:"user_id" binding:"required"`
	Model          string  `json:"model" binding:"required"`
	Group          string  `json:"group"`
	Prompt         string  `json:"prompt" binding:"required"`
	Image          string  `json:"image,omitempty"`
	Duration       float64 `json:"duration,omitempty"`
	Width          int     `json:"width,omitempty"`
	Height         int     `json:"height,omitempty"`
	Fps            int     `json:"fps,omitempty"`
	Seed           int     `json:"seed,omitempty"`
	N              uint    `json:"n,omitempty"`
	ResponseFormat string  `json:"response_format,omitempty"`
	// 用匿名参数接收额外参数，支持厂商特定参数（metadata等）
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON 自定义JSON解析，提取额外字段到Extra中
func (r *SyncImageGenerationRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]json.RawMessage
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 调试日志：打印所有收到的字段
	allKeys := make([]string, 0, len(rawMap))
	for k := range rawMap {
		allKeys = append(allKeys, k)
	}
	common.SysLog(fmt.Sprintf("[SyncImageGenerationRequest] UnmarshalJSON 收到的所有字段: %v", allKeys))

	// 兼容前端使用驼峰写法的 responseFormat -> 统一映射到 response_format
	if v, ok := rawMap["responseFormat"]; ok {
		if _, exists := rawMap["response_format"]; !exists {
			rawMap["response_format"] = v
			common.SysLog("[SyncImageGenerationRequest] 兼容字段: responseFormat -> response_format")
		}
		// 保证后续 Extra 中不再出现重复的 responseFormat
		delete(rawMap, "responseFormat")
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
		"input_fidelity":  {}, // gpt-image-1 图生图参数
		// 注意：image 和 images 应该放入 Extra，不在此列表中
	}

	// 正常解析已定义字段
	type Alias SyncImageGenerationRequest
	var known Alias
	// 使用规范化后的 rawMap 重新反序列化，确保兼容字段能正确映射
	normalized, err := json.Marshal(rawMap)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(normalized, &known); err != nil {
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

	// 调试日志
	if len(r.Extra) > 0 {
		extraKeys := make([]string, 0, len(r.Extra))
		for k := range r.Extra {
			extraKeys = append(extraKeys, k)
		}
		common.SysLog(fmt.Sprintf("[SyncImageGenerationRequest] UnmarshalJSON Extra keys: %v", extraKeys))
	} else {
		common.SysLog("[SyncImageGenerationRequest] UnmarshalJSON Extra is empty")
	}

	return nil
}

// UnmarshalJSON 自定义JSON解析，提取额外字段到Extra中
func (r *SyncVideoGenerationRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]json.RawMessage
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 兼容前端使用驼峰写法的 responseFormat -> 统一映射到 response_format
	if v, ok := rawMap["responseFormat"]; ok {
		if _, exists := rawMap["response_format"]; !exists {
			rawMap["response_format"] = v
			common.SysLog("[SyncVideoGenerationRequest] 兼容字段: responseFormat -> response_format")
		}
		delete(rawMap, "responseFormat")
	}

	// 定义已知字段
	knownFields := map[string]struct{}{
		"user_id":         {},
		"model":           {},
		"group":           {},
		"prompt":          {},
		"image":           {},
		"duration":        {},
		"width":           {},
		"height":          {},
		"fps":             {},
		"seed":            {},
		"n":               {},
		"response_format": {},
	}

	// 正常解析已定义字段
	type Alias SyncVideoGenerationRequest
	var known Alias
	normalized, err := json.Marshal(rawMap)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(normalized, &known); err != nil {
		return err
	}
	*r = SyncVideoGenerationRequest(known)

	// 提取多余字段到Extra
	r.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			r.Extra[k] = v
		}
	}
	return nil
}
