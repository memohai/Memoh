package decisionruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/sessionruntime"
)

func (r *Router) bindCommandHandlers() {
	if r == nil || r.manager == nil || r.resolver == nil {
		return
	}
	bindCommandHandlers(r.manager, r.resolver)
}

// BindCommandHandlers installs the shared owner command decoder on a manager.
// It remains exported for transports that assemble their dependencies through
// setters, while Router owns the production application-level routing policy.
func BindCommandHandlers(manager *sessionruntime.Manager, commandResolver CommandResolver) {
	if manager == nil || commandResolver == nil {
		return
	}
	bindCommandHandlers(manager, commandResolver)
}

func bindCommandHandlers(manager runtimeManager, commandResolver CommandResolver) {
	manager.SetCommandHandler(func(ctx context.Context, command sessionruntime.Command) error {
		switch command.Type {
		case sessionruntime.CommandToolApprovalResponse:
			input, err := decodeToolApprovalCommand(command)
			if err != nil {
				return err
			}
			return commandResolver.RespondToolApproval(ctx, input, nil)
		case sessionruntime.CommandUserInputResponse:
			input, err := decodeUserInputCommand(command)
			if err != nil {
				return err
			}
			return commandResolver.RespondUserInput(ctx, input, nil)
		default:
			return fmt.Errorf("unsupported runtime command %q", command.Type)
		}
	})
	reconciler, ok := commandResolver.(commandReconciler)
	if !ok {
		manager.SetCommandReconciler(nil)
		return
	}
	manager.SetCommandReconciler(func(ctx context.Context, command sessionruntime.Command) (bool, error) {
		switch command.Type {
		case sessionruntime.CommandToolApprovalResponse:
			input, err := decodeToolApprovalCommand(command)
			if err != nil {
				return true, err
			}
			return reconciler.ReconcileToolApprovalResponse(ctx, input)
		case sessionruntime.CommandUserInputResponse:
			input, err := decodeUserInputCommand(command)
			if err != nil {
				return true, err
			}
			return reconciler.ReconcileUserInputResponse(ctx, input)
		default:
			return false, nil
		}
	})
}

func decodeToolApprovalCommand(command sessionruntime.Command) (flow.ToolApprovalResponseInput, error) {
	var input flow.ToolApprovalResponseInput
	if err := json.Unmarshal(command.Payload, &input); err != nil {
		return input, fmt.Errorf("decode tool approval response: %w", err)
	}
	input.BotID = command.BotID
	input.SessionID = command.SessionID
	input.ApprovalID = command.TargetID
	input.ExplicitID = command.TargetID
	input.ReplyExternalMessageID = ""
	input.ChatToken = ""
	input.SuppressActivePromptAttach = true
	input.ResolveOnly = true
	return input, nil
}

func decodeUserInputCommand(command sessionruntime.Command) (flow.UserInputResponseInput, error) {
	var input flow.UserInputResponseInput
	if err := json.Unmarshal(command.Payload, &input); err != nil {
		return input, fmt.Errorf("decode user input response: %w", err)
	}
	input.BotID = command.BotID
	input.SessionID = command.SessionID
	input.UserInputID = command.TargetID
	input.ExplicitID = command.TargetID
	input.ReplyExternalMessageID = ""
	input.ChatToken = ""
	input.SuppressActivePromptAttach = true
	input.ResolveOnly = true
	return input, nil
}
