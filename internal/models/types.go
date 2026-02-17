package models

import (
	"errors"

	"github.com/google/uuid"
)

// ModelType is the model kind: chat or embedding.
type ModelType string

// Model type constants.
const (
	ModelTypeChat      ModelType = "chat"
	ModelTypeEmbedding ModelType = "embedding"
)

// Supported model input types for multimodal models.
const (
	ModelInputText  = "text"
	ModelInputImage = "image"
)

// ClientType is the LLM provider client type (openai, anthropic, etc.).
type ClientType string

// Client type constants for LLM providers.
const (
	ClientTypeOpenAI       ClientType = "openai"
	ClientTypeOpenAICompat ClientType = "openai-compat"
	ClientTypeAnthropic    ClientType = "anthropic"
	ClientTypeGoogle       ClientType = "google"
	ClientTypeAzure        ClientType = "azure"
	ClientTypeBedrock      ClientType = "bedrock"
	ClientTypeMistral      ClientType = "mistral"
	ClientTypeXAI          ClientType = "xai"
	ClientTypeOllama       ClientType = "ollama"
	ClientTypeDashscope    ClientType = "dashscope"
)

// Model is a single model definition (id, provider, type, dimensions, input types).
type Model struct {
	ModelID       string    `json:"model_id"`
	Name          string    `json:"name"`
	LlmProviderID string    `json:"llm_provider_id"`
	IsMultimodal  bool      `json:"is_multimodal"`
	Input         []string  `json:"input"`
	Type          ModelType `json:"type"`
	Dimensions    int       `json:"dimensions"`
}

// Validate checks model ID, provider ID, type, and dimensions (for embedding).
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
	if m.Type == ModelTypeEmbedding && m.Dimensions <= 0 {
		return errors.New("dimensions must be greater than 0")
	}

	return nil
}

// AddRequest is the input for creating a model (same shape as Model).
type AddRequest Model

// AddResponse returns the created model ID.
type AddResponse struct {
	ID      string `json:"id"`
	ModelID string `json:"model_id"`
}

// GetRequest is the input for getting a model by ID.
type GetRequest struct {
	ID string `json:"id"`
}

// GetResponse is the full model with model_id (for API response).
type GetResponse struct {
	ModelID string `json:"model_id"`
	Model
}

// UpdateRequest is the input for updating a model (same shape as Model).
type UpdateRequest Model

// ListRequest optionally filters by type and client type.
type ListRequest struct {
	Type       ModelType  `json:"type,omitempty"`
	ClientType ClientType `json:"client_type,omitempty"`
}

// DeleteRequest identifies a model by ID or model_id.
type DeleteRequest struct {
	ID      string `json:"id,omitempty"`
	ModelID string `json:"model_id,omitempty"`
}

// DeleteResponse holds a message after delete.
type DeleteResponse struct {
	Message string `json:"message"`
}

// CountResponse holds the total count for list/count API.
type CountResponse struct {
	Count int64 `json:"count"`
}
