package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/models"
)

type canonicalRecordingMessageService struct {
	recordingMessageService
	starts  []messagepkg.CanonicalTurnStart
	appends [][]messagepkg.PersistInput
	nextID  int
}

func (s *canonicalRecordingMessageService) StartCanonicalTurn(
	_ context.Context,
	start messagepkg.CanonicalTurnStart,
) (messagepkg.CanonicalTurn, messagepkg.Message, error) {
	s.starts = append(s.starts, start)
	request := messagepkg.Message{
		ID:        "request-new",
		BotID:     start.Request.BotID,
		SessionID: start.Request.SessionID,
		Role:      "user",
		Content:   start.Request.Content,
	}
	return messagepkg.CanonicalTurn{
		ID:               "turn-new",
		BotID:            request.BotID,
		SessionID:        request.SessionID,
		RequestMessageID: request.ID,
	}, request, nil
}

func (s *canonicalRecordingMessageService) AppendCanonicalTurn(
	_ context.Context,
	turn messagepkg.CanonicalTurn,
	inputs []messagepkg.PersistInput,
) ([]messagepkg.Message, error) {
	if turn.ID != "turn-new" || turn.RequestMessageID != "request-new" {
		return nil, fmt.Errorf("unexpected canonical turn: %#v", turn)
	}
	s.appends = append(s.appends, append([]messagepkg.PersistInput(nil), inputs...))
	messages := make([]messagepkg.Message, 0, len(inputs))
	for _, input := range inputs {
		s.nextID++
		messages = append(messages, messagepkg.Message{
			ID:        fmt.Sprintf("step-%d", s.nextID),
			BotID:     input.BotID,
			SessionID: input.SessionID,
			Role:      input.Role,
			Content:   input.Content,
		})
	}
	return messages, nil
}

func TestCanonicalStepPersistenceStartsReplacementAndDoesNotDuplicateTerminalStep(t *testing.T) {
	t.Parallel()

	messages := &canonicalRecordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	replacement := &replacementPersistenceState{oldTurnID: "turn-old", reason: "retry"}
	ctx := context.WithValue(context.Background(), replacementPersistenceContextKey{}, replacement)
	state, err := resolver.beginCanonicalStepPersistence(ctx, conversation.ChatRequest{
		BotID:                     "bot-1",
		SessionID:                 "session-1",
		Query:                     "hello",
		RawQuery:                  "hello",
		ReusePersistedUserMessage: true,
		PersistedUserMessageID:    "request-old",
		SkipHistoryTurn:           true,
	}, resolvedContext{model: models.GetResponse{ID: "model-1"}})
	if err != nil {
		t.Fatalf("begin canonical persistence: %v", err)
	}
	if state == nil {
		t.Fatal("canonical persistence state is nil")
	}
	if got := state.requestMessageID(); got != "request-new" {
		t.Fatalf("canonical request message id = %q, want request-new", got)
	}
	if len(messages.starts) != 1 || messages.starts[0].Replacement == nil {
		t.Fatalf("canonical starts = %#v", messages.starts)
	}
	if messages.starts[0].Replacement.OldTurnID != "turn-old" || !replacement.atomicCommitted {
		t.Fatalf("replacement start = %#v, committed=%v", messages.starts[0].Replacement, replacement.atomicCommitted)
	}
	if persisted, committed := state.result(); !committed || len(persisted) != 1 || persisted[0].Role != "user" {
		t.Fatalf("canonical start result = (%#v, %v), want committed user-only turn", persisted, committed)
	}

	cfg := state.attachToRunConfig(agentpkg.RunConfig{})
	cfg.InjectedRecorder("steer", []sdk.ImagePart{{Image: "data:image/png;base64,aW1hZ2U=", MediaType: "image/png"}}, 0)
	step := &sdk.StepResult{Messages: []sdk.Message{sdk.AssistantMessage("answer")}}
	if err := cfg.OnStepCompleted(context.Background(), step); err != nil {
		t.Fatalf("append completed step: %v", err)
	}
	if err := state.appendTerminalSnapshot(context.Background(), terminalSnapshot{sdkMessages: step.Messages}); err != nil {
		t.Fatalf("append terminal snapshot: %v", err)
	}
	if len(messages.appends) != 1 {
		t.Fatalf("canonical append calls = %d, want 1", len(messages.appends))
	}
	if got := messages.appends[0]; len(got) != 2 || got[0].Role != "user" || got[1].Role != "assistant" {
		t.Fatalf("canonical appended roles = %#v", got)
	}
	if !strings.Contains(string(messages.appends[0][0].Content), "data:image/png;base64,aW1hZ2U=") {
		t.Fatalf("canonical injected user content lost image: %s", messages.appends[0][0].Content)
	}
	persisted, committed := state.result()
	if !committed || len(persisted) != 3 {
		t.Fatalf("canonical result = (%d, %v), want (3, true)", len(persisted), committed)
	}
}
