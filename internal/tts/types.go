package tts

import "time"

type CreateProviderRequest struct {
	Name     string         `json:"name"`
	Provider TtsType        `json:"provider"`
	Config   map[string]any `json:"config,omitempty"`
}

type UpdateProviderRequest struct {
	Name     *string        `json:"name,omitempty"`
	Provider *TtsType       `json:"provider,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

type ProviderResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ProviderMetaResponse struct {
	Provider     string       `json:"provider"`
	DisplayName  string       `json:"display_name"`
	Description  string       `json:"description"`
	Capabilities Capabilities `json:"capabilities"`
}

type TestSynthesizeRequest struct {
	Text       string         `json:"text"`
	Config     map[string]any `json:"config,omitempty"`
}
