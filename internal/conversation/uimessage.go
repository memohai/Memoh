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

type UIReplyRef struct {
	MessageID   string         `json:"message_id,omitempty"`
	Sender      string         `json:"sender,omitempty"`
	Preview     string         `json:"preview,omitempty"`
	Attachments []UIAttachment `json:"attachments,omitempty"`
}

type UIForwardRef struct {
	MessageID          string `json:"message_id,omitempty"`
	FromUserID         string `json:"from_user_id,omitempty"`
	FromConversationID string `json:"from_conversation_id,omitempty"`
	Sender             string `json:"sender,omitempty"`
	Date               int64  `json:"date,omitempty"`
}

// UIMessage is the normalized assistant output block used by the web frontend.
type UIMessage struct {
	ID          int               `json:"id"`
	Type        UIMessageType     `json:"type"`
	Content     string            `json:"content,omitempty"`
	Name        string            `json:"name,omitempty"`
	Input       any               `json:"input,omitempty"`
	Output      any               `json:"output,omitempty"`
	ToolCallID  string            `json:"tool_call_id,omitempty"`
	Running     *bool             `json:"running,omitempty"`
	Progress    []any             `json:"progress,omitempty"`
	Approval    *UIToolApproval   `json:"approval,omitempty"`
	Attachments []UIAttachment    `json:"attachments,omitempty"`
	Background  *UIBackgroundTask `json:"background_task,omitempty"`
}

type UIToolApproval struct {
	ApprovalID     string `json:"approval_id"`
	ShortID        int    `json:"short_id,omitempty"`
	Status         string `json:"status"`
	DecisionReason string `json:"decision_reason,omitempty"`
	CanApprove     bool   `json:"can_approve,omitempty"`
}

// UITurn is the normalized chat turn used by the web frontend.
type UITurn struct {
	Role              string            `json:"role"`
	Kind              string            `json:"kind,omitempty"`
	Messages          []UIMessage       `json:"messages,omitempty"`
	Text              string            `json:"text,omitempty"`
	Attachments       []UIAttachment    `json:"attachments,omitempty"`
	Reply             *UIReplyRef       `json:"reply,omitempty"`
	Forward           *UIForwardRef     `json:"forward,omitempty"`
	BackgroundTask    *UIBackgroundTask `json:"background_task,omitempty"`
	Timestamp         time.Time         `json:"timestamp"`
	Platform          string            `json:"platform,omitempty"`
	SenderDisplayName string            `json:"sender_display_name,omitempty"`
	SenderAvatarURL   string            `json:"sender_avatar_url,omitempty"`
	SenderUserID      string            `json:"sender_user_id,omitempty"`
	ExternalMessageID string            `json:"external_message_id,omitempty"`
	ID                string            `json:"id,omitempty"`
}

// UIBackgroundTask is the compact background exec state sent to the Web UI.
type UIBackgroundTask struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	Command    string `json:"command,omitempty"`
	OutputFile string `json:"output_file,omitempty"`
	ExitCode   int32  `json:"exit_code,omitempty"`
	Duration   string `json:"duration,omitempty"`
	OutputTail string `json:"output_tail,omitempty"`
	Stream     string `json:"stream,omitempty"`
	Chunk      string `json:"chunk,omitempty"`
	Stalled    bool   `json:"stalled,omitempty"`
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
	ApprovalID  string
	ShortID     int
	Status      string
	Metadata    map[string]any
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
