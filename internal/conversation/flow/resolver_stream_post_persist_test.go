package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	sdk "github.com/memohai/twilight-ai/sdk"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/contextbudget"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

func TestFinishStreamPostPersistPrioritizesMaterializationError(t *testing.T) {
	t.Parallel()

	overflow := &PromptEnvelopeOverflowError{ContextBudget: 10, TotalTokens: 11}
	state := &initialPromptState{}
	state.Store(initialPromptResult{}, overflow)
	rc := resolvedContext{promptState: state}
	postPersistCalls := 0

	err := finishStreamPostPersist(context.Background(), rc, nil, func(context.Context, []messagepkg.Message) error {
		postPersistCalls++
		return errors.New("replacement message was not persisted")
	})
	var gotOverflow *PromptEnvelopeOverflowError
	if !errors.As(err, &gotOverflow) || gotOverflow != overflow {
		t.Fatalf("finishStreamPostPersist() error = %v, want typed overflow", err)
	}
	if postPersistCalls != 0 {
		t.Fatalf("postPersist calls = %d, want 0 after materialization failure", postPersistCalls)
	}
}

func TestStreamPostPersistFailureSettlesTerminalBeforeReturning(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation:      contextbudget.Allocation{CompactableTokens: 31},
	}, nil)
	resolved := resolvedContext{
		runConfig: agentpkg.RunConfig{Model: &sdk.Model{
			ID:       "stream-model",
			Provider: &triggerPromptProvider{},
			Type:     sdk.ModelTypeChat,
		}},
		promptState: state,
	}
	postPersistErr := errors.New("replacement persist failed")
	postPersistCalls := 0
	messages := &failingRecordingMessageService{}
	resolver := &Resolver{
		agent:          agentpkg.New(agentpkg.Deps{Logger: slog.New(slog.DiscardHandler)}),
		logger:         slog.New(slog.DiscardHandler),
		messageService: messages,
		resolveContextFn: func(context.Context, conversation.ChatRequest) (resolvedContext, error) {
			return resolved, nil
		},
	}

	events := make(chan WSStreamEvent, 16)
	_, err := resolver.streamChatWSResultWithHooks(
		context.Background(),
		conversation.ChatRequest{BotID: "bot", SessionID: "session", UserMessagePersisted: true, SkipTitleGeneration: true},
		events,
		make(chan struct{}),
		nil,
		func(context.Context, []messagepkg.Message) error {
			postPersistCalls++
			return postPersistErr
		},
	)
	if !errors.Is(err, postPersistErr) {
		t.Fatalf("streamChatWSResultWithHooks() error = %v, want %v", err, postPersistErr)
	}
	if postPersistCalls != 1 {
		t.Fatalf("postPersist calls = %d, want 1", postPersistCalls)
	}
	if len(messages.persisted) == 0 {
		t.Fatal("terminal persistence was not attempted")
	}
	forwardedTerminal := false
	for len(events) > 0 {
		var event agentpkg.StreamEvent
		if err := json.Unmarshal(<-events, &event); err != nil {
			t.Fatalf("unmarshal forwarded event: %v", err)
		}
		forwardedTerminal = forwardedTerminal || event.IsTerminal()
	}
	if forwardedTerminal {
		t.Fatal("postPersist failure published a successful terminal event")
	}
	if _, _, claimed := resolved.claimCompactionPressure(); claimed {
		t.Fatal("postPersist failure left the prompt receipt unconsumed")
	}
}

func TestStreamPostPersistSuccessPublishesTerminalAfterHook(t *testing.T) {
	t.Parallel()

	resolved := resolvedContext{runConfig: agentpkg.RunConfig{Model: &sdk.Model{
		ID:       "stream-model",
		Provider: &triggerPromptProvider{},
		Type:     sdk.ModelTypeChat,
	}}}
	resolver := &Resolver{
		agent:          agentpkg.New(agentpkg.Deps{Logger: slog.New(slog.DiscardHandler)}),
		logger:         slog.New(slog.DiscardHandler),
		messageService: &recordingMessageService{},
		resolveContextFn: func(context.Context, conversation.ChatRequest) (resolvedContext, error) {
			return resolved, nil
		},
	}
	events := make(chan WSStreamEvent, 16)
	terminalBeforeHook := false

	_, err := resolver.streamChatWSResultWithHooks(
		context.Background(),
		conversation.ChatRequest{BotID: "bot", SessionID: "session", UserMessagePersisted: true, SkipTitleGeneration: true},
		events,
		make(chan struct{}),
		nil,
		func(context.Context, []messagepkg.Message) error {
			for len(events) > 0 {
				var event agentpkg.StreamEvent
				if err := json.Unmarshal(<-events, &event); err != nil {
					t.Fatalf("unmarshal pre-hook event: %v", err)
				}
				terminalBeforeHook = terminalBeforeHook || event.IsTerminal()
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("streamChatWSResultWithHooks() error = %v", err)
	}
	if terminalBeforeHook {
		t.Fatal("terminal event was published before postPersist completed")
	}
	forwardedTerminal := false
	for len(events) > 0 {
		var event agentpkg.StreamEvent
		if err := json.Unmarshal(<-events, &event); err != nil {
			t.Fatalf("unmarshal post-hook event: %v", err)
		}
		forwardedTerminal = forwardedTerminal || event.IsTerminal()
	}
	if !forwardedTerminal {
		t.Fatal("successful postPersist did not publish the terminal event")
	}
}

func TestShouldForwardAgentStreamEventDefersMaterializationErrorToTypedChannel(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{}, errors.New("materialization failed"))
	rc := resolvedContext{promptState: state}
	if shouldForwardAgentStreamEvent(rc, agentpkg.StreamEvent{Type: agentpkg.EventError}) {
		t.Fatal("materialization EventError was forwarded in addition to typed error")
	}
	if !shouldForwardAgentStreamEvent(rc, agentpkg.StreamEvent{Type: agentpkg.EventTextDelta}) {
		t.Fatal("non-error stream event was suppressed")
	}
	if !shouldForwardAgentStreamEvent(resolvedContext{}, agentpkg.StreamEvent{Type: agentpkg.EventError}) {
		t.Fatal("ordinary provider EventError was suppressed")
	}
}

type recordingMessageService struct {
	persisted []messagepkg.PersistInput
	replaced  int
}

func (s *recordingMessageService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persisted = append(s.persisted, input)
	return messagepkg.Message{ID: "message-id", SessionID: input.SessionID, Role: input.Role, Content: input.Content, DisplayContent: input.DisplayText}, nil
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

func (*recordingMessageService) ListLatestBySession(context.Context, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBeforeBySession(context.Context, string, time.Time, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListBeforeMessageBySession(context.Context, string, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) LocateByExternalIDBySession(context.Context, string, string, int32, int32) (messagepkg.LocateResult, error) {
	return messagepkg.LocateResult{}, nil
}

func (*recordingMessageService) GetByIDBySession(context.Context, string, string) (messagepkg.Message, error) {
	return messagepkg.Message{}, nil
}

func (*recordingMessageService) ListVisibleFromBySession(context.Context, string, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) GetVisibleTurnByMessage(context.Context, string, string) (messagepkg.HistoryTurn, error) {
	return messagepkg.HistoryTurn{}, nil
}

func (*recordingMessageService) GetLatestVisibleTurnBySession(context.Context, string) (messagepkg.HistoryTurn, error) {
	return messagepkg.HistoryTurn{}, nil
}

func (s *recordingMessageService) ReplaceTurn(context.Context, string, string, string, string, string) (messagepkg.HistoryTurn, error) {
	s.replaced++
	return messagepkg.HistoryTurn{}, nil
}

func (*recordingMessageService) DeleteByIDs(context.Context, []string) error {
	return nil
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
