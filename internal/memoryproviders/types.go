package memoryproviders

import "time"

type ProviderType string

const (
	ProviderBuiltin ProviderType = "builtin"
)

type CreateRequest struct {
	Name     string         `json:"name"`
	Provider ProviderType   `json:"provider"`
	Config   map[string]any `json:"config,omitempty"`
}

type UpdateRequest struct {
	Name   *string        `json:"name,omitempty"`
	Config map[string]any `json:"config,omitempty"`
}

type GetResponse struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	Config    map[string]any `json:"config,omitempty"`
	IsDefault bool           `json:"is_default"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ProviderConfigSchema struct {
	Fields map[string]ProviderFieldSchema `json:"fields"`
}

type ProviderFieldSchema struct {
	Type        string `json:"type"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Example     any    `json:"example,omitempty"`
}

type ProviderMeta struct {
	Provider     string               `json:"provider"`
	DisplayName  string               `json:"display_name"`
	ConfigSchema ProviderConfigSchema `json:"config_schema"`
}
