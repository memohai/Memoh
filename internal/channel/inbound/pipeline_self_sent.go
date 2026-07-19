package inbound

import (
	"strings"

	"github.com/memohai/memoh/internal/channel"
)

var channelSelfIdentityKeys = []string{
	"user_id", "userId",
	"open_id", "openId",
	"bot_id", "botId",
	"aibot_id", "aibotId",
	"bot_user_id", "botUserId",
}

func pipelineInboundMessage(cfg channel.ChannelConfig, msg channel.InboundMessage) channel.InboundMessage {
	if msg.IsSelfSent || !matchesChannelSelfIdentity(cfg, msg.Sender.SubjectID) {
		return msg
	}
	msg.IsSelfSent = true
	return msg
}

func matchesChannelSelfIdentity(cfg channel.ChannelConfig, subjectID string) bool {
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "" {
		return false
	}
	if matchesExternalIdentity(subjectID, cfg.ExternalIdentity) {
		return true
	}
	for _, key := range channelSelfIdentityKeys {
		if subjectID == strings.TrimSpace(channel.ReadString(cfg.SelfIdentity, key)) {
			return true
		}
	}
	return false
}

func matchesExternalIdentity(subjectID, externalIdentity string) bool {
	externalIdentity = strings.TrimSpace(externalIdentity)
	if subjectID == externalIdentity {
		return true
	}
	prefix, value, ok := strings.Cut(externalIdentity, ":")
	if !ok || !isChannelSelfIdentityKey(prefix) {
		return false
	}
	return subjectID == strings.TrimSpace(value)
}

func isChannelSelfIdentityKey(candidate string) bool {
	for _, key := range channelSelfIdentityKeys {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return true
		}
	}
	return false
}
