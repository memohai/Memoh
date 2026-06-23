package flow

import (
	"strings"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/agent/sessionmode"
	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
)

func buildContextFragScope(req conversation.ChatRequest, displayName string, identity agentpkg.SessionContext) contextfrag.Scope {
	channelIdentityID := firstNonEmpty(req.SourceChannelIdentityID, identity.ChannelIdentityID)
	scope := contextfrag.Scope{
		BotID:                     firstNonEmpty(req.BotID, identity.BotID),
		ChatID:                    firstNonEmpty(req.ChatID, identity.ChatID),
		SessionID:                 firstNonEmpty(req.SessionID, identity.SessionID),
		ChannelIdentityID:         strings.TrimSpace(channelIdentityID),
		DisplayName:               strings.TrimSpace(displayName),
		Platform:                  firstNonEmpty(req.CurrentChannel, identity.CurrentPlatform),
		ConversationType:          firstNonEmpty(req.ConversationType, identity.ConversationType),
		ConversationName:          strings.TrimSpace(req.ConversationName),
		ReplyTarget:               firstNonEmpty(req.ReplyTarget, identity.ReplyTarget),
		CurrentMessageID:          strings.TrimSpace(req.ExternalMessageID),
		EventID:                   strings.TrimSpace(req.EventID),
		ReplyToMessageID:          strings.TrimSpace(req.SourceReplyToMessageID),
		ReplySender:               strings.TrimSpace(req.ReplySender),
		MentionsBot:               req.MentionsBot,
		RepliesToBot:              req.RepliesToBot,
		ForwardMessageID:          strings.TrimSpace(req.ForwardMessageID),
		ForwardFromUserID:         strings.TrimSpace(req.ForwardFromUserID),
		ForwardFromConversationID: strings.TrimSpace(req.ForwardFromConversationID),
	}
	scope.Attention = contextFragAttentionReasons(req)
	return scope
}

func contextFragAttentionReasons(req conversation.ChatRequest) []contextfrag.AttentionReason {
	var reasons []contextfrag.AttentionReason
	add := func(reason contextfrag.AttentionReason) {
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}

	switch strings.TrimSpace(req.SessionType) {
	case sessionmode.Schedule:
		add(contextfrag.AttentionSchedule)
	case sessionmode.Heartbeat:
		add(contextfrag.AttentionHeartbeat)
	}
	if req.MentionsBot {
		add(contextfrag.AttentionMention)
	}
	if req.RepliesToBot {
		add(contextfrag.AttentionReply)
	}
	query := strings.TrimSpace(firstNonEmpty(req.RawQuery, req.Query))
	if strings.HasPrefix(query, "/") {
		add(contextfrag.AttentionCommand)
	}
	switch strings.ToLower(strings.TrimSpace(req.ConversationType)) {
	case conversation.KindDirect, "private", "":
		add(contextfrag.AttentionDirect)
	case conversation.KindGroup, conversation.KindThread:
		if len(reasons) == 0 {
			add(contextfrag.AttentionPassive)
		}
	}
	if len(reasons) == 0 {
		add(contextfrag.AttentionPassive)
	}
	return reasons
}
