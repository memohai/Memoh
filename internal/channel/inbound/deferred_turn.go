package inbound

import (
	"context"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/route"
	"github.com/memohai/memoh/internal/conversation"
)

type deferredTurn struct {
	id                  string
	ctx                 context.Context
	cfg                 channel.ChannelConfig
	msg                 channel.InboundMessage
	sender              channel.StreamReplySender
	identity            InboundIdentity
	resolved            route.ResolveConversationResult
	resolvedAttachments []channel.Attachment
	attachments         []conversation.ChatAttachment
	replyAttachments    []conversation.ChatAttachment
	text                string
	modelText           string
	userMessageKind     string
	userVisibleText     string
	requestedSkills     []conversation.RequestedSkillContext
	skillActivation     *conversation.SkillActivation
	hasPendingSkill     bool
	sessionID           string
	sessionRuntimeOwner string
	acpRuntimeSession   SessionResult
	activeChatID        string
	inboundMode         InboundMode
	eventID             string
	receipt             conversation.UserMessageReceipt
}
