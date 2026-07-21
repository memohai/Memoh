package inprocess

import (
	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
)

// chatRequestFromCommand translates the pure-data command into the
// resolver's request type, field for field. Function- and channel-typed
// fields (InjectCh, OutboundAssetCollector) are wired by the adapter.
// Data-carrier fields are type aliases, so assignment is direct.
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
		ReplyAttachments:          cmd.ReplyAttachments,
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
		SkillActivation:           cmd.SkillActivation,
		SkipMemoryExtraction:      cmd.SkipMemoryExtraction,
		SkipTitleGeneration:       cmd.SkipTitleGeneration,
		CurrentChannel:            cmd.CurrentChannel,
		Channels:                  cmd.Channels,
		UserMessagePersisted:      cmd.UserMessagePersisted,
		Attachments:               cmd.Attachments,
		RequestedSkills:           cmd.RequestedSkills,
		EventID:                   cmd.EventID,
		Model:                     cmd.Model,
		ReasoningEffort:           cmd.ReasoningEffort,
		WorkspaceTargetID:         cmd.WorkspaceTargetID,
	}
}

func toolApprovalInputFromResponse(in turn.ToolApprovalResponse) flow.ToolApprovalResponseInput {
	return flow.ToolApprovalResponseInput{
		BotID:                      in.BotID,
		SessionID:                  in.SessionID,
		ActorChannelIdentityID:     in.ActorChannelIdentityID,
		ActorUserID:                in.ActorUserID,
		ApprovalID:                 in.ApprovalID,
		ExplicitID:                 in.ExplicitID,
		ReplyExternalMessageID:     in.ReplyExternalMessageID,
		Decision:                   in.Decision,
		Reason:                     in.Reason,
		ChatToken:                  in.ChatToken,
		SuppressActivePromptAttach: in.SuppressActivePromptAttach,
	}
}

func userInputInputFromResponse(in turn.UserInputResponse) flow.UserInputResponseInput {
	return flow.UserInputResponseInput{
		BotID:                      in.BotID,
		SessionID:                  in.SessionID,
		ActorChannelIdentityID:     in.ActorChannelIdentityID,
		ActorUserID:                in.ActorUserID,
		UserInputID:                in.UserInputID,
		ExplicitID:                 in.ExplicitID,
		ReplyExternalMessageID:     in.ReplyExternalMessageID,
		Answers:                    in.Answers,
		TextAnswer:                 in.TextAnswer,
		Canceled:                   in.Canceled,
		Reason:                     in.Reason,
		ChatToken:                  in.ChatToken,
		SuppressActivePromptAttach: in.SuppressActivePromptAttach,
	}
}
