package ali

// SubmitRequest 提交任务请求
type SubmitRequest struct {
	Model      string     `json:"model"`
	Input      Input      `json:"input"`
	Parameters Parameters `json:"parameters,omitempty"`
}

// Input 输入参数
type Input struct {
	Prompt   string `json:"prompt"`
	ImgURL   string `json:"img_url,omitempty"`   // 官方参数名：img_url
	AudioURL string `json:"audio_url,omitempty"` // 音频URL（可选）
}

// Parameters 生成参数
// 注意：i2v 和 t2v 模型的参数格式不同
// i2v: 使用 resolution (如 "720P")
// t2v: 使用 size (如 "1280*720")
type Parameters struct {
	Duration     int    `json:"duration,omitempty"`      // 5 or 10
	Resolution   string `json:"resolution,omitempty"`    // i2v 模型：480P/720P/1080P（官方使用大写P）
	Size         string `json:"size,omitempty"`          // t2v 模型：具体分辨率（如 "1280*720"）
	Seed         int    `json:"seed,omitempty"`          // 随机种子
	PromptExtend bool   `json:"prompt_extend,omitempty"` // 官方参数名：prompt_extend（提示词扩写）
	Audio        bool   `json:"audio,omitempty"`         // 官方参数名：audio（生成音频）
}

// SubmitResponse 提交任务响应
type SubmitResponse struct {
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Output     struct {
		TaskID string `json:"task_id"`
	} `json:"output"`
}

// TaskResponse 轮询任务响应
type TaskResponse struct {
	StatusCode int    `json:"status_code"`
	RequestID  string `json:"request_id"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	Output     struct {
		TaskID        string `json:"task_id"`
		TaskStatus    string `json:"task_status"` // PENDING/RUNNING/SUCCEEDED/FAILED
		VideoURL      string `json:"video_url,omitempty"`
		SubmitTime    string `json:"submit_time"`
		ScheduledTime string `json:"scheduled_time,omitempty"`
		EndTime       string `json:"end_time,omitempty"`
		OrigPrompt    string `json:"orig_prompt,omitempty"`
		ActualPrompt  string `json:"actual_prompt,omitempty"`
	} `json:"output"`
	Usage Usage `json:"usage"`
}

// Usage 使用量信息
// 注意：i2v 和 t2v 模型的返回格式不同
// i2v: { "SR": 720, "duration": 5, "video_count": 1 }
// t2v: { "video_duration": 10, "video_ratio": "832*480", "video_count": 1 }
type Usage struct {
	VideoCount    int    `json:"video_count"`    // 视频数量，固定为1
	Duration      int    `json:"duration"`       // 视频时长（秒）：i2v 模型使用
	SR            int    `json:"SR"`             // 分辨率：i2v 模型使用（480/720/1080）
	VideoDuration int    `json:"video_duration"` // 视频时长（秒）：t2v 模型使用
	VideoRatio    string `json:"video_ratio"`    // 分辨率：t2v 模型使用（如 "832*480"）
}
