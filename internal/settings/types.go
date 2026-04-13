package settings

const (
	DefaultLanguage          = "auto"
	DefaultReasoningEffort   = "medium"
	DefaultHeartbeatInterval = 30
)

type Settings struct {
	ChatModelID            string `json:"chat_model_id"`
	ImageModelID           string `json:"image_model_id"`
	SearchProviderID       string `json:"search_provider_id"`
	MemoryProviderID       string `json:"memory_provider_id"`
	TtsModelID             string `json:"tts_model_id"`
	BrowserContextID       string `json:"browser_context_id"`
	Language               string `json:"language"`
	AclDefaultEffect       string `json:"acl_default_effect"`
	Timezone               string `json:"timezone"`
	ReasoningEnabled       bool   `json:"reasoning_enabled"`
	ReasoningEffort        string `json:"reasoning_effort"`
	HeartbeatEnabled       bool   `json:"heartbeat_enabled"`
	HeartbeatInterval      int    `json:"heartbeat_interval"`
	HeartbeatModelID       string `json:"heartbeat_model_id"`
	TitleModelID           string `json:"title_model_id"`
	CompactionEnabled      bool   `json:"compaction_enabled"`
	CompactionThreshold    int    `json:"compaction_threshold"`
	CompactionRatio        int    `json:"compaction_ratio"`
	CompactionModelID      string `json:"compaction_model_id,omitempty"`
	DiscussProbeModelID    string `json:"discuss_probe_model_id,omitempty"`
	ContextTokenBudget     int    `json:"context_token_budget"`
	PersistFullToolResults bool   `json:"persist_full_tool_results"`
}

type UpsertRequest struct {
	ChatModelID            string  `json:"chat_model_id,omitempty"`
	ImageModelID           string  `json:"image_model_id,omitempty"`
	SearchProviderID       string  `json:"search_provider_id,omitempty"`
	MemoryProviderID       string  `json:"memory_provider_id,omitempty"`
	TtsModelID             string  `json:"tts_model_id,omitempty"`
	BrowserContextID       string  `json:"browser_context_id,omitempty"`
	Language               string  `json:"language,omitempty"`
	AclDefaultEffect       string  `json:"acl_default_effect,omitempty"`
	Timezone               *string `json:"timezone,omitempty"`
	ReasoningEnabled       *bool   `json:"reasoning_enabled,omitempty"`
	ReasoningEffort        *string `json:"reasoning_effort,omitempty"`
	HeartbeatEnabled       *bool   `json:"heartbeat_enabled,omitempty"`
	HeartbeatInterval      *int    `json:"heartbeat_interval,omitempty"`
	HeartbeatModelID       string  `json:"heartbeat_model_id,omitempty"`
	TitleModelID           string  `json:"title_model_id,omitempty"`
	CompactionEnabled      *bool   `json:"compaction_enabled,omitempty"`
	CompactionThreshold    *int    `json:"compaction_threshold,omitempty"`
	CompactionRatio        *int    `json:"compaction_ratio,omitempty"`
	CompactionModelID      *string `json:"compaction_model_id,omitempty"`
	DiscussProbeModelID    string  `json:"discuss_probe_model_id,omitempty"`
	ContextTokenBudget     *int    `json:"context_token_budget,omitempty"`
	PersistFullToolResults *bool   `json:"persist_full_tool_results,omitempty"`
}
