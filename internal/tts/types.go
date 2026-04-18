package tts

import "time"

// ProviderMetaResponse exposes adapter metadata (from the registry, not DB).
type ProviderMetaResponse struct {
	Provider     string       `json:"provider"`
	DisplayName  string       `json:"display_name"`
	Description  string       `json:"description"`
	ConfigSchema ConfigSchema `json:"config_schema,omitempty"`
	DefaultModel string       `json:"default_model"`
	Models       []ModelInfo  `json:"models"`
}

// SpeechProviderResponse represents a speech-capable provider from the unified providers table.
type SpeechProviderResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	ClientType string    `json:"client_type"`
	Enable     bool      `json:"enable"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SpeechModelResponse represents a speech model from the unified models table.
type SpeechModelResponse struct {
	ID           string         `json:"id"`
	ModelID      string         `json:"model_id"`
	Name         string         `json:"name"`
	ProviderID   string         `json:"provider_id"`
	ProviderType string         `json:"provider_type,omitempty"`
	Config       map[string]any `json:"config,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// UpdateSpeechProviderRequest is used for updating a speech provider.
type UpdateSpeechProviderRequest struct {
	Name   *string `json:"name,omitempty"`
	Enable *bool   `json:"enable,omitempty"`
}

// UpdateSpeechModelRequest is used for updating a speech model.
type UpdateSpeechModelRequest struct {
	Name   *string        `json:"name,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

// TestSynthesizeRequest represents a text-to-speech test request.
type TestSynthesizeRequest struct {
	Text   string         `json:"text"`
	Config map[string]any `json:"config,omitempty"`
}
