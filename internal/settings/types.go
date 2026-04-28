package settings

const (
	DefaultLanguage          = "auto"
	DefaultReasoningEffort   = "medium"
	DefaultHeartbeatInterval = 30
)

type Settings struct {
	ChatModelID            string             `json:"chat_model_id"`
	ImageModelID           string             `json:"image_model_id"`
	SearchProviderID       string             `json:"search_provider_id"`
	MemoryProviderID       string             `json:"memory_provider_id"`
	TtsModelID             string             `json:"tts_model_id"`
	TranscriptionModelID   string             `json:"transcription_model_id"`
	BrowserContextID       string             `json:"browser_context_id"`
	Language               string             `json:"language"`
	AclDefaultEffect       string             `json:"acl_default_effect"`
	Timezone               string             `json:"timezone"`
	ReasoningEnabled       bool               `json:"reasoning_enabled"`
	ReasoningEffort        string             `json:"reasoning_effort"`
	HeartbeatEnabled       bool               `json:"heartbeat_enabled"`
	HeartbeatInterval      int                `json:"heartbeat_interval"`
	HeartbeatModelID       string             `json:"heartbeat_model_id"`
	TitleModelID           string             `json:"title_model_id"`
	CompactionEnabled      bool               `json:"compaction_enabled"`
	CompactionThreshold    int                `json:"compaction_threshold"`
	CompactionRatio        int                `json:"compaction_ratio"`
	CompactionModelID      string             `json:"compaction_model_id,omitempty"`
	DiscussProbeModelID    string             `json:"discuss_probe_model_id,omitempty"`
	PersistFullToolResults bool               `json:"persist_full_tool_results"`
	ShowToolCallsInIM      bool               `json:"show_tool_calls_in_im"`
	ToolApprovalConfig     ToolApprovalConfig `json:"tool_approval_config"`
}

type UpsertRequest struct {
	ChatModelID            string              `json:"chat_model_id,omitempty"`
	ImageModelID           string              `json:"image_model_id,omitempty"`
	SearchProviderID       string              `json:"search_provider_id,omitempty"`
	MemoryProviderID       string              `json:"memory_provider_id,omitempty"`
	TtsModelID             string              `json:"tts_model_id,omitempty"`
	TranscriptionModelID   string              `json:"transcription_model_id,omitempty"`
	BrowserContextID       string              `json:"browser_context_id,omitempty"`
	Language               string              `json:"language,omitempty"`
	AclDefaultEffect       string              `json:"acl_default_effect,omitempty"`
	Timezone               *string             `json:"timezone,omitempty"`
	ReasoningEnabled       *bool               `json:"reasoning_enabled,omitempty"`
	ReasoningEffort        *string             `json:"reasoning_effort,omitempty"`
	HeartbeatEnabled       *bool               `json:"heartbeat_enabled,omitempty"`
	HeartbeatInterval      *int                `json:"heartbeat_interval,omitempty"`
	HeartbeatModelID       string              `json:"heartbeat_model_id,omitempty"`
	TitleModelID           string              `json:"title_model_id,omitempty"`
	CompactionEnabled      *bool               `json:"compaction_enabled,omitempty"`
	CompactionThreshold    *int                `json:"compaction_threshold,omitempty"`
	CompactionRatio        *int                `json:"compaction_ratio,omitempty"`
	CompactionModelID      *string             `json:"compaction_model_id,omitempty"`
	DiscussProbeModelID    string              `json:"discuss_probe_model_id,omitempty"`
	PersistFullToolResults *bool               `json:"persist_full_tool_results,omitempty"`
	ShowToolCallsInIM      *bool               `json:"show_tool_calls_in_im,omitempty"`
	ToolApprovalConfig     *ToolApprovalConfig `json:"tool_approval_config,omitempty"`
}

type ToolApprovalConfig struct {
	Enabled bool                   `json:"enabled"`
	Write   ToolApprovalFilePolicy `json:"write"`
	Edit    ToolApprovalFilePolicy `json:"edit"`
	Exec    ToolApprovalExecPolicy `json:"exec"`
}

type ToolApprovalFilePolicy struct {
	RequireApproval  bool     `json:"require_approval"`
	BypassGlobs      []string `json:"bypass_globs"`
	ForceReviewGlobs []string `json:"force_review_globs"`
}

type ToolApprovalExecPolicy struct {
	RequireApproval     bool     `json:"require_approval"`
	BypassCommands      []string `json:"bypass_commands"`
	ForceReviewCommands []string `json:"force_review_commands"`
}

func DefaultToolApprovalConfig() ToolApprovalConfig {
	fileBypass := []string{"/data/**", "/tmp/**"}
	return ToolApprovalConfig{
		Enabled: false,
		Write: ToolApprovalFilePolicy{
			RequireApproval:  true,
			BypassGlobs:      append([]string(nil), fileBypass...),
			ForceReviewGlobs: []string{},
		},
		Edit: ToolApprovalFilePolicy{
			RequireApproval:  true,
			BypassGlobs:      append([]string(nil), fileBypass...),
			ForceReviewGlobs: []string{},
		},
		Exec: ToolApprovalExecPolicy{
			RequireApproval:     false,
			BypassCommands:      []string{},
			ForceReviewCommands: []string{},
		},
	}
}

func NormalizeToolApprovalConfig(cfg ToolApprovalConfig) ToolApprovalConfig {
	defaults := DefaultToolApprovalConfig()
	defaults.Enabled = cfg.Enabled
	defaults.Write = normalizeFilePolicy(cfg.Write, defaults.Write)
	defaults.Edit = normalizeFilePolicy(cfg.Edit, defaults.Edit)
	defaults.Exec = normalizeExecPolicy(cfg.Exec, defaults.Exec)
	return defaults
}

func normalizeFilePolicy(policy, defaults ToolApprovalFilePolicy) ToolApprovalFilePolicy {
	if policy.RequireApproval {
		defaults.RequireApproval = true
	} else {
		defaults.RequireApproval = false
	}
	if policy.BypassGlobs != nil {
		defaults.BypassGlobs = append([]string(nil), policy.BypassGlobs...)
	}
	if policy.ForceReviewGlobs != nil {
		defaults.ForceReviewGlobs = append([]string(nil), policy.ForceReviewGlobs...)
	}
	return defaults
}

func normalizeExecPolicy(policy, defaults ToolApprovalExecPolicy) ToolApprovalExecPolicy {
	if policy.RequireApproval {
		defaults.RequireApproval = true
	} else {
		defaults.RequireApproval = false
	}
	if policy.BypassCommands != nil {
		defaults.BypassCommands = append([]string(nil), policy.BypassCommands...)
	}
	if policy.ForceReviewCommands != nil {
		defaults.ForceReviewCommands = append([]string(nil), policy.ForceReviewCommands...)
	}
	return defaults
}
