package providertemplates

import "time"

type Domain string

const (
	DomainLLM           Domain = "llm"
	DomainSpeech        Domain = "speech"
	DomainTranscription Domain = "transcription"
	DomainVideo         Domain = "video"
)

func IsValidDomain(domain Domain) bool {
	switch domain {
	case DomainLLM, DomainSpeech, DomainTranscription, DomainVideo:
		return true
	default:
		return false
	}
}

type Definition struct {
	Key           string
	Domain        Domain
	Name          string
	Description   string
	Icon          string
	Driver        string
	ConfigSchema  map[string]any
	DefaultConfig map[string]any
	Metadata      map[string]any
	Source        string
	SortOrder     int
	Models        []ModelDefinition
}

type ModelDefinition struct {
	ModelID   string
	Name      string
	Type      string
	Config    map[string]any
	Metadata  map[string]any
	SortOrder int
}

type ModelResponse struct {
	ID        string         `json:"id"`
	ModelID   string         `json:"model_id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	SortOrder int            `json:"sort_order"`
}

type GetResponse struct {
	ID            string          `json:"id"`
	Key           string          `json:"key"`
	Domain        string          `json:"domain"`
	Name          string          `json:"name"`
	Description   string          `json:"description,omitempty"`
	Icon          string          `json:"icon,omitempty"`
	Driver        string          `json:"driver"`
	ConfigSchema  map[string]any  `json:"config_schema,omitempty"`
	DefaultConfig map[string]any  `json:"default_config,omitempty"`
	Metadata      map[string]any  `json:"metadata,omitempty"`
	Source        string          `json:"source,omitempty"`
	SortOrder     int             `json:"sort_order"`
	Configured    bool            `json:"configured"`
	Models        []ModelResponse `json:"models,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type CreateInstanceRequest struct {
	TemplateID string         `json:"template_id" validate:"required"`
	Name       string         `json:"name,omitempty"`
	Config     map[string]any `json:"config,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}
