package searchproviders

import "time"

// ProviderName identifies a search provider (e.g. brave).
type ProviderName string

// Supported provider name constants.
const (
	ProviderBrave ProviderName = "brave"
)

// ProviderConfigSchema describes the config fields for a provider (for UI).
type ProviderConfigSchema struct {
	Fields map[string]ProviderFieldSchema `json:"fields"`
}

// ProviderFieldSchema describes a single config field (type, title, required, etc.).
type ProviderFieldSchema struct {
	Type        string   `json:"type"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Example     any      `json:"example,omitempty"`
}

// ProviderMeta is metadata for a provider (display name and config schema).
type ProviderMeta struct {
	Provider     string               `json:"provider"`
	DisplayName  string               `json:"display_name"`
	ConfigSchema ProviderConfigSchema `json:"config_schema"`
}

// CreateRequest is the input for creating a search provider config.
type CreateRequest struct {
	Name     string         `json:"name"`
	Provider ProviderName   `json:"provider"`
	Config   map[string]any `json:"config,omitempty"`
}

// UpdateRequest is the input for updating a search provider config (all fields optional).
type UpdateRequest struct {
	Name     *string        `json:"name,omitempty"`
	Provider *ProviderName  `json:"provider,omitempty"`
	Config   map[string]any `json:"config,omitempty"`
}

// GetResponse is the API response for a single search provider config.
type GetResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}
