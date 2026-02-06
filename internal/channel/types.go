package channel

import (
	"fmt"
	"strings"
	"time"
)

type ChannelType string

func ParseChannelType(raw string) (ChannelType, error) {
	normalized := normalizeChannelType(raw)
	if normalized == "" {
		return "", fmt.Errorf("unsupported channel type: %s", raw)
	}
	if _, ok := GetChannelDescriptor(normalized); !ok {
		return "", fmt.Errorf("unsupported channel type: %s", raw)
	}
	return normalized, nil
}

type Identity struct {
	ExternalID  string
	DisplayName string
	Attributes  map[string]string
}

func (i Identity) Attribute(key string) string {
	if i.Attributes == nil {
		return ""
	}
	return strings.TrimSpace(i.Attributes[key])
}

type Conversation struct {
	ID       string
	Type     string
	Name     string
	ThreadID string
	Metadata map[string]any
}

type InboundMessage struct {
	Channel      ChannelType
	Message      Message
	BotID        string
	ReplyTarget  string
	SessionKey   string
	Sender       Identity
	Conversation Conversation
	ReceivedAt   time.Time
	Source       string
	Metadata     map[string]any
}

// SessionID 结构: platform:bot_id:conversation_id[:sender_id]
func (m InboundMessage) SessionID() string {
	if strings.TrimSpace(m.SessionKey) != "" {
		return strings.TrimSpace(m.SessionKey)
	}
	senderID := strings.TrimSpace(m.Sender.ExternalID)
	if senderID == "" {
		senderID = strings.TrimSpace(m.Sender.DisplayName)
	}
	return GenerateSessionID(string(m.Channel), m.BotID, m.Conversation.ID, m.Conversation.Type, senderID)
}

// GenerateSessionID 统一生成 SessionID 的逻辑
func GenerateSessionID(platform, botID, conversationID, conversationType, senderID string) string {
	parts := []string{platform, botID, conversationID}
	// 如果是群聊，增加发送者 ID 以支持个人上下文
	ct := strings.ToLower(strings.TrimSpace(conversationType))
	if ct != "" && ct != "p2p" && ct != "private" {
		senderID = strings.TrimSpace(senderID)
		if senderID != "" {
			parts = append(parts, senderID)
		}
	}
	return strings.Join(parts, ":")
}

type OutboundMessage struct {
	Target  string  `json:"target"`
	Message Message `json:"message"`
}

type MessageFormat string

const (
	MessageFormatPlain    MessageFormat = "plain"
	MessageFormatMarkdown MessageFormat = "markdown"
	MessageFormatRich     MessageFormat = "rich"
)

type MessagePartType string

const (
	MessagePartText      MessagePartType = "text"
	MessagePartLink      MessagePartType = "link"
	MessagePartCodeBlock MessagePartType = "code_block"
	MessagePartMention   MessagePartType = "mention"
	MessagePartEmoji     MessagePartType = "emoji"
)

type MessageTextStyle string

const (
	MessageStyleBold          MessageTextStyle = "bold"
	MessageStyleItalic        MessageTextStyle = "italic"
	MessageStyleStrikethrough MessageTextStyle = "strikethrough"
	MessageStyleCode          MessageTextStyle = "code"
)

type MessagePart struct {
	Type     MessagePartType    `json:"type"`
	Text     string             `json:"text,omitempty"`
	URL      string             `json:"url,omitempty"`
	Styles   []MessageTextStyle `json:"styles,omitempty"`
	Language string             `json:"language,omitempty"`
	UserID   string             `json:"user_id,omitempty"`
	Emoji    string             `json:"emoji,omitempty"`
	Metadata map[string]any     `json:"metadata,omitempty"`
}

type AttachmentType string

const (
	AttachmentImage AttachmentType = "image"
	AttachmentAudio AttachmentType = "audio"
	AttachmentVideo AttachmentType = "video"
	AttachmentVoice AttachmentType = "voice"
	AttachmentFile  AttachmentType = "file"
	AttachmentGIF   AttachmentType = "gif"
)

type Attachment struct {
	Type         AttachmentType `json:"type"`
	URL          string         `json:"url,omitempty"`
	Name         string         `json:"name,omitempty"`
	Size         int64          `json:"size,omitempty"`
	Mime         string         `json:"mime,omitempty"`
	DurationMs   int64          `json:"duration_ms,omitempty"`
	Width        int            `json:"width,omitempty"`
	Height       int            `json:"height,omitempty"`
	ThumbnailURL string         `json:"thumbnail_url,omitempty"`
	Caption      string         `json:"caption,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type Action struct {
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
	Value string `json:"value,omitempty"`
	URL   string `json:"url,omitempty"`
}

type ThreadRef struct {
	ID string `json:"id"`
}

type ReplyRef struct {
	Target    string `json:"target,omitempty"`
	MessageID string `json:"message_id,omitempty"`
}

type Message struct {
	ID          string         `json:"id,omitempty"`
	Format      MessageFormat  `json:"format,omitempty"`
	Text        string         `json:"text,omitempty"`
	Parts       []MessagePart  `json:"parts,omitempty"`
	Attachments []Attachment   `json:"attachments,omitempty"`
	Actions     []Action       `json:"actions,omitempty"`
	Thread      *ThreadRef     `json:"thread,omitempty"`
	Reply       *ReplyRef      `json:"reply,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

func (m Message) IsEmpty() bool {
	return strings.TrimSpace(m.Text) == "" &&
		len(m.Parts) == 0 &&
		len(m.Attachments) == 0 &&
		len(m.Actions) == 0
}

func (m Message) PlainText() string {
	if strings.TrimSpace(m.Text) != "" {
		return strings.TrimSpace(m.Text)
	}
	if len(m.Parts) == 0 {
		return ""
	}
	lines := make([]string, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch part.Type {
		case MessagePartText, MessagePartLink, MessagePartCodeBlock, MessagePartMention, MessagePartEmoji:
			value := strings.TrimSpace(part.Text)
			if value == "" && part.Type == MessagePartLink {
				value = strings.TrimSpace(part.URL)
			}
			if value == "" && part.Type == MessagePartEmoji {
				value = strings.TrimSpace(part.Emoji)
			}
			if value == "" {
				continue
			}
			lines = append(lines, value)
		default:
			continue
		}
	}
	return strings.Join(lines, "\n")
}

type BindingCriteria struct {
	ExternalID string
	Attributes map[string]string
}

func (c BindingCriteria) Attribute(key string) string {
	if c.Attributes == nil {
		return ""
	}
	return strings.TrimSpace(c.Attributes[key])
}

func BindingCriteriaFromIdentity(identity Identity) BindingCriteria {
	return BindingCriteria{
		ExternalID: strings.TrimSpace(identity.ExternalID),
		Attributes: identity.Attributes,
	}
}

type ChannelConfig struct {
	ID               string
	BotID            string
	ChannelType      ChannelType
	Credentials      map[string]any
	ExternalIdentity string
	SelfIdentity     map[string]any
	Routing          map[string]any
	Capabilities     map[string]any
	Status           string
	VerifiedAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type ChannelUserBinding struct {
	ID          string
	ChannelType ChannelType
	UserID      string
	Config      map[string]any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpsertConfigRequest struct {
	Credentials      map[string]any `json:"credentials"`
	ExternalIdentity string         `json:"external_identity,omitempty"`
	SelfIdentity     map[string]any `json:"self_identity,omitempty"`
	Routing          map[string]any `json:"routing,omitempty"`
	Capabilities     map[string]any `json:"capabilities,omitempty"`
	Status           string         `json:"status,omitempty"`
	VerifiedAt       *time.Time     `json:"verified_at,omitempty"`
}

type UpsertUserConfigRequest struct {
	Config map[string]any `json:"config"`
}

type ChannelSession struct {
	SessionID       string
	BotID           string
	ChannelConfigID string
	UserID          string
	ContactID       string
	Platform        string
	ReplyTarget     string
	ThreadID        string
	Metadata        map[string]any
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type SendRequest struct {
	Target  string  `json:"target,omitempty"`
	UserID  string  `json:"user_id,omitempty"`
	Message Message `json:"message"`
}
