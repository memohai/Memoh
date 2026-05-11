package registry

// ProviderDefinition describes a built-in provider loaded from a YAML file.
type ProviderDefinition struct {
	Name       string            `yaml:"name"`
	ClientType string            `yaml:"client_type"`
	Icon       string            `yaml:"icon,omitempty"`
	BaseURL    string            `yaml:"base_url,omitempty"`
	Config     map[string]any    `yaml:"config,omitempty"`
	Models     []ModelDefinition `yaml:"models"`
	Source     string            `yaml:"-"`
}

// ModelDefinition describes a model within a provider definition.
type ModelDefinition struct {
	ModelID string         `yaml:"model_id"`
	Name    string         `yaml:"name"`
	Type    string         `yaml:"type"`
	Config  map[string]any `yaml:"config"`
}
