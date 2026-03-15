package tts

import "time"

// --- Provider types ---

type CreateProviderRequest struct {
	Name     string  `json:"name"`
	Provider TtsType `json:"provider"`
}

type UpdateProviderRequest struct {
	Name *string `json:"name,omitempty"`
}

type ProviderResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProviderMetaResponse struct {
	Provider     string      `json:"provider"`
	DisplayName  string      `json:"display_name"`
	Description  string      `json:"description"`
	DefaultModel string      `json:"default_model"`
	Models       []ModelInfo `json:"models"`
}

// --- Model types ---

type ModelResponse struct {
	ID            string         `json:"id"`
	ModelID       string         `json:"model_id"`
	Name          string         `json:"name"`
	TtsProviderID string         `json:"tts_provider_id"`
	ProviderType  string         `json:"provider_type,omitempty"`
	Config        map[string]any `json:"config,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type CreateModelRequest struct {
	ModelID       string         `json:"model_id"`
	Name          string         `json:"name"`
	TtsProviderID string         `json:"tts_provider_id"`
	Config        map[string]any `json:"config,omitempty"`
}

type UpdateModelRequest struct {
	Name   *string        `json:"name,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

// --- Synthesis types ---

type TestSynthesizeRequest struct {
	Text   string         `json:"text"`
	Config map[string]any `json:"config,omitempty"`
}
