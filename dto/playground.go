package dto

type PlayGroundRequest struct {
	Model string `json:"model,omitempty"`
	Group string `json:"group,omitempty"`
}

// SyncPlaygroundRequest 外部系统操练场请求结构体
type SyncPlaygroundRequest struct {
	UserId           int64                    `json:"user_id" binding:"required"`
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
