package flow

import (
	"strings"
	"time"
)

// UserMessageMeta holds the structured metadata attached to every user
// message. It is the single source of truth for the YAML header sent to the LLM.
type UserMessageMeta struct {
	MessageID         string   `json:"message-id,omitempty"`
	ChannelIdentityID string   `json:"channel-identity-id"`
	DisplayName       string   `json:"display-name"`
	Channel           string   `json:"channel"`
	ConversationType  string   `json:"conversation-type"`
	ConversationName  string   `json:"conversation-name,omitempty"`
	Time              string   `json:"time"`
	AttachmentPaths   []string `json:"attachments"`
}

// BuildUserMessageMeta constructs a UserMessageMeta from the inbound parameters.
func BuildUserMessageMeta(messageID, channelIdentityID, displayName, channel, conversationType, conversationName string, attachmentPaths []string) UserMessageMeta {
	if attachmentPaths == nil {
		attachmentPaths = []string{}
	}
	return UserMessageMeta{
		MessageID:         messageID,
		ChannelIdentityID: channelIdentityID,
		DisplayName:       displayName,
		Channel:           channel,
		ConversationType:  conversationType,
		ConversationName:  conversationName,
		Time:              time.Now().UTC().Format(time.RFC3339),
		AttachmentPaths:   attachmentPaths,
	}
}

// ToMap returns the metadata as a map with the same keys used in the YAML header.
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
	return result
}

// FormatUserHeader wraps a user query with YAML front-matter metadata so
// the LLM sees structured context (sender, channel, time, attachments)
// alongside the raw message. This must be the single source of truth for
// user-message formatting — the agent gateway must NOT add its own header.
func FormatUserHeader(messageID, channelIdentityID, displayName, channel, conversationType, conversationName string, attachmentPaths []string, query string) string {
	meta := BuildUserMessageMeta(messageID, channelIdentityID, displayName, channel, conversationType, conversationName, attachmentPaths)
	return FormatUserHeaderFromMeta(meta, query)
}

// FormatUserHeaderFromMeta formats a pre-built UserMessageMeta into the
// YAML front-matter string sent to the LLM.
func FormatUserHeaderFromMeta(meta UserMessageMeta, query string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	if meta.MessageID != "" {
		writeYAMLString(&sb, "message-id", meta.MessageID)
	}
	writeYAMLString(&sb, "channel-identity-id", meta.ChannelIdentityID)
	writeYAMLString(&sb, "display-name", meta.DisplayName)
	writeYAMLString(&sb, "channel", meta.Channel)
	writeYAMLString(&sb, "conversation-type", meta.ConversationType)
	if meta.ConversationName != "" {
		writeYAMLString(&sb, "conversation-name", meta.ConversationName)
	}
	writeYAMLString(&sb, "time", meta.Time)
	if len(meta.AttachmentPaths) > 0 {
		sb.WriteString("attachments:\n")
		for _, p := range meta.AttachmentPaths {
			sb.WriteString("  - ")
			sb.WriteString(p)
			sb.WriteByte('\n')
		}
	} else {
		sb.WriteString("attachments: []\n")
	}
	sb.WriteString("---\n")
	sb.WriteString(query)
	return sb.String()
}

func writeYAMLString(sb *strings.Builder, key, value string) {
	sb.WriteString(key)
	sb.WriteString(": ")
	if value == "" || needsYAMLQuote(value) {
		sb.WriteByte('"')
		sb.WriteString(strings.ReplaceAll(value, `"`, `\"`))
		sb.WriteByte('"')
	} else {
		sb.WriteString(value)
	}
	sb.WriteByte('\n')
}

func needsYAMLQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, c := range s {
		if c == ':' || c == '#' || c == '"' || c == '\'' || c == '{' || c == '}' || c == '[' || c == ']' || c == ',' || c == '\n' {
			return true
		}
	}
	return false
}
