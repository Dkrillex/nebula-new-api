package model

import "encoding/json"

// TaskMediaInfo 记录异步任务关联的 OSS 文件信息。
type TaskMediaInfo struct {
	OssID    string `json:"ossId"`
	OssURL   string `json:"ossUrl"`
	FileName string `json:"fileName,omitempty"`
}

// ParseTaskMediaInfo 尝试解析 fail_reason 中的 OSS 文件列表。
func ParseTaskMediaInfo(raw string) ([]TaskMediaInfo, bool) {
	if raw == "" {
		return nil, false
	}

	var list []TaskMediaInfo
	if err := json.Unmarshal([]byte(raw), &list); err == nil && len(list) > 0 {
		return list, true
	}

	var wrapper struct {
		Files []TaskMediaInfo `json:"files"`
	}
	if err := json.Unmarshal([]byte(raw), &wrapper); err == nil && len(wrapper.Files) > 0 {
		return wrapper.Files, true
	}

	return nil, false
}

// SerializeTaskMediaInfo 将 OSS 文件列表序列化为 JSON 字符串。
func SerializeTaskMediaInfo(list []TaskMediaInfo) (string, error) {
	if len(list) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
