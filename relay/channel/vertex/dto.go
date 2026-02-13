package vertex

import (
	"one-api/dto"
	"strings"
)

type VertexAIClaudeRequest struct {
	AnthropicVersion string              `json:"anthropic_version"`
	Messages         []dto.ClaudeMessage `json:"messages"`
	System           any                 `json:"system,omitempty"`
	MaxTokens        uint                `json:"max_tokens,omitempty"`
	StopSequences    []string            `json:"stop_sequences,omitempty"`
	Stream           bool                `json:"stream,omitempty"`
	Temperature      *float64            `json:"temperature,omitempty"`
	TopP             float64             `json:"top_p,omitempty"`
	TopK             int                 `json:"top_k,omitempty"`
	Tools            any                 `json:"tools,omitempty"`
	ToolChoice       any                 `json:"tool_choice,omitempty"`
	Thinking         *dto.Thinking       `json:"thinking,omitempty"`
}

func copyRequest(req *dto.ClaudeRequest, version string) *VertexAIClaudeRequest {
	return &VertexAIClaudeRequest{
		AnthropicVersion: version,
		System:           req.System,
		Messages:         req.Messages,
		MaxTokens:        req.MaxTokens,
		Stream:           req.Stream,
		Temperature:      req.Temperature,
		TopP:             req.TopP,
		TopK:             req.TopK,
		StopSequences:    req.StopSequences,
		Tools:            req.Tools,
		ToolChoice:       req.ToolChoice,
		Thinking:         req.Thinking,
	}
}

// VertexPredictEmbeddingInstance is one instance for Vertex AI predict embedding API.
type VertexPredictEmbeddingInstance struct {
	Content  string `json:"content"`
	TaskType string `json:"task_type,omitempty"`
	Title    string `json:"title,omitempty"`
}

// VertexPredictEmbeddingRequest is the request body for Vertex AI :predict embedding.
type VertexPredictEmbeddingRequest struct {
	Instances []VertexPredictEmbeddingInstance `json:"instances"`
}

// taskTypeFromGemini converts Gemini taskType (camelCase) to Vertex snake_case.
func taskTypeFromGemini(taskType string) string {
	if taskType == "" {
		return ""
	}
	// RETRIEVAL_DOCUMENT, RETRIEVAL_QUERY, etc. are already uppercase with underscore
	if strings.Contains(taskType, "_") {
		return taskType
	}
	// camelCase to snake_case for known values
	switch taskType {
	case "RETRIEVAL_DOCUMENT", "RETRIEVAL_QUERY", "SEMANTIC_SIMILARITY", "CLASSIFICATION", "CLUSTERING":
		return taskType
	}
	return strings.ReplaceAll(taskType, " ", "_")
}

// GeminiEmbeddingToVertexPredictRequest converts Gemini native embedding request to Vertex predict format.
func GeminiEmbeddingToVertexPredictRequest(req dto.Request, isBatch bool) (*VertexPredictEmbeddingRequest, error) {
	var instances []VertexPredictEmbeddingInstance
	if isBatch {
		batchReq, ok := req.(*dto.GeminiBatchEmbeddingRequest)
		if !ok || batchReq == nil {
			return nil, nil
		}
		for _, r := range batchReq.Requests {
			instances = append(instances, geminiEmbeddingReqToInstance(r))
		}
	} else {
		singleReq, ok := req.(*dto.GeminiEmbeddingRequest)
		if !ok || singleReq == nil {
			return nil, nil
		}
		instances = []VertexPredictEmbeddingInstance{geminiEmbeddingReqToInstance(singleReq)}
	}
	return &VertexPredictEmbeddingRequest{Instances: instances}, nil
}

func geminiEmbeddingReqToInstance(r *dto.GeminiEmbeddingRequest) VertexPredictEmbeddingInstance {
	var content string
	for _, part := range r.Content.Parts {
		if part.Text != "" {
			content = part.Text
			break
		}
	}
	taskType := taskTypeFromGemini(r.TaskType)
	inst := VertexPredictEmbeddingInstance{Content: content, TaskType: taskType, Title: r.Title}
	return inst
}

// VertexPredictEmbeddingPrediction is one prediction item from Vertex AI :predict embedding response.
type VertexPredictEmbeddingPrediction struct {
	Embeddings struct {
		Values     []float64 `json:"values"`
		Statistics struct {
			Truncated  bool `json:"truncated"`
			TokenCount int  `json:"token_count"`
		} `json:"statistics"`
	} `json:"embeddings"`
}

// VertexPredictEmbeddingResponse is the response body from Vertex AI :predict embedding.
type VertexPredictEmbeddingResponse struct {
	Predictions []VertexPredictEmbeddingPrediction `json:"predictions"`
	Metadata    map[string]interface{}             `json:"metadata,omitempty"`
}
