package flow

import (
	"context"
	"log/slog"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/userinput"
)

type recordingMessageService struct {
	persisted                 []messagepkg.PersistInput
	activeBranchTurnSessionID string
	activeBranchTurnBranchID  string
	activeBranchTurnTurnID    string
	activeBranchTurnCallCount int
	activeBranchOnlyCallCount int
}

func (s *recordingMessageService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persisted = append(s.persisted, input)
	branchID := input.BranchID
	if branchID == "" {
		branchID = "branch-id"
	}
	turnID := input.TurnID
	if turnID == "" {
		turnID = "turn-id"
	}
	return messagepkg.Message{ID: "message-id", Role: input.Role, BranchID: branchID, TurnID: turnID}, nil
}

func (*recordingMessageService) List(context.Context, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListActiveSince(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListLatest(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBefore(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBySession(context.Context, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListActiveSinceBySession(context.Context, string, time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *recordingMessageService) ListActiveSinceBySessionBranch(context.Context, string, string, time.Time) ([]messagepkg.Message, error) {
	s.activeBranchOnlyCallCount++
	return nil, nil
}

func (s *recordingMessageService) ListActiveSinceBySessionBranchTurn(_ context.Context, sessionID, branchID, turnID string, _ time.Time) ([]messagepkg.Message, error) {
	s.activeBranchTurnSessionID = sessionID
	s.activeBranchTurnBranchID = branchID
	s.activeBranchTurnTurnID = turnID
	s.activeBranchTurnCallCount++
	return nil, nil
}

func (*recordingMessageService) ListLatestBySession(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBeforeBySession(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) LocateByExternalIDBySession(context.Context, string, string, int32, int32) (messagepkg.LocateResult, error) {
	return messagepkg.LocateResult{}, nil
}

func (*recordingMessageService) DeleteByBot(context.Context, string) error {
	return nil
}

func (*recordingMessageService) DeleteBySession(context.Context, string) error {
	return nil
}

func (*recordingMessageService) LinkAssets(context.Context, string, []messagepkg.AssetRef) error {
	return nil
}

func TestLoadMessagesUsesPinnedTurnHistory(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, err := resolver.loadMessages(context.Background(), conversation.ChatRequest{
		BotID:           "bot-1",
		SessionID:       "session-1",
		PersistBranchID: "branch-1",
		PersistTurnID:   "turn-1",
	}, defaultMaxContextMinutes)
	if err != nil {
		t.Fatalf("loadMessages returned error: %v", err)
	}
	if messages.activeBranchTurnCallCount != 1 {
		t.Fatalf("pinned turn history calls = %d, want 1", messages.activeBranchTurnCallCount)
	}
	if messages.activeBranchOnlyCallCount != 0 {
		t.Fatalf("branch-only history calls = %d, want 0", messages.activeBranchOnlyCallCount)
	}
	if messages.activeBranchTurnSessionID != "session-1" || messages.activeBranchTurnBranchID != "branch-1" || messages.activeBranchTurnTurnID != "turn-1" {
		t.Fatalf(
			"pinned turn args = (%q, %q, %q), want (session-1, branch-1, turn-1)",
			messages.activeBranchTurnSessionID,
			messages.activeBranchTurnBranchID,
			messages.activeBranchTurnTurnID,
		)
	}
}

type recordingUserInputService struct {
	updatedRequestID string
	updatedBranchID  string
	updatedTurnID    string
}

func (*recordingUserInputService) CreatePending(context.Context, userinput.CreatePendingInput) (userinput.Request, error) {
	return userinput.Request{}, nil
}

func (*recordingUserInputService) ResolveTarget(context.Context, userinput.ResolveInput) (userinput.Request, error) {
	return userinput.Request{}, nil
}

func (*recordingUserInputService) Submit(context.Context, userinput.SubmitInput) (userinput.Request, error) {
	return userinput.Request{}, nil
}

func (*recordingUserInputService) Cancel(context.Context, userinput.CancelInput) (userinput.Request, error) {
	return userinput.Request{}, nil
}

func (*recordingUserInputService) CanRespond(userinput.Request) bool {
	return true
}

func (s *recordingUserInputService) UpdatePersistContext(_ context.Context, requestID, branchID, turnID string) (userinput.Request, error) {
	s.updatedRequestID = requestID
	s.updatedBranchID = branchID
	s.updatedTurnID = turnID
	return userinput.Request{}, nil
}

func TestPersistPartialResultDoesNotStoreUserOnlyFailure(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	resolver.persistPartialResult(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		resolvedContext{},
		nil,
		0,
		false,
		true,
	)

	if len(messages.persisted) != 0 {
		t.Fatalf("expected failed stream not to persist user-only history, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotSkipsUserOnlySnapshot(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	if didStore, err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.UserMessage("hello")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	} else if didStore {
		t.Fatalf("persistTerminalSnapshot stored user-only snapshot")
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected user-only terminal snapshot not to persist, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotSkipsEmptyAssistantSnapshot(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	if didStore, err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	} else if didStore {
		t.Fatalf("persistTerminalSnapshot stored empty assistant snapshot")
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected empty assistant terminal snapshot not to persist, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotStoresAssistantOutput(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	if didStore, err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("partial answer")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	} else if !didStore {
		t.Fatalf("persistTerminalSnapshot did not report persisted assistant output")
	}

	if len(messages.persisted) != 2 {
		t.Fatalf("expected user and assistant messages to persist, got %#v", messages.persisted)
	}
	if messages.persisted[0].Role != "user" || messages.persisted[1].Role != "assistant" {
		t.Fatalf("unexpected persisted roles: %q, %q", messages.persisted[0].Role, messages.persisted[1].Role)
	}
}

func TestPersistTerminalSnapshotUpdatesUserInputPersistContext(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	userInput := &recordingUserInputService{}
	resolver := &Resolver{
		messageService: messages,
		userInput:      userInput,
		logger:         slog.New(slog.DiscardHandler),
	}

	if didStore, err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:           "bot-1",
			SessionID:       "session-1",
			Query:           "hello",
			PersistBranchID: "branch-1",
			PersistTurnID:   "turn-1",
		},
		resolvedContext{},
		terminalSnapshot{
			sdkMessages:    []sdk.Message{sdk.AssistantMessage("answer")},
			deferredToolID: "input-1",
			deferredKind:   userinput.DeferredKind,
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	} else if !didStore {
		t.Fatalf("persistTerminalSnapshot did not report persisted assistant output")
	}

	if userInput.updatedRequestID != "input-1" || userInput.updatedBranchID != "branch-1" || userInput.updatedTurnID != "turn-1" {
		t.Fatalf("updated context = request %q branch %q turn %q, want input-1 branch-1 turn-1",
			userInput.updatedRequestID,
			userInput.updatedBranchID,
			userInput.updatedTurnID,
		)
	}
}
