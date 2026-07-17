package inprocess

import (
	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/conversation"
)

func toChatAttachments(in []turn.Attachment) []conversation.ChatAttachment {
	if in == nil {
		return nil
	}
	out := make([]conversation.ChatAttachment, len(in))
	for i, a := range in {
		out[i] = conversation.ChatAttachment{
			Type:        a.Type,
			Base64:      a.Base64,
			Path:        a.Path,
			URL:         a.URL,
			PlatformKey: a.PlatformKey,
			ContentHash: a.ContentHash,
			Name:        a.Name,
			Mime:        a.Mime,
			Size:        a.Size,
			Metadata:    a.Metadata,
		}
	}
	return out
}

func toSkillActivation(in *turn.SkillActivation) *conversation.SkillActivation {
	if in == nil {
		return nil
	}
	skills := make([]conversation.SkillActivationSkill, len(in.Skills))
	for i, s := range in.Skills {
		skills[i] = conversation.SkillActivationSkill{
			Name:        s.Name,
			DisplayName: s.DisplayName,
			Description: s.Description,
			SourceKind:  s.SourceKind,
			State:       s.State,
		}
	}
	return &conversation.SkillActivation{Skills: skills, Prompt: in.Prompt}
}

func toRequestedSkills(in []turn.RequestedSkillContext) []conversation.RequestedSkillContext {
	if in == nil {
		return nil
	}
	out := make([]conversation.RequestedSkillContext, len(in))
	for i, s := range in {
		out[i] = conversation.RequestedSkillContext{
			Name:           s.Name,
			Description:    s.Description,
			Content:        s.Content,
			SourceKind:     s.SourceKind,
			OpaqueSourceID: s.OpaqueSourceID,
			ContentHash:    s.ContentHash,
			Identity:       s.Identity,
		}
	}
	return out
}

func toInjectMessage(in turn.InjectMessage) conversation.InjectMessage {
	return conversation.InjectMessage{
		Text:            in.Text,
		Attachments:     toChatAttachments(in.Attachments),
		HeaderifiedText: in.HeaderifiedText,
	}
}

func fromAssetRefs(in []turn.OutboundAssetRef) []conversation.OutboundAssetRef {
	if in == nil {
		return nil
	}
	out := make([]conversation.OutboundAssetRef, len(in))
	for i, r := range in {
		out[i] = conversation.OutboundAssetRef{
			ContentHash: r.ContentHash,
			Role:        r.Role,
			Ordinal:     r.Ordinal,
			Mime:        r.Mime,
			SizeBytes:   r.SizeBytes,
			StorageKey:  r.StorageKey,
			Name:        r.Name,
			Metadata:    r.Metadata,
		}
	}
	return out
}

// chatRequestFromCommand translates the pure-data command into the
// resolver's request type, field for field. Function- and channel-typed
// fields (InjectCh, OutboundAssetCollector) are wired by the adapter.
func chatRequestFromCommand(cmd turn.StartTurnCommand) conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:                     cmd.BotID,
		ChatID:                    cmd.ChatID,
		SessionID:                 cmd.SessionID,
		Token:                     cmd.Token,
		UserID:                    cmd.UserID,
		SourceChannelIdentityID:   cmd.SourceChannelIdentityID,
		DisplayName:               cmd.DisplayName,
		RouteID:                   cmd.RouteID,
		ChatToken:                 cmd.ChatToken,
		ExternalMessageID:         cmd.ExternalMessageID,
		ReplyTarget:               cmd.ReplyTarget,
		ConversationType:          cmd.ConversationType,
		ConversationName:          cmd.ConversationName,
		SourceReplyToMessageID:    cmd.SourceReplyToMessageID,
		ReplySender:               cmd.ReplySender,
		ReplyPreview:              cmd.ReplyPreview,
		ReplyAttachments:          toChatAttachments(cmd.ReplyAttachments),
		MentionsBot:               cmd.MentionsBot,
		RepliesToBot:              cmd.RepliesToBot,
		ForwardMessageID:          cmd.ForwardMessageID,
		ForwardFromUserID:         cmd.ForwardFromUserID,
		ForwardFromConversationID: cmd.ForwardFromConversationID,
		ForwardSender:             cmd.ForwardSender,
		ForwardDate:               cmd.ForwardDate,
		Query:                     cmd.Query,
		ModelQuery:                cmd.ModelQuery,
		UserMessageKind:           cmd.UserMessageKind,
		UserVisibleText:           cmd.UserVisibleText,
		SkillActivation:           toSkillActivation(cmd.SkillActivation),
		SkipMemoryExtraction:      cmd.SkipMemoryExtraction,
		SkipTitleGeneration:       cmd.SkipTitleGeneration,
		CurrentChannel:            cmd.CurrentChannel,
		Channels:                  cmd.Channels,
		UserMessagePersisted:      cmd.UserMessagePersisted,
		Attachments:               toChatAttachments(cmd.Attachments),
		RequestedSkills:           toRequestedSkills(cmd.RequestedSkills),
		EventID:                   cmd.EventID,
		Model:                     cmd.Model,
		ReasoningEffort:           cmd.ReasoningEffort,
		WorkspaceTargetID:         cmd.WorkspaceTargetID,
	}
}
