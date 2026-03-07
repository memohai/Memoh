package settings

const (
	DefaultMaxContextLoadTime = 24 * 60
	DefaultMaxInboxItems      = 50
	DefaultLanguage           = "auto"
	DefaultReasoningEffort    = "medium"
	DefaultHeartbeatInterval  = 30
)

type Settings struct {
	ChatModelID        string `json:"chat_model_id"`
	SearchProviderID   string `json:"search_provider_id"`
	MemoryProviderID   string `json:"memory_provider_id"`
	TtsModelID         string `json:"tts_model_id"`
	BrowserContextID   string `json:"browser_context_id"`
	MaxContextLoadTime int    `json:"max_context_load_time"`
	MaxContextTokens   int    `json:"max_context_tokens"`
	MaxInboxItems      int    `json:"max_inbox_items"`
	Language           string `json:"language"`
	AllowGuest         bool   `json:"allow_guest"`
	ReasoningEnabled   bool   `json:"reasoning_enabled"`
	ReasoningEffort    string `json:"reasoning_effort"`
	HeartbeatEnabled   bool   `json:"heartbeat_enabled"`
	HeartbeatInterval  int    `json:"heartbeat_interval"`
	HeartbeatModelID   string `json:"heartbeat_model_id"`
}

type UpsertRequest struct {
	ChatModelID        string  `json:"chat_model_id,omitempty"`
	SearchProviderID   string  `json:"search_provider_id,omitempty"`
	MemoryProviderID   string  `json:"memory_provider_id,omitempty"`
	TtsModelID         string  `json:"tts_model_id,omitempty"`
	BrowserContextID   string  `json:"browser_context_id,omitempty"`
	MaxContextLoadTime *int    `json:"max_context_load_time,omitempty"`
	MaxContextTokens   *int    `json:"max_context_tokens,omitempty"`
	MaxInboxItems      *int    `json:"max_inbox_items,omitempty"`
	Language           string  `json:"language,omitempty"`
	AllowGuest         *bool   `json:"allow_guest,omitempty"`
	ReasoningEnabled   *bool   `json:"reasoning_enabled,omitempty"`
	ReasoningEffort    *string `json:"reasoning_effort,omitempty"`
	HeartbeatEnabled   *bool   `json:"heartbeat_enabled,omitempty"`
	HeartbeatInterval  *int    `json:"heartbeat_interval,omitempty"`
	HeartbeatModelID   string  `json:"heartbeat_model_id,omitempty"`
}
