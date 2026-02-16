package models

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type ModelType string

const (
	ModelTypeChat      ModelType = "chat"
	ModelTypeEmbedding ModelType = "embedding"
)

const (
	ModelInputText  = "text"
	ModelInputImage = "image"
	ModelInputAudio = "audio"
	ModelInputVideo = "video"
	ModelInputFile  = "file"
)

type ClientType string

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

type Model struct {
	ModelID         string    `json:"model_id"`
	Name            string    `json:"name"`
	LlmProviderID   string    `json:"llm_provider_id"`
	InputModalities []string  `json:"input_modalities,omitempty"`
	Type            ModelType `json:"type"`
	Dimensions      int       `json:"dimensions"`
}

// validInputModalities is the set of recognised input modality tokens.
var validInputModalities = map[string]struct{}{
	ModelInputText: {}, ModelInputImage: {}, ModelInputAudio: {},
	ModelInputVideo: {}, ModelInputFile: {},
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
	if m.Type == ModelTypeEmbedding && m.Dimensions <= 0 {
		return errors.New("dimensions must be greater than 0")
	}
	// Input modalities only apply to chat models.
	if m.Type == ModelTypeChat {
		for _, mod := range m.InputModalities {
			if _, ok := validInputModalities[mod]; !ok {
				return fmt.Errorf("invalid input modality: %s", mod)
			}
		}
	}
	return nil
}

// HasInputModality checks whether the model supports a given input modality.
func (m *Model) HasInputModality(mod string) bool {
	for _, v := range m.InputModalities {
		if v == mod {
			return true
		}
	}
	return false
}

// IsMultimodal returns true if the model supports any input modality beyond text.
func (m *Model) IsMultimodal() bool {
	for _, v := range m.InputModalities {
		if v != ModelInputText {
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
	ModelID string `json:"model_id"`
	Model
}

type UpdateRequest Model

type ListRequest struct {
	Type       ModelType  `json:"type,omitempty"`
	ClientType ClientType `json:"client_type,omitempty"`
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
