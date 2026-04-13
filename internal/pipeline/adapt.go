package pipeline

import (
	"strings"
	"time"

	"github.com/memohai/memoh/internal/channel"
)

// AdaptInbound converts a channel.InboundMessage into a pipeline CanonicalEvent.
// The event type is determined by the "event_type" metadata key set by channel
// adapters: "edit" → EditEvent, "service" → ServiceEvent. All other messages
// (including the default) produce a MessageEvent.
func AdaptInbound(msg channel.InboundMessage, sessionID, channelIdentityID, displayName string) CanonicalEvent {
	eventType, _ := msg.Metadata["event_type"].(string)
	switch eventType {
	case "edit":
		return adaptEdit(msg, sessionID, channelIdentityID, displayName)
	case "service":
		return adaptService(msg, sessionID)
	default:
		return adaptMessage(msg, sessionID, channelIdentityID, displayName)
	}
}

func adaptMessage(msg channel.InboundMessage, sessionID, channelIdentityID, displayName string) MessageEvent {
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

	var replyToMessageID, replyToSender, replyToPreview string
	if msg.Message.Reply != nil {
		replyToMessageID = strings.TrimSpace(msg.Message.Reply.MessageID)
		replyToSender = strings.TrimSpace(msg.Message.Reply.Sender)
		replyToPreview = strings.TrimSpace(msg.Message.Reply.Preview)
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
		ReplyToSender:    replyToSender,
		ReplyToPreview:   replyToPreview,
		Attachments:      attachments,
		Conversation: ConversationMeta{
			Channel:          msg.Channel.String(),
			ConversationName: strings.TrimSpace(msg.Conversation.Name),
			ConversationType: convType,
			Target:           strings.TrimSpace(msg.ReplyTarget),
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
			Type:        string(a.Type),
			MimeType:    strings.TrimSpace(a.Mime),
			FileName:    strings.TrimSpace(a.Name),
			ContentHash: strings.TrimSpace(a.ContentHash),
			Width:       a.Width,
			Height:      a.Height,
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

func adaptEdit(msg channel.InboundMessage, sessionID, channelIdentityID, displayName string) EditEvent {
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

	_, offset := now.Zone()
	return EditEvent{
		SessionID:    sessionID,
		MessageID:    strings.TrimSpace(msg.Message.ID),
		Sender:       sender,
		ReceivedAtMs: now.UnixMilli(),
		TimestampSec: now.Unix(),
		UTCOffsetMin: offset / 60,
		Content:      adaptContent(msg.Message.Text),
		Attachments:  adaptAttachments(msg.Message.Attachments),
	}
}

func adaptService(msg channel.InboundMessage, sessionID string) ServiceEvent {
	now := msg.ReceivedAt
	if now.IsZero() {
		now = time.Now()
	}

	action, _ := msg.Metadata["service_action"].(string)
	var actor *CanonicalUser
	if msg.Sender.SubjectID != "" || msg.Sender.DisplayName != "" {
		actor = &CanonicalUser{
			ID:          strings.TrimSpace(msg.Sender.SubjectID),
			DisplayName: strings.TrimSpace(msg.Sender.DisplayName),
			Username:    strings.TrimSpace(msg.Sender.Attribute("username")),
		}
	}

	_, offset := now.Zone()
	event := ServiceEvent{
		SessionID:    sessionID,
		Action:       ServiceAction(action),
		Actor:        actor,
		ReceivedAtMs: now.UnixMilli(),
		TimestampSec: now.Unix(),
		UTCOffsetMin: offset / 60,
	}
	if title, ok := msg.Metadata["new_title"].(string); ok {
		event.NewTitle = title
	}
	if title, ok := msg.Metadata["old_title"].(string); ok {
		event.OldTitle = title
	}
	return event
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
