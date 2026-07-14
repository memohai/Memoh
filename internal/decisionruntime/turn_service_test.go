package decisionruntime

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/agent/turn"
	"github.com/memohai/memoh/internal/runtimefence"
	"github.com/memohai/memoh/internal/userinput"
)

type turnServiceTestBase struct {
	plainTextCalls int
}

func (*turnServiceTestBase) StartTurn(context.Context, turn.StartTurnCommand) (turn.RunHandle, error) {
	return nil, nil
}

func (*turnServiceTestBase) RespondToolApproval(context.Context, turn.ToolApprovalResponse, chan<- json.RawMessage) error {
	return nil
}

func (*turnServiceTestBase) RespondUserInput(context.Context, turn.UserInputResponse, chan<- json.RawMessage) error {
	return nil
}

func (b *turnServiceTestBase) AdvancePlainTextUserInput(context.Context, userinput.AdvanceTextInput) (userinput.AdvanceTextResult, error) {
	b.plainTextCalls++
	return userinput.AdvanceTextResult{}, nil
}

func TestTurnServiceRoutesDecisionAndDelegatesPlainTextLookup(t *testing.T) {
	resolver := &routerTestResolver{prepared: runtimefence.PreservedDecision{
		Kind: runtimefence.DecisionToolApproval, ID: decisionTargetID, BotID: decisionBotID, SessionID: decisionSessionID,
	}}
	manager := &routerTestManager{dispatchHandled: true, invokeHandler: true}
	router := newRouter(slog.New(slog.DiscardHandler), manager, resolver)
	base := &turnServiceTestBase{}
	service := NewTurnService(base, router)

	err := service.RespondToolApproval(context.Background(), turn.ToolApprovalResponse{
		BotID: decisionBotID, ApprovalID: decisionTargetID, Decision: "approve", ChatToken: "secret",
	}, nil)
	if err != nil {
		t.Fatalf("respond tool approval: %v", err)
	}
	if len(resolver.respondApproval) != 1 || !resolver.respondApproval[0].ResolveOnly {
		t.Fatalf("routed approval = %#v", resolver.respondApproval)
	}
	if _, err := service.AdvancePlainTextUserInput(context.Background(), userinput.AdvanceTextInput{}); err != nil {
		t.Fatalf("advance plain text input: %v", err)
	}
	if base.plainTextCalls != 1 {
		t.Fatalf("plain text calls = %d, want 1", base.plainTextCalls)
	}
}
