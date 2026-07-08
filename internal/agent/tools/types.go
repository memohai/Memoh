package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/agent/sessionmode"
	"github.com/memohai/memoh/internal/agent/tools/internal/toolset"
)

// SkillDetail holds the description and content of a loadable skill.
type SkillDetail struct {
	Description string
	Content     string
	Path        string
}

// StreamEventType identifies the kind of stream event emitted by tools.
type StreamEventType string

const (
	StreamEventAttachment     StreamEventType = "attachment"
	StreamEventReaction       StreamEventType = "reaction"
	StreamEventSpeech         StreamEventType = "speech"
	StreamEventSpawnHeartbeat StreamEventType = "spawn_heartbeat"
)

// ToolStreamEvent is a side-effect event emitted by a tool targeting the
// current conversation (e.g. inline attachment, reaction, or TTS speech).
// The agent framework converts these into the appropriate wire-level events.
type ToolStreamEvent struct {
	Type StreamEventType
	// ToolCallID identifies the tool call that produced this side effect, so
	// downstream persistence can anchor it to the right message (and keep the
	// live ordering after a history reload). Empty when unknown.
	ToolCallID  string
	Attachments []Attachment
	Reactions   []Reaction
	Speeches    []Speech
}

// Attachment describes a file reference emitted by a tool.
type Attachment struct {
	Type        string         `json:"type"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
	Base64      string         `json:"base64,omitempty"`
	PlatformKey string         `json:"platform_key,omitempty"`
	Mime        string         `json:"mime,omitempty"`
	Name        string         `json:"name,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// Reaction describes an emoji reaction emitted by a tool.
type Reaction struct {
	Emoji     string `json:"emoji"`
	MessageID string `json:"message_id"`
	Remove    bool   `json:"remove,omitempty"`
}

// Speech describes a TTS speech request emitted by a tool.
type Speech struct {
	Text string `json:"text"`
}

// StreamEmitter pushes a side-effect event into the current agent stream.
// Nil when no stream is available (e.g. subagent or non-streaming contexts
// where the caller collects events after generation).
type StreamEmitter func(ToolStreamEvent)

// SessionContext carries request-scoped identity for tool execution.
type SessionContext struct {
	BotID               string
	ChatID              string
	SessionID           string
	SessionType         string
	ChannelIdentityID   string
	SessionToken        string //nolint:gosec // carries session credential material at runtime
	CurrentPlatform     string
	ReplyTarget         string
	ConversationType    string
	CanRequestUserInput bool
	CanListUserInput    bool
	SupportsImageInput  bool
	IsSubagent          bool
	Skills              map[string]SkillDetail
	TimezoneLocation    *time.Location
	Emitter             StreamEmitter
	LiveStream          bool
}

// CanAskUser reports whether ask_user can be both shown to the model and
// delivered to the user in this run.
func (s SessionContext) CanAskUser() bool {
	return s.CanRequestUserInput && sessionmode.IsInteractive(s.SessionType)
}

// IsSameConversation reports whether the given platform+target pair refers to
// the conversation that the agent is currently replying to.
func (s SessionContext) IsSameConversation(platform, target string) bool {
	if strings.TrimSpace(s.ReplyTarget) == "" {
		return false
	}
	if platform == "" {
		platform = strings.TrimSpace(s.CurrentPlatform)
	}
	if target == "" {
		target = strings.TrimSpace(s.ReplyTarget)
	}
	return strings.EqualFold(platform, strings.TrimSpace(s.CurrentPlatform)) &&
		target == strings.TrimSpace(s.ReplyTarget)
}

// CanOmitMessagingTarget reports whether messaging tools can safely default to
// the current conversation. Background sessions may have no live reply target,
// so their usage guidance should ask for explicit platform/target instead.
func (s SessionContext) CanOmitMessagingTarget() bool {
	switch s.SessionType {
	case sessionmode.Heartbeat, sessionmode.Schedule:
		return false
	default:
		return strings.TrimSpace(s.CurrentPlatform) != "" &&
			strings.TrimSpace(s.ReplyTarget) != ""
	}
}

// CanUseLocalMessagingShortcut reports whether current-conversation side
// effects can be represented by the live agent stream instead of the channel
// sender. Non-interactive runs must use the real sender even when their target
// equals the current conversation.
func (s SessionContext) CanUseLocalMessagingShortcut() bool {
	if !s.LiveStream || s.Emitter == nil || !s.CanOmitMessagingTarget() {
		return false
	}
	switch s.SessionType {
	case "", sessionmode.Chat:
		return true
	default:
		return false
	}
}

// FormatTime formats a time.Time using the session timezone (falls back to UTC).
func (s SessionContext) FormatTime(t time.Time) string {
	if s.TimezoneLocation != nil {
		t = t.In(s.TimezoneLocation)
	}
	return t.Format(time.RFC3339)
}

// ToolProvider supplies a set of tools for the agent.
// Tools() is called per-request; implementations may return different
// tool sets based on session context (e.g. subagent restrictions, bot settings).
type ToolProvider interface {
	Tools(ctx context.Context, session SessionContext) ([]sdk.Tool, error)
}

// AvailableTools is the set of tool names registered for the current session.
type AvailableTools = toolset.Available

func NewAvailableTools(tools []sdk.Tool) AvailableTools {
	names := make([]ToolName, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Name)
		if builtIn, ok := lookupBuiltInToolName(name); ok {
			names = append(names, builtIn)
		}
	}
	return toolset.New(names)
}

func usageSection(title string, items []string) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			lines = append(lines, "- "+item)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "### " + title + "\n\n" + strings.Join(lines, "\n")
}

func joinRefs(refs []string, conjunction string) string {
	switch len(refs) {
	case 0:
		return ""
	case 1:
		return refs[0]
	case 2:
		return refs[0] + " " + strings.TrimSpace(conjunction) + " " + refs[1]
	default:
		return strings.Join(refs[:len(refs)-1], ", ") + ", " + strings.TrimSpace(conjunction) + " " + refs[len(refs)-1]
	}
}

// ToolUsage is an optional capability a ToolProvider may also implement to
// contribute group-level usage guidance to the system prompt — how this set of
// tools is meant to be used together (e.g. "look up a target with get_contacts
// before messaging another conversation"). The agent injects the returned text
// only when the same provider actually returns tools for the session, so the
// guidance shares that provider's gating and stays in lockstep with the tools
// that provider registers. available contains the complete registered tool set
// for this session; use available.Ref/Refs before naming cross-provider tools.
// Return "" to contribute nothing.
type ToolUsage interface {
	Usage(ctx context.Context, session SessionContext, available AvailableTools) string
}

// ---- argument parsing helpers ----

func StringArg(arguments map[string]any, key string) string {
	if arguments == nil {
		return ""
	}
	raw, ok := arguments[key]
	if !ok {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", raw))
	}
}

func FirstStringArg(arguments map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := StringArg(arguments, key); value != "" {
			return value
		}
	}
	return ""
}

func IntArg(arguments map[string]any, key string) (int, bool, error) {
	if arguments == nil {
		return 0, false, nil
	}
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	switch value := raw.(type) {
	case int:
		return value, true, nil
	case int64:
		if value < int64(math.MinInt) || value > int64(math.MaxInt) {
			return 0, true, fmt.Errorf("%s out of range", key)
		}
		return int(value), true, nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, true, fmt.Errorf("%s must be a valid number", key)
		}
		if value < float64(math.MinInt) || value > float64(math.MaxInt) {
			return 0, true, fmt.Errorf("%s out of range", key)
		}
		return int(value), true, nil
	case json.Number:
		i, err := value.Int64()
		if err != nil {
			return 0, true, fmt.Errorf("%s must be an integer", key)
		}
		if i < int64(math.MinInt) || i > int64(math.MaxInt) {
			return 0, true, fmt.Errorf("%s out of range", key)
		}
		return int(i), true, nil
	default:
		return 0, true, fmt.Errorf("%s must be a number", key)
	}
}

func BoolArg(arguments map[string]any, key string) (bool, bool, error) {
	if arguments == nil {
		return false, false, nil
	}
	raw, ok := arguments[key]
	if !ok || raw == nil {
		return false, false, nil
	}
	value, ok := raw.(bool)
	if !ok {
		return false, true, fmt.Errorf("%s must be a boolean", key)
	}
	return value, true, nil
}

func inputAsMap(input any) map[string]any {
	args, ok := input.(map[string]any)
	if ok {
		return args
	}
	if input == nil {
		return map[string]any{}
	}
	raw, _ := json.Marshal(input)
	_ = json.Unmarshal(raw, &args)
	if args == nil {
		args = map[string]any{}
	}
	return args
}
