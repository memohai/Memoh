package flow

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type deferredContinuationMessageService struct {
	*recordingMessageService
	history   []messagepkg.Message
	persisted []messagepkg.PersistInput
}

func (s *deferredContinuationMessageService) ListBySession(context.Context, string) ([]messagepkg.Message, error) {
	return append([]messagepkg.Message(nil), s.history...), nil
}

func (s *deferredContinuationMessageService) PersistRound(_ context.Context, inputs []messagepkg.PersistInput, _ messagepkg.RoundPersistenceOptions) ([]messagepkg.Message, bool, error) {
	result := make([]messagepkg.Message, 0, len(inputs))
	for _, input := range inputs {
		s.persisted = append(s.persisted, input)
		message := messagepkg.Message{
			ID: input.MessageID, BotID: input.BotID, SessionID: input.SessionID,
			Role: input.Role, Content: input.Content, TurnID: input.TurnID,
			TurnPosition: input.TurnPosition, TurnMessageSeq: input.TurnMessageSeq,
		}
		s.history = append(s.history, message)
		result = append(result, message)
	}
	return result, true, nil
}

func TestDeferredContinuationPreservesTurnAndAppendsRows(t *testing.T) {
	t.Parallel()

	const (
		botID     = "11111111-1111-1111-1111-111111111111"
		sessionID = "22222222-2222-2222-2222-222222222222"
		turnID    = "33333333-3333-4333-8333-333333333333"
		userID    = "44444444-4444-4444-8444-444444444444"
		callID    = "call-1"
	)
	service := &deferredContinuationMessageService{
		recordingMessageService: &recordingMessageService{},
		history: []messagepkg.Message{
			{ID: userID, BotID: botID, SessionID: sessionID, Role: "user", Content: json.RawMessage(`"inspect"`), TurnID: turnID, TurnPosition: 7, TurnMessageSeq: 1},
			{ID: "55555555-5555-4555-8555-555555555555", BotID: botID, SessionID: sessionID, Role: "assistant", Content: json.RawMessage(`[{"type":"tool-call","toolCallId":"call-1"}]`), TurnID: turnID, TurnPosition: 7, TurnMessageSeq: 2},
		},
	}
	resolver := &Resolver{messageService: service, logger: slog.New(slog.DiscardHandler)}
	continuation, err := resolver.prepareDeferredContinuation(context.Background(), sessionID, callID)
	if err != nil {
		t.Fatalf("prepareDeferredContinuation() error = %v", err)
	}
	reservation := continuation.TurnReservation()
	if reservation.TurnID != turnID || reservation.TurnPosition != 7 || reservation.Request.MessageID != userID || reservation.Request.TurnMessageSeq != 1 {
		t.Fatalf("turn reservation = %#v", reservation)
	}
	initialLedger := continuation.InitialRowLedger()
	if len(initialLedger) != 3 || initialLedger[0].StableID != userID || initialLedger[1].Role != "assistant" || initialLedger[2].Role != "tool" || initialLedger[2].TurnMessageSeq != 3 {
		t.Fatalf("initial ledger = %#v", initialLedger)
	}

	toolResult := sdk.ToolResultPart{ToolCallID: callID, ToolName: "exec", Result: "done"}
	continuation, err = resolver.persistDeferredResponse(context.Background(), conversation.ChatRequest{
		BotID: botID, ChatID: botID, SessionID: sessionID, UserMessagePersisted: true,
	}, sdkMessagesToModelMessages([]sdk.Message{sdk.ToolMessage(toolResult)}), continuation)
	if err != nil {
		t.Fatalf("persistDeferredResponse() error = %v", err)
	}
	if len(service.persisted) != 1 {
		t.Fatalf("persisted rows = %d, want 1", len(service.persisted))
	}
	response := service.persisted[0]
	if response.Role != "tool" || response.TurnID != turnID || response.TurnPosition != 7 || response.TurnMessageSeq != 3 || response.MessageID == "" {
		t.Fatalf("response row = %#v", response)
	}

	tracker := newDeferredContinuationRowTracker(continuation)
	start := agentpkg.StreamEvent{Type: agentpkg.EventAgentStart}
	tracker.annotate(&start)
	if len(start.LedgerRows) != 2 || start.LedgerRows[0].TurnMessageSeq != 2 || start.LedgerRows[1].StableID != response.MessageID || start.LedgerRows[1].TurnMessageSeq != 3 {
		t.Fatalf("continuation start ledger = %#v", start.LedgerRows)
	}
	step := agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart}
	tracker.annotate(&step)
	if len(step.LedgerRows) != 1 || step.LedgerRows[0].Role != "assistant" || step.LedgerRows[0].TurnMessageSeq != 4 {
		t.Fatalf("continuation assistant row = %#v", step.LedgerRows)
	}
	terminalRows := tracker.bindTerminalRows([]sdk.Message{sdk.AssistantMessage("complete")})
	if len(terminalRows) != 1 || terminalRows[0].MessageID != step.LedgerRows[0].StableID || terminalRows[0].TurnMessageSeq != 4 {
		t.Fatalf("terminal rows = %#v", terminalRows)
	}
}
