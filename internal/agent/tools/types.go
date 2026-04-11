package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"
)

// SkillDetail holds the description and content of a loadable skill.
type SkillDetail struct {
	Description string
	Content     string
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
	Type        StreamEventType
	Attachments []Attachment
	Reactions   []Reaction
	Speeches    []Speech
}

// Attachment describes a file reference emitted by a tool.
type Attachment struct {
	Type        string         `json:"type"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
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
	BotID              string
	ChatID             string
	SessionID          string
	SessionType        string
	ChannelIdentityID  string
	SessionToken       string //nolint:gosec // carries session credential material at runtime
	CurrentPlatform    string
	ReplyTarget        string
	SupportsImageInput bool
	IsSubagent         bool
	Skills             map[string]SkillDetail
	TimezoneLocation   *time.Location
	Emitter            StreamEmitter
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
