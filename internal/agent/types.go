package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/background"
)

// SessionContext carries request-scoped identity and routing information.
type SessionContext struct {
	BotID             string
	ChatID            string
	SessionID         string
	ChannelIdentityID string
	CurrentPlatform   string
	ReplyTarget       string
	ConversationType  string
	Timezone          string
	TimezoneLocation  *time.Location
	SessionToken      string //nolint:gosec // carries session credential material at runtime
	IsSubagent        bool
}

// SkillEntry represents a skill loaded from the bot container.
type SkillEntry struct {
	Name        string
	Description string
	Content     string
	Path        string
	Metadata    map[string]any
}

// Schedule represents a scheduled task definition.
type Schedule struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Pattern     string `json:"pattern"`
	MaxCalls    *int   `json:"maxCalls,omitempty"`
	Command     string `json:"command"`
}

// LoopDetectionConfig controls loop detection behavior.
type LoopDetectionConfig struct {
	Enabled bool
}

// InjectMessage carries a user message to be injected into a running agent
// stream between tool rounds via the PrepareStep hook.
type InjectMessage struct {
	Text            string
	HeaderifiedText string
	// ImageParts carries inline images (data URL or public URL) to attach
	// alongside the injected text when the model supports vision input.
	ImageParts []sdk.ImagePart
}

// RunConfig holds everything needed for a single agent invocation.
type RunConfig struct {
	Model              *sdk.Model
	ReasoningEffort    string
	Messages           []sdk.Message
	Query              string
	System             string
	SessionType        string
	SupportsImageInput bool
	SupportsToolCall   bool
	InlineImages       []sdk.ImagePart
	Identity           SessionContext
	Skills             []SkillEntry
	LoopDetection      LoopDetectionConfig
	Retry              RetryConfig

	// PromptCacheTTL controls prompt caching for this run. Empty or
	// unrecognized values default to 5m. Use "1h" for the long-cache tier
	// or "off" to disable caching entirely. The TTL is honored only when
	// the resolved model's vendor implements prompt caching (currently
	// Anthropic Messages); for other vendors the value is ignored.
	PromptCacheTTL string

	// MidTaskPruneThreshold is the minimum number of messages before mid-task
	// pruning kicks in. When the accumulated message count reaches this
	// threshold, older tool-result pairs are pruned to keep the context
	// within budget. Defaults to MidTaskPruneThresholdDefault (20).
	MidTaskPruneThreshold int

	// MidTaskPruneKeepSteps is the number of recent tool-call cycles to
	// preserve when mid-task pruning is triggered. Defaults to
	// MidTaskPruneKeepStepsDefault (4).
	MidTaskPruneKeepSteps int

	// InjectCh receives user messages to inject between tool rounds.
	// When non-nil, a PrepareStep hook drains this channel and appends
	// user messages to the conversation before the next LLM call.
	InjectCh <-chan InjectMessage

	// InjectedRecorder is called each time a message is injected via
	// PrepareStep, recording the headerified text and the number of SDK
	// output messages that preceded the injection. Used by the resolver
	// to interleave injected messages at the correct position in storeRound.
	InjectedRecorder func(headerifiedText string, insertAfter int)

	// BackgroundManager provides access to the background task system.
	// When non-nil, the agent loop drains pending notifications at step
	// boundaries and injects them as user messages so the model learns
	// about completed background work.
	BackgroundManager *background.Manager

	ToolApprovalHandler func(ctx context.Context, call sdk.ToolCall) (sdk.ToolApprovalResult, error)
}

// GenerateResult holds the result of a non-streaming agent invocation.
type GenerateResult struct {
	Messages    []sdk.Message
	Text        string
	Attachments []FileAttachment
	Reactions   []ReactionItem
	Speeches    []SpeechItem
	Usage       *sdk.Usage
}

// FileAttachment represents a file reference extracted from agent output.
type FileAttachment struct {
	Type        string         `json:"type"`
	Base64      string         `json:"base64,omitempty"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
	PlatformKey string         `json:"platform_key,omitempty"`
	Mime        string         `json:"mime,omitempty"`
	Name        string         `json:"name,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ReactionItem represents an emoji reaction extracted from agent output.
type ReactionItem struct {
	Emoji string `json:"emoji"`
}

// SpeechItem represents a TTS request extracted from agent output.
type SpeechItem struct {
	Text string `json:"text"`
}

// SystemFile is a file loaded from the bot container for prompt generation.
type SystemFile struct {
	Filename string
	Content  string
}

// ModelConfig holds provider and model information resolved from DB.
type ModelConfig struct {
	ModelID         string
	ClientType      string
	APIKey          string //nolint:gosec // carries provider credential material at runtime
	CodexAccountID  string
	BaseURL         string
	HTTPClient      *http.Client
	ReasoningConfig *ReasoningConfig
}

// ReasoningConfig controls extended thinking/reasoning behavior.
type ReasoningConfig struct {
	Enabled bool
	Effort  string
}

func mustMarshal(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return data
}

// TimeNow is a hook for testing. Defaults to time.Now.
var TimeNow = time.Now
