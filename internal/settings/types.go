package settings

const (
	DefaultMaxContextLoadTime = 24 * 60
	DefaultLanguage           = "auto"
)

type Settings struct {
	ChatModelID        string `json:"chat_model_id"`
	MemoryModelID      string `json:"memory_model_id"`
	EmbeddingModelID   string `json:"embedding_model_id"`
	MaxContextLoadTime int    `json:"max_context_load_time"`
	Language           string `json:"language"`
	AllowGuest         bool   `json:"allow_guest"`
}

type UpsertRequest struct {
	ChatModelID        string `json:"chat_model_id,omitempty"`
	MemoryModelID      string `json:"memory_model_id,omitempty"`
	EmbeddingModelID   string `json:"embedding_model_id,omitempty"`
	MaxContextLoadTime *int   `json:"max_context_load_time,omitempty"`
	Language           string `json:"language,omitempty"`
	AllowGuest         *bool  `json:"allow_guest,omitempty"`
}
