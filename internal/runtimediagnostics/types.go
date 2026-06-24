package runtimediagnostics

import (
	"time"

	"github.com/memohai/memoh/internal/acpclient"
)

type State string

const (
	StateOK            State = "ok"
	StateWarn          State = "warn"
	StateError         State = "error"
	StateDisabled      State = "disabled"
	StateUnknown       State = "unknown"
	StateNotApplicable State = "not_applicable"
)

type ACPModelState string

const (
	ACPModelStateKnown                    ACPModelState = "known"
	ACPModelStateUnknown                  ACPModelState = "unknown"
	ACPModelStateUnsupported              ACPModelState = "unsupported"
	ACPModelStateUnknownUntilRuntimeStart ACPModelState = "unknown_until_runtime_start"
)

type ACPSessionResumeState string

const (
	ACPSessionResumeStateWarmResumable     ACPSessionResumeState = "warm_resumable"
	ACPSessionResumeStateColdStartRequired ACPSessionResumeState = "cold_start_required"
	ACPSessionResumeStateNoACPSession      ACPSessionResumeState = "no_acp_session"
	ACPSessionResumeStateDisabled          ACPSessionResumeState = "disabled"
	ACPSessionResumeStateBlocked           ACPSessionResumeState = "blocked"
)

type DiagnosticItem struct {
	State      State          `json:"state"`
	Code       string         `json:"code"`
	Label      string         `json:"label"`
	Detail     string         `json:"detail,omitempty"`
	Evidence   map[string]any `json:"evidence,omitempty"`
	NextAction string         `json:"next_action,omitempty"`
}

type Response struct {
	CheckedAt    time.Time             `json:"checked_at"`
	OverallState State                 `json:"overall_state"`
	Summary      string                `json:"summary"`
	Workspace    WorkspaceDiagnostic   `json:"workspace"`
	Container    ContainerDiagnostic   `json:"container"`
	Display      DisplayDiagnostic     `json:"display"`
	ACPAgents    []ACPAgentDiagnostic  `json:"acp_agents"`
	RecentEvents []RuntimeEventSummary `json:"recent_events"`
}

type WorkspaceDiagnostic struct {
	DiagnosticItem
	Backend         string `json:"backend,omitempty"`
	DefaultWorkDir  string `json:"default_workdir,omitempty"`
	BridgeReachable bool   `json:"bridge_reachable"`
	MCPReachable    bool   `json:"mcp_reachable"`
}

type ContainerDiagnostic struct {
	DiagnosticItem
	Exists                   bool       `json:"exists"`
	ContainerID              string     `json:"container_id,omitempty"`
	WorkspaceBackend         string     `json:"workspace_backend,omitempty"`
	RuntimeBackend           string     `json:"runtime_backend,omitempty"`
	TaskRunning              bool       `json:"task_running"`
	Status                   string     `json:"status,omitempty"`
	MetricsSupported         bool       `json:"metrics_supported"`
	MetricsUnsupportedReason string     `json:"metrics_unsupported_reason,omitempty"`
	SampledAt                *time.Time `json:"sampled_at,omitempty"`
}

type DisplayDiagnostic struct {
	DiagnosticItem
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	Running           bool   `json:"running"`
	Transport         string `json:"transport,omitempty"`
	Encoder           string `json:"encoder,omitempty"`
	EncoderAvailable  bool   `json:"encoder_available"`
	DesktopAvailable  bool   `json:"desktop_available"`
	BrowserAvailable  bool   `json:"browser_available"`
	ToolkitAvailable  bool   `json:"toolkit_available"`
	A11yAvailable     bool   `json:"a11y_available"`
	PrepareSupported  bool   `json:"prepare_supported"`
	PrepareSystem     string `json:"prepare_system,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type ACPAgentDiagnostic struct {
	DiagnosticItem
	AgentID          string                     `json:"agent_id"`
	DisplayName      string                     `json:"display_name"`
	Enabled          bool                       `json:"enabled"`
	SetupMode        string                     `json:"setup_mode"`
	WorkspaceBackend string                     `json:"workspace_backend,omitempty"`
	CLI              ACPCLIDiagnostic           `json:"cli"`
	Auth             ACPAuthDiagnostic          `json:"auth"`
	Profile          ACPProfileDiagnostic       `json:"profile"`
	Model            ACPModelDiagnostic         `json:"model"`
	SessionResume    ACPSessionResumeDiagnostic `json:"session_resume"`
	LastError        *RuntimeEventSummary       `json:"last_error,omitempty"`
}

type ACPCLIDiagnostic struct {
	ConfiguredCommand string   `json:"configured_command"`
	ConfiguredArgs    []string `json:"configured_args,omitempty"`
	EffectiveCommand  string   `json:"effective_command"`
	EffectiveArgs     []string `json:"effective_args,omitempty"`
	ResolvedPath      string   `json:"resolved_path,omitempty"`
	Source            string   `json:"source,omitempty"`
	Available         bool     `json:"available"`
	Error             string   `json:"error,omitempty"`
	Checks            []string `json:"checks,omitempty"`
}

type ACPAuthDiagnostic struct {
	Mode           string   `json:"mode"`
	APIKeyPresent  bool     `json:"api_key_present"`
	OAuthPresent   bool     `json:"oauth_present"`
	SelfManaged    bool     `json:"self_managed"`
	Source         string   `json:"source,omitempty"`
	RequiredFields []string `json:"required_fields,omitempty"`
	MissingFields  []string `json:"missing_fields,omitempty"`
	WarningCode    string   `json:"warning_code,omitempty"`
	WarningDetail  string   `json:"warning_detail,omitempty"`
}

type ACPProfileDiagnostic struct {
	Registered        bool              `json:"registered"`
	BackendSupported  bool              `json:"backend_supported"`
	SessionModePin    string            `json:"session_mode_pin,omitempty"`
	SessionConfigPins map[string]string `json:"session_config_pins,omitempty"`
}

type ACPModelDiagnostic struct {
	State          ACPModelState         `json:"state"`
	CurrentModelID string                `json:"current_model_id,omitempty"`
	DefaultModelID string                `json:"default_model_id,omitempty"`
	Available      []acpclient.ModelInfo `json:"available_models,omitempty"`
	Detail         string                `json:"detail,omitempty"`
}

type ACPSessionResumeDiagnostic struct {
	State      ACPSessionResumeState `json:"state"`
	SessionID  string                `json:"session_id,omitempty"`
	RuntimeID  string                `json:"runtime_id,omitempty"`
	ACPSession string                `json:"acp_session_id,omitempty"`
	Detail     string                `json:"detail,omitempty"`
}

type RuntimeEventSummary struct {
	ID        string         `json:"id"`
	BotID     string         `json:"bot_id"`
	Scope     string         `json:"scope"`
	AgentID   string         `json:"agent_id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	RuntimeID string         `json:"runtime_id,omitempty"`
	Phase     string         `json:"phase"`
	Severity  string         `json:"severity"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
