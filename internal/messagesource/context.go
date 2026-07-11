package messagesource

import "strings"

const Version1 = 1

type Context struct {
	Version           int    `json:"version"`
	SenderDisplayName string `json:"sender_display_name,omitempty"`
	Platform          string `json:"platform,omitempty"`
	ConversationType  string `json:"conversation_type,omitempty"`
	ConversationName  string `json:"conversation_name,omitempty"`
}

func NewV1(senderDisplayName, platform, conversationType, conversationName string) Context {
	return Context{
		Version:           Version1,
		SenderDisplayName: strings.TrimSpace(senderDisplayName),
		Platform:          strings.TrimSpace(platform),
		ConversationType:  strings.TrimSpace(conversationType),
		ConversationName:  strings.TrimSpace(conversationName),
	}
}

func (context Context) Normalize() Context {
	context.SenderDisplayName = strings.TrimSpace(context.SenderDisplayName)
	context.Platform = strings.TrimSpace(context.Platform)
	context.ConversationType = strings.TrimSpace(context.ConversationType)
	context.ConversationName = strings.TrimSpace(context.ConversationName)
	return context
}
