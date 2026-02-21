// Package conversation defines conversation domain types and rules.
package conversation

import (
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// Conversation kind constants.
const (
	KindDirect = "direct"
	KindGroup  = "group"
	KindThread = "thread"
)

// Participant role constants.
const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Conversation list access mode constants.
const (
	AccessModeParticipant             = "participant"
	AccessModeChannelIdentityObserved = "channel_identity_observed"
)

// Conversation is the first-class conversation container.
type Conversation struct {
	ID           string         `json:"id"`
	BotID        string         `json:"bot_id"`
	Kind         string         `json:"kind"`
	ParentChatID string         `json:"parent_chat_id,omitempty"`
	Title        string         `json:"title,omitempty"`
	CreatedBy    string         `json:"created_by"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

// ConversationListItem is a conversation entry with access context for list rendering.
type ConversationListItem struct {
	ID              string         `json:"id"`
	BotID           string         `json:"bot_id"`
	Kind            string         `json:"kind"`
	ParentChatID    string         `json:"parent_chat_id,omitempty"`
	Title           string         `json:"title,omitempty"`
	CreatedBy       string         `json:"created_by"`
	Metadata        map[string]any `json:"metadata,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	AccessMode      string         `json:"access_mode"`
	ParticipantRole string         `json:"participant_role,omitempty"`
	LastObservedAt  *time.Time     `json:"last_observed_at,omitempty"`
}

// ConversationReadAccess is the resolved access context for reading conversation content.
type ConversationReadAccess struct {
	AccessMode      string
	ParticipantRole string
	LastObservedAt  *time.Time
}

// Participant represents a chat member.
type Participant struct {
	ChatID   string    `json:"chat_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// Settings holds per-chat configuration.
type Settings struct {
	ChatID  string `json:"chat_id"`
	ModelID string `json:"model_id,omitempty"`
}

// CreateRequest is the input for creating a bot-scoped conversation container.
type CreateRequest struct {
	Kind         string         `json:"kind"`
	Title        string         `json:"title,omitempty"`
	ParentChatID string         `json:"parent_chat_id,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// UpdateSettingsRequest is the input for updating chat settings.
type UpdateSettingsRequest struct {
	ModelID *string `json:"model_id,omitempty"`
}

// ModelMessage is the canonical message format exchanged with the agent gateway.
// Aligned with Vercel AI SDK ModelMessage structure.
type ModelMessage struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// TextContent extracts the plain text from the message content.
// If content is a string, it returns it directly.
// If content is an array of parts, it joins all text-type parts.
func (m ModelMessage) TextContent() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []ContentPart
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		texts := make([]string, 0, len(parts))
		for _, p := range parts {
			if strings.TrimSpace(p.Text) != "" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n")
	}
	return ""
}

// ContentParts parses the content as an array of ContentPart.
// Returns nil if the content is a plain string or not parseable.
func (m ModelMessage) ContentParts() []ContentPart {
	if len(m.Content) == 0 {
		return nil
	}
	var parts []ContentPart
	if err := json.Unmarshal(m.Content, &parts); err != nil {
		return nil
	}
	return parts
}

// HasContent reports whether the message carries non-empty content or tool calls.
func (m ModelMessage) HasContent() bool {
	if strings.TrimSpace(m.TextContent()) != "" {
		return true
	}
	if len(m.ContentParts()) > 0 {
		return true
	}
	return len(m.ToolCalls) > 0
}

// NewTextContent creates a json.RawMessage from a plain string.
func NewTextContent(text string) json.RawMessage {
	data, err := json.Marshal(text)
	if err != nil {
		slog.Warn("NewTextContent: marshal failed", slog.Any("error", err))
		return nil
	}
	return data
}

// ContentPart represents one element of a multi-part message content.
type ContentPart struct {
	Type              string         `json:"type"`
	Text              string         `json:"text,omitempty"`
	URL               string         `json:"url,omitempty"`
	Styles            []string       `json:"styles,omitempty"`
	Language          string         `json:"language,omitempty"`
	ChannelIdentityID string         `json:"channel_identity_id,omitempty"`
	Emoji             string         `json:"emoji,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

// HasValue reports whether the content part carries a meaningful value.
func (p ContentPart) HasValue() bool {
	return strings.TrimSpace(p.Text) != "" ||
		strings.TrimSpace(p.URL) != "" ||
		strings.TrimSpace(p.Emoji) != ""
}

// ToolCall represents a function/tool invocation in an assistant message.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

// ToolCallFunction holds the name and serialized arguments of a tool call.
type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ChatAttachment is a media attachment carried in a chat request.
type ChatAttachment struct {
	Type        string         `json:"type"`
	Base64      string         `json:"base64,omitempty"`
	Path        string         `json:"path,omitempty"`
	URL         string         `json:"url,omitempty"`
	PlatformKey string         `json:"platform_key,omitempty"`
	ContentHash string         `json:"content_hash,omitempty"`
	Name        string         `json:"name,omitempty"`
	Mime        string         `json:"mime,omitempty"`
	Size        int64          `json:"size,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// OutboundAssetRef carries an asset reference accumulated during outbound streaming.
type OutboundAssetRef struct {
	ContentHash string
	Role        string
	Ordinal     int
	Mime        string
	SizeBytes   int64
	StorageKey  string
}

// ChatRequest is the input for Chat and StreamChat.
type ChatRequest struct {
	BotID                   string `json:"-"`
	ChatID                  string `json:"-"`
	Token                   string `json:"-"`
	UserID                  string `json:"-"`
	SourceChannelIdentityID string `json:"-"`
	ContainerID             string `json:"-"`
	DisplayName             string `json:"-"`
	RouteID                 string `json:"-"`
	ChatToken               string `json:"-"`
	ExternalMessageID       string `json:"-"`
	ConversationType        string `json:"-"`
	ConversationName        string `json:"-"`
	UserMessagePersisted    bool   `json:"-"`

	// OutboundAssetCollector returns asset refs accumulated during outbound streaming.
	// Set by the inbound channel processor; called by the resolver at persist time.
	OutboundAssetCollector func() []OutboundAssetRef `json:"-"`

	Query              string           `json:"query"`
	Model              string           `json:"model,omitempty"`
	Provider           string           `json:"provider,omitempty"`
	MaxContextLoadTime int              `json:"max_context_load_time,omitempty"`
	Channels           []string         `json:"channels,omitempty"`
	CurrentChannel     string           `json:"current_channel,omitempty"`
	Messages           []ModelMessage   `json:"messages,omitempty"`
	Skills             []string         `json:"skills,omitempty"`
	AllowedActions     []string         `json:"allowed_actions,omitempty"`
	Attachments        []ChatAttachment `json:"attachments,omitempty"`
}

// ChatResponse is the output of a non-streaming chat call.
type ChatResponse struct {
	Messages []ModelMessage `json:"messages"`
	Skills   []string       `json:"skills,omitempty"`
	Model    string         `json:"model,omitempty"`
	Provider string         `json:"provider,omitempty"`
}

// StreamChunk is a raw JSON chunk from the streaming response.
type StreamChunk = json.RawMessage

// AssistantOutput holds extracted assistant content for downstream consumers.
type AssistantOutput struct {
	Content string
	Parts   []ContentPart
}
