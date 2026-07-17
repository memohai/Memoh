package flow

import (
	"strings"
	"time"
)

// UserMessageMeta holds the structured metadata attached to every user
// message. It is the single source of truth for the XML message tag sent to the LLM.
type UserMessageMeta struct {
	MessageID          string   `json:"message-id,omitempty"`
	ChannelIdentityID  string   `json:"channel-identity-id"`
	DisplayName        string   `json:"display-name"`
	Channel            string   `json:"channel"`
	ConversationType   string   `json:"conversation-type"`
	ConversationName   string   `json:"conversation-name,omitempty"`
	Target             string   `json:"target,omitempty"`
	Time               string   `json:"time"`
	Timezone           string   `json:"timezone,omitempty"`
	AttachmentPaths    []string `json:"attachments"`
	ReplyToMessageID   string   `json:"reply-to-message-id,omitempty"`
	ReplySender        string   `json:"reply-sender,omitempty"`
	ReplyPreview       string   `json:"reply-preview,omitempty"`
	ForwardedFrom      string   `json:"forwarded-from,omitempty"`
	ForwardedMessageID string   `json:"forwarded-message-id,omitempty"`
}

// UserMessageHeaderInput is the unified input for building user message headers.
// Keeping this as a struct avoids long positional argument lists and makes
// future metadata extension backward-compatible for call sites.
type UserMessageHeaderInput struct {
	MessageID          string
	ChannelIdentityID  string
	DisplayName        string
	Channel            string
	ConversationType   string
	ConversationName   string
	Target             string
	AttachmentPaths    []string
	Time               time.Time
	Timezone           string
	ReplyToMessageID   string
	ReplySender        string
	ReplyPreview       string
	ForwardedFrom      string
	ForwardedMessageID string
}

// BuildUserMessageMetaFromInput constructs metadata from one cohesive input.
func BuildUserMessageMetaFromInput(input UserMessageHeaderInput) UserMessageMeta {
	attachmentPaths := input.AttachmentPaths
	if attachmentPaths == nil {
		attachmentPaths = []string{}
	}
	meta := UserMessageMeta{
		MessageID:          input.MessageID,
		ChannelIdentityID:  input.ChannelIdentityID,
		DisplayName:        input.DisplayName,
		Channel:            input.Channel,
		ConversationType:   input.ConversationType,
		ConversationName:   input.ConversationName,
		Target:             strings.TrimSpace(input.Target),
		Time:               time.Now().UTC().Format(time.RFC3339),
		Timezone:           strings.TrimSpace(input.Timezone),
		AttachmentPaths:    attachmentPaths,
		ReplyToMessageID:   strings.TrimSpace(input.ReplyToMessageID),
		ReplySender:        strings.TrimSpace(input.ReplySender),
		ReplyPreview:       strings.TrimSpace(input.ReplyPreview),
		ForwardedFrom:      strings.TrimSpace(input.ForwardedFrom),
		ForwardedMessageID: strings.TrimSpace(input.ForwardedMessageID),
	}
	if !input.Time.IsZero() {
		meta.Time = input.Time.Format(time.RFC3339)
	}
	return meta
}

// BuildUserMessageMetaWithTime constructs metadata with an explicit timestamp
// and timezone label for user-facing prompts.
func BuildUserMessageMetaWithTime(messageID, channelIdentityID, displayName, channel, conversationType, conversationName string, attachmentPaths []string, now time.Time, timezone string) UserMessageMeta {
	meta := BuildUserMessageMetaFromInput(UserMessageHeaderInput{
		MessageID:         messageID,
		ChannelIdentityID: channelIdentityID,
		DisplayName:       displayName,
		Channel:           channel,
		ConversationType:  conversationType,
		ConversationName:  conversationName,
		AttachmentPaths:   attachmentPaths,
		Time:              now,
		Timezone:          timezone,
	})
	if !now.IsZero() {
		meta.Time = now.Format(time.RFC3339)
	}
	meta.Timezone = strings.TrimSpace(timezone)
	return meta
}

// ToMap returns the metadata as a map with the same keys used in the XML
// attributes, suitable for storing as inbox content JSONB.
func (m UserMessageMeta) ToMap() map[string]any {
	result := map[string]any{
		"channel-identity-id": m.ChannelIdentityID,
		"display-name":        m.DisplayName,
		"channel":             m.Channel,
		"conversation-type":   m.ConversationType,
		"time":                m.Time,
		"attachments":         m.AttachmentPaths,
	}
	if m.MessageID != "" {
		result["message-id"] = m.MessageID
	}
	if m.ConversationName != "" {
		result["conversation-name"] = m.ConversationName
	}
	if m.Target != "" {
		result["target"] = m.Target
	}
	if strings.TrimSpace(m.Timezone) != "" {
		result["timezone"] = m.Timezone
	}
	if m.ReplyToMessageID != "" {
		result["reply-to-message-id"] = m.ReplyToMessageID
	}
	if m.ReplySender != "" {
		result["reply-sender"] = m.ReplySender
	}
	if m.ReplyPreview != "" {
		result["reply-preview"] = m.ReplyPreview
	}
	if m.ForwardedFrom != "" {
		result["forwarded-from"] = m.ForwardedFrom
	}
	if m.ForwardedMessageID != "" {
		result["forwarded-message-id"] = m.ForwardedMessageID
	}
	return result
}

// FormatUserHeader wraps a user query in an XML <message> tag so the LLM sees
// structured context (sender, channel, conversation, time, attachments)
// alongside the raw message. This must be the single source of truth for
// user-message formatting — the agent gateway must NOT add its own header.
func FormatUserHeader(input UserMessageHeaderInput, query string) string {
	meta := BuildUserMessageMetaFromInput(input)
	return FormatUserHeaderFromMeta(meta, query)
}

// FormatUserHeaderFromMeta formats a pre-built UserMessageMeta into the
// XML <message> string sent to the LLM.
func FormatUserHeaderFromMeta(meta UserMessageMeta, query string) string {
	var sb strings.Builder

	sb.WriteString("<message")
	if meta.MessageID != "" {
		writeXMLAttr(&sb, "id", meta.MessageID)
	}
	writeXMLAttr(&sb, "sender", meta.DisplayName)
	writeXMLAttr(&sb, "t", meta.Time)
	writeXMLAttr(&sb, "channel", meta.Channel)
	if meta.ConversationName != "" {
		writeXMLAttr(&sb, "conversation", meta.ConversationName)
	}
	if meta.ConversationType != "" {
		writeXMLAttr(&sb, "type", meta.ConversationType)
	}
	if meta.Target != "" {
		writeXMLAttr(&sb, "target", meta.Target)
	}
	if meta.ForwardedFrom != "" {
		writeXMLAttr(&sb, "forwarded_from", meta.ForwardedFrom)
	}
	if meta.ForwardedMessageID != "" {
		writeXMLAttr(&sb, "forwarded_message_id", meta.ForwardedMessageID)
	}
	sb.WriteString(">\n")
	if meta.ReplyToMessageID != "" {
		sb.WriteString("<in-reply-to")
		writeXMLAttr(&sb, "id", meta.ReplyToMessageID)
		if meta.ReplySender != "" {
			writeXMLAttr(&sb, "sender", meta.ReplySender)
		}
		sb.WriteString(">")
		sb.WriteString(escapeXMLText(meta.ReplyPreview))
		sb.WriteString("</in-reply-to>\n")
	}

	if len(meta.AttachmentPaths) > 0 {
		for _, p := range meta.AttachmentPaths {
			sb.WriteString("<attachment path=\"")
			sb.WriteString(escapeXMLAttr(p))
			sb.WriteString("\"/>\n")
		}
	}

	sb.WriteString(query)
	sb.WriteString("\n</message>")
	return sb.String()
}

// escapeXMLAttr escapes a string for use inside an XML attribute value.
func escapeXMLAttr(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
	)
	return r.Replace(s)
}

func escapeXMLText(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(s)
}

func writeXMLAttr(sb *strings.Builder, key, value string) {
	sb.WriteByte(' ')
	sb.WriteString(key)
	sb.WriteString("=\"")
	sb.WriteString(escapeXMLAttr(value))
	sb.WriteByte('"')
}
