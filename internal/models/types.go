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
	ClientTypeOpenAIResponses    ClientType = "openai-responses"
	ClientTypeOpenAICompletions  ClientType = "openai-completions"
	ClientTypeAnthropicMessages  ClientType = "anthropic-messages"
	ClientTypeGoogleGenerativeAI ClientType = "google-generative-ai"
)

type Model struct {
	ModelID            string     `json:"model_id"`
	Name               string     `json:"name"`
	LlmProviderID      string     `json:"llm_provider_id"`
	ClientType         ClientType `json:"client_type,omitempty"`
	InputModalities    []string   `json:"input_modalities,omitempty"`
	SupportsReasoning  bool       `json:"supports_reasoning"`
	Type               ModelType  `json:"type"`
	Dimensions         int        `json:"dimensions"`
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
	if m.Type == ModelTypeChat {
		if m.ClientType == "" {
			return errors.New("client_type is required for chat models")
		}
		if !isValidClientType(m.ClientType) {
			return fmt.Errorf("invalid client_type: %s", m.ClientType)
		}
	}
	if m.Type == ModelTypeEmbedding && m.Dimensions <= 0 {
		return errors.New("dimensions must be greater than 0")
	}
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
	ID      string `json:"id"`
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
