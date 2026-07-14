package decisionruntime

import (
	"context"
	"encoding/json"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/userinput"
)

// TurnService keeps decision routing behind the Channel-owned turn boundary.
// Ordinary turns and plain-text decision lookup stay delegated to the base
// service; approval and ask_user responses use the runtime owner router.
type TurnService struct {
	base   turn.Service
	router *Router
}

func NewTurnService(base turn.Service, router *Router) turn.Service {
	return &TurnService{base: base, router: router}
}

func (s *TurnService) StartTurn(ctx context.Context, cmd turn.StartTurnCommand) (turn.RunHandle, error) {
	return s.base.StartTurn(ctx, cmd)
}

func (s *TurnService) RespondToolApproval(ctx context.Context, input turn.ToolApprovalResponse, output chan<- json.RawMessage) error {
	return s.router.RespondToolApproval(ctx, flow.ToolApprovalResponseInput{
		BotID:                      input.BotID,
		SessionID:                  input.SessionID,
		ActorChannelIdentityID:     input.ActorChannelIdentityID,
		ActorUserID:                input.ActorUserID,
		ApprovalID:                 input.ApprovalID,
		ExplicitID:                 input.ExplicitID,
		ReplyExternalMessageID:     input.ReplyExternalMessageID,
		Decision:                   input.Decision,
		Reason:                     input.Reason,
		ChatToken:                  input.ChatToken,
		SuppressActivePromptAttach: input.SuppressActivePromptAttach,
	}, output)
}

func (s *TurnService) RespondUserInput(ctx context.Context, input turn.UserInputResponse, output chan<- json.RawMessage) error {
	return s.router.RespondUserInput(ctx, flow.UserInputResponseInput{
		BotID:                      input.BotID,
		SessionID:                  input.SessionID,
		ActorChannelIdentityID:     input.ActorChannelIdentityID,
		ActorUserID:                input.ActorUserID,
		UserInputID:                input.UserInputID,
		ExplicitID:                 input.ExplicitID,
		ReplyExternalMessageID:     input.ReplyExternalMessageID,
		Answers:                    input.Answers,
		TextAnswer:                 input.TextAnswer,
		Canceled:                   input.Canceled,
		Reason:                     input.Reason,
		ChatToken:                  input.ChatToken,
		SuppressActivePromptAttach: input.SuppressActivePromptAttach,
	}, output)
}

func (s *TurnService) AdvancePlainTextUserInput(ctx context.Context, input userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error) {
	return s.base.AdvancePlainTextUserInput(ctx, input)
}
