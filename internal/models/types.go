package models

import (
	"errors"

	"github.com/google/uuid"
)

type ModelType string

const (
	ModelTypeChat      ModelType = "chat"
	ModelTypeEmbedding ModelType = "embedding"
)

type ClientType string

const (
	ClientTypeOpenAIResponses    ClientType = "openai-responses"
	ClientTypeOpenAICompletions  ClientType = "openai-completions"
	ClientTypeAnthropicMessages  ClientType = "anthropic-messages"
	ClientTypeGoogleGenerativeAI ClientType = "google-generative-ai"
	ClientTypeOpenAICodex        ClientType = "openai-codex"
)

const (
	CompatVision      = "vision"
	CompatToolCall    = "tool-call"
	CompatImageOutput = "image-output"
	CompatReasoning   = "reasoning"
)

const (
	ReasoningEffortNone   = "none"
	ReasoningEffortLow    = "low"
	ReasoningEffortMedium = "medium"
	ReasoningEffortHigh   = "high"
	ReasoningEffortXHigh  = "xhigh"
)

// validCompatibilities enumerates accepted compatibility tokens.
var validCompatibilities = map[string]struct{}{
	CompatVision: {}, CompatToolCall: {}, CompatImageOutput: {}, CompatReasoning: {},
}

var validReasoningEfforts = map[string]struct{}{
	ReasoningEffortNone:   {},
	ReasoningEffortLow:    {},
	ReasoningEffortMedium: {},
	ReasoningEffortHigh:   {},
	ReasoningEffortXHigh:  {},
}

// ModelConfig holds the JSONB config stored per model.
type ModelConfig struct {
	Dimensions       *int     `json:"dimensions,omitempty"`
	Compatibilities  []string `json:"compatibilities,omitempty"`
	ContextWindow    *int     `json:"context_window,omitempty"`
	ReasoningEfforts []string `json:"reasoning_efforts,omitempty"`
}

type Model struct {
	ModelID       string      `json:"model_id"`
	Name          string      `json:"name"`
	LlmProviderID string      `json:"llm_provider_id"`
	Type          ModelType   `json:"type"`
	Config        ModelConfig `json:"config"`
}

func (m *Model) Validate() error {
	if m.ModelID == "" {
		return errors.New("model ID is required")
	}
	if m.LlmProviderID == "" {
		return errors.New("llm provider ID is required")
	}
	if _, err := uuid.Parse(m.LlmProviderID); err != nil {
		return errors.New("llm provider ID must be a valid UUID")
	}
	if m.Type != ModelTypeChat && m.Type != ModelTypeEmbedding {
		return errors.New("invalid model type")
	}
	if m.Type == ModelTypeEmbedding {
		if m.Config.Dimensions == nil || *m.Config.Dimensions <= 0 {
			return errors.New("dimensions must be greater than 0 for embedding models")
		}
	}
	for _, c := range m.Config.Compatibilities {
		if _, ok := validCompatibilities[c]; !ok {
			return errors.New("invalid compatibility: " + c)
		}
	}
	for _, effort := range m.Config.ReasoningEfforts {
		if _, ok := validReasoningEfforts[effort]; !ok {
			return errors.New("invalid reasoning effort: " + effort)
		}
	}
	return nil
}

// HasCompatibility checks whether the model config includes the given capability.
func (m *Model) HasCompatibility(c string) bool {
	for _, v := range m.Config.Compatibilities {
		if v == c {
			return true
		}
	}
	return false
}

type AddRequest Model

type AddResponse struct {
	ID      string `json:"id"`
	ModelID string `json:"model_id"`
}

type GetRequest struct {
	ID string `json:"id"`
}

type GetResponse struct {
	ID      string `json:"id"`
	ModelID string `json:"model_id"`
	Model
}

type UpdateRequest Model

type ListRequest struct {
	Type ModelType `json:"type,omitempty"`
}

type DeleteRequest struct {
	ID      string `json:"id,omitempty"`
	ModelID string `json:"model_id,omitempty"`
}

type DeleteResponse struct {
	Message string `json:"message"`
}

type CountResponse struct {
	Count int64 `json:"count"`
}

// TestStatus represents the outcome of probing a model.
type TestStatus string

const (
	TestStatusOK                TestStatus = "ok"
	TestStatusAuthError         TestStatus = "auth_error"
	TestStatusModelNotSupported TestStatus = "model_not_supported"
	TestStatusError             TestStatus = "error"
)

// TestResponse is returned by POST /models/:id/test.
type TestResponse struct {
	Status    TestStatus `json:"status"`
	Reachable bool       `json:"reachable"`
	LatencyMs int64      `json:"latency_ms,omitempty"`
	Message   string     `json:"message,omitempty"`
}
