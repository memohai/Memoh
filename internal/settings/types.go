package settings

// Default values for bot settings when not set.
const (
	DefaultMaxContextLoadTime = 24 * 60
	DefaultLanguage           = "auto"
)

// Settings holds bot-level settings (models, max context minutes, language, guest access).
type Settings struct {
	ChatModelID        string `json:"chat_model_id"`
	MemoryModelID      string `json:"memory_model_id"`
	EmbeddingModelID   string `json:"embedding_model_id"`
	SearchProviderID   string `json:"search_provider_id"`
	MaxContextLoadTime int    `json:"max_context_load_time"`
	Language           string `json:"language"`
	AllowGuest         bool   `json:"allow_guest"`
}

// UpsertRequest is the input for upserting bot settings (all fields optional).
type UpsertRequest struct {
	ChatModelID        string `json:"chat_model_id,omitempty"`
	MemoryModelID      string `json:"memory_model_id,omitempty"`
	EmbeddingModelID   string `json:"embedding_model_id,omitempty"`
	SearchProviderID   string `json:"search_provider_id,omitempty"`
	MaxContextLoadTime *int   `json:"max_context_load_time,omitempty"`
	Language           string `json:"language,omitempty"`
	AllowGuest         *bool  `json:"allow_guest,omitempty"`
}
