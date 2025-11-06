package dto

import (
	"encoding/json"
	"fmt"
	"one-api/common"
	"one-api/types"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
)

type ImageRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt" binding:"required"`
	N                 uint            `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Style             json.RawMessage `json:"style,omitempty"`
	User              json.RawMessage `json:"user,omitempty"`
	ExtraFields       json.RawMessage `json:"extra_fields,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	Moderation        json.RawMessage `json:"moderation,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	PartialImages     json.RawMessage `json:"partial_images,omitempty"`
	InputFidelity     string          `json:"input_fidelity,omitempty"` // gpt-image-1 特有: low/medium/high
	Stream            *bool           `json:"stream,omitempty"`         // gpt-image-1 支持流式响应
	Watermark         *bool           `json:"watermark,omitempty"`
	// 用匿名参数接收额外参数
	Extra map[string]json.RawMessage `json:"-"`
}

func (i *ImageRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]interface{}
	var rawMap map[string]json.RawMessage
	if err := common.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 调试日志
	allKeys := make([]string, 0, len(rawMap))
	for k := range rawMap {
		allKeys = append(allKeys, k)
	}
	common.SysLog(fmt.Sprintf("[ImageRequest] UnmarshalJSON 收到的所有字段: %v", allKeys))

	// 用 struct tag 获取所有已定义字段名
	knownFields := GetJSONFieldNames(reflect.TypeOf(*i))

	// 再正常解析已定义字段
	type Alias ImageRequest
	var known Alias
	if err := common.Unmarshal(data, &known); err != nil {
		return err
	}
	*i = ImageRequest(known)

	// 提取多余字段
	i.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			i.Extra[k] = v
		}
	}

	// 调试日志
	if len(i.Extra) > 0 {
		extraKeys := make([]string, 0, len(i.Extra))
		for k := range i.Extra {
			extraKeys = append(extraKeys, k)
		}
		common.SysLog(fmt.Sprintf("[ImageRequest] UnmarshalJSON Extra keys: %v", extraKeys))
	} else {
		common.SysLog("[ImageRequest] UnmarshalJSON Extra is empty")
	}

	return nil
}

// 序列化时需要重新把字段平铺
func (r ImageRequest) MarshalJSON() ([]byte, error) {
	// 将已定义字段转为 map
	type Alias ImageRequest
	alias := Alias(r)
	base, err := common.Marshal(alias)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]json.RawMessage
	if err := common.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}

	// 合并 Extra 字段到最终的 JSON 中
	// 只添加不存在的字段，避免覆盖已定义字段
	for k, v := range r.Extra {
		if _, exists := baseMap[k]; !exists {
			baseMap[k] = v
		}
	}

	return common.Marshal(baseMap)
}

func GetJSONFieldNames(t reflect.Type) map[string]struct{} {
	fields := make(map[string]struct{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过匿名字段（例如 ExtraFields）
		if field.Anonymous {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}

		// 取逗号前字段名（排除 omitempty 等）
		name := tag
		if commaIdx := indexComma(tag); commaIdx != -1 {
			name = tag[:commaIdx]
		}
		fields[name] = struct{}{}
	}
	return fields
}

func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}

func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0

	if strings.HasPrefix(i.Model, "dall-e") {
		// Size
		if i.Size == "256x256" {
			sizeRatio = 0.4
		} else if i.Size == "512x512" {
			sizeRatio = 0.45
		} else if i.Size == "1024x1024" {
			sizeRatio = 1
		} else if i.Size == "1024x1792" || i.Size == "1792x1024" {
			sizeRatio = 2
		}

		if i.Model == "dall-e-3" && i.Quality == "hd" {
			qualityRatio = 2.0
			if i.Size == "1024x1792" || i.Size == "1792x1024" {
				qualityRatio = 1.5
			}
		}
	} else if strings.HasPrefix(i.Model, "gpt-image-1") {
		// gpt-image-1 的尺寸计费比率
		if i.Size == "1024x1024" {
			sizeRatio = 1.0
		} else if i.Size == "1024x1536" || i.Size == "1536x1024" {
			sizeRatio = 1.5
		}

		// gpt-image-1 的质量计费比率
		switch i.Quality {
		case "low":
			qualityRatio = 0.5
		case "medium":
			qualityRatio = 1.0
		case "high":
			qualityRatio = 2.0
		default:
			qualityRatio = 1.0
		}
	}

	// not support token count for dalle and gpt-image-1
	return &types.TokenCountMeta{
		CombineText:     i.Prompt,
		MaxTokens:       1584,
		ImagePriceRatio: sizeRatio * qualityRatio * float64(i.N),
	}
}

func (i *ImageRequest) IsStream(c *gin.Context) bool {
	return false
}

func (i *ImageRequest) SetModelName(modelName string) {
	if modelName != "" {
		i.Model = modelName
	}
}

type ImageResponse struct {
	Data     []ImageData `json:"data"`
	Created  int64       `json:"created"`
	Metadata any         `json:"metadata,omitempty"` // 厂家原始响应数据
}
type ImageData struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}
