package registry

// ProviderDefinition describes a built-in provider loaded from a YAML file.
type ProviderDefinition struct {
	Name       string            `yaml:"name"`
	ClientType string            `yaml:"client_type"`
	Icon       string            `yaml:"icon,omitempty"`
	BaseURL    string            `yaml:"base_url"`
	Models     []ModelDefinition `yaml:"models"`
}

// ModelDefinition describes a model within a provider definition.
type ModelDefinition struct {
	ModelID string      `yaml:"model_id"`
	Name    string      `yaml:"name"`
	Type    string      `yaml:"type"`
	Config  ModelConfig `yaml:"config"`
}

// ModelConfig mirrors the JSONB config stored per model.
type ModelConfig struct {
	Dimensions      *int     `yaml:"dimensions,omitempty"      json:"dimensions,omitempty"`
	Compatibilities []string `yaml:"compatibilities,omitempty" json:"compatibilities,omitempty"`
	ContextWindow   *int     `yaml:"context_window,omitempty"  json:"context_window,omitempty"`
}
