package pipeline

import (
	"strings"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

// AdaptInbound converts a channel.InboundMessage into a pipeline MessageEvent.
// This is the primary adaptation path — all channel adapters normalize to
// InboundMessage before reaching the pipeline.
func AdaptInbound(msg channel.InboundMessage, sessionID, channelIdentityID, displayName string) MessageEvent {
	now := msg.ReceivedAt
	if now.IsZero() {
		now = time.Now()
	}

	var sender *CanonicalUser
	if channelIdentityID != "" || displayName != "" {
		sender = &CanonicalUser{
			ID:          channelIdentityID,
			DisplayName: displayName,
			Username:    strings.TrimSpace(msg.Sender.Attribute("username")),
			IsBot:       metadataBool(msg.Metadata, "is_bot"),
		}
	}

	content := adaptContent(msg.Message.Text)
	attachments := adaptAttachments(msg.Message.Attachments)

	var replyToMessageID string
	if msg.Message.Reply != nil {
		replyToMessageID = strings.TrimSpace(msg.Message.Reply.MessageID)
	}

	_, offset := now.Zone()
	utcOffsetMin := offset / 60

	convType := channel.NormalizeConversationType(msg.Conversation.Type)

	return MessageEvent{
		SessionID:        sessionID,
		MessageID:        strings.TrimSpace(msg.Message.ID),
		Sender:           sender,
		ReceivedAtMs:     now.UnixMilli(),
		TimestampSec:     now.Unix(),
		UTCOffsetMin:     utcOffsetMin,
		Content:          content,
		ReplyToMessageID: replyToMessageID,
		Attachments:      attachments,
		Conversation: ConversationMeta{
			Channel:          msg.Channel.String(),
			ConversationName: strings.TrimSpace(msg.Conversation.Name),
			ConversationType: convType,
		},
	}
}

func adaptContent(text string) []ContentNode {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return []ContentNode{{Type: "text", Text: text}}
}

func adaptAttachments(atts []channel.Attachment) []Attachment {
	if len(atts) == 0 {
		return nil
	}
	result := make([]Attachment, 0, len(atts))
	for _, a := range atts {
		att := Attachment{
			Type:     string(a.Type),
			MimeType: strings.TrimSpace(a.Mime),
			FileName: strings.TrimSpace(a.Name),
			Width:    a.Width,
			Height:   a.Height,
		}
		if a.DurationMs > 0 {
			att.Duration = int(a.DurationMs / 1000)
		}
		if ref := a.Reference(); ref != "" {
			att.FilePath = ref
		}
		result = append(result, att)
	}
	return result
}

func metadataBool(meta map[string]any, key string) bool {
	if meta == nil {
		return false
	}
	v, ok := meta[key]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(val, "true") || val == "1"
	default:
		return false
	}
}
