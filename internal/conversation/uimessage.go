package conversation

import (
	"strings"
	"time"
)

// UIMessageType identifies the frontend-friendly message block type.
type UIMessageType string

const (
	UIMessageText        UIMessageType = "text"
	UIMessageReasoning   UIMessageType = "reasoning"
	UIMessageTool        UIMessageType = "tool"
	UIMessageAttachments UIMessageType = "attachments"
)

// UIAttachment is the normalized attachment shape used by the web frontend.
type UIAttachment struct {
	ID          string         `json:"id,omitempty"`
	Type        string         `json:"type"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
	Base64      string         `json:"base64,omitempty"`
	Name        string         `json:"name,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	BotID       string         `json:"bot_id,omitempty"`
	Mime        string         `json:"mime,omitempty"`
	Size        int64          `json:"size,omitempty"`
	StorageKey  string         `json:"storage_key,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// UIMessage is the normalized assistant output block used by the web frontend.
type UIMessage struct {
	ID          int            `json:"id"`
	Type        UIMessageType  `json:"type"`
	Content     string         `json:"content,omitempty"`
	Name        string         `json:"name,omitempty"`
	Input       any            `json:"input,omitempty"`
	Output      any            `json:"output,omitempty"`
	ToolCallID  string         `json:"tool_call_id,omitempty"`
	Running     *bool          `json:"running,omitempty"`
	Progress    []any          `json:"progress,omitempty"`
	Attachments []UIAttachment `json:"attachments,omitempty"`
}

// UITurn is the normalized chat turn used by the web frontend.
type UITurn struct {
	Role              string         `json:"role"`
	Messages          []UIMessage    `json:"messages,omitempty"`
	Text              string         `json:"text,omitempty"`
	Attachments       []UIAttachment `json:"attachments,omitempty"`
	Timestamp         time.Time      `json:"timestamp"`
	Platform          string         `json:"platform,omitempty"`
	SenderDisplayName string         `json:"sender_display_name,omitempty"`
	SenderAvatarURL   string         `json:"sender_avatar_url,omitempty"`
	SenderUserID      string         `json:"sender_user_id,omitempty"`
	ID                string         `json:"id,omitempty"`
}

// UIMessageStreamEvent is the generic event shape accepted by the UI stream converter.
// The handler layer adapts agent/channel events to this struct to avoid package cycles.
type UIMessageStreamEvent struct {
	Type        string
	Delta       string
	ToolName    string
	ToolCallID  string
	Input       any
	Output      any
	Progress    any
	Attachments []UIAttachment
	Error       string
}

func uiBoolPtr(v bool) *bool {
	return &v
}

func normalizeUIAttachmentType(kind, mime string) string {
	if trimmed := strings.ToLower(strings.TrimSpace(kind)); trimmed != "" {
		return trimmed
	}

	normalizedMime := strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(normalizedMime, "image/"):
		return "image"
	case strings.HasPrefix(normalizedMime, "audio/"):
		return "audio"
	case strings.HasPrefix(normalizedMime, "video/"):
		return "video"
	default:
		return "file"
	}
}
