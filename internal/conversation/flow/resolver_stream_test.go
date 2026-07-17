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
	"github.com/memohai/memoh/internal/conversation"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/settings"
)

type recordingMessageService struct {
	persisted []messagepkg.PersistInput
	replaced  int
	deleted   [][]string
}

func TestSendWSAgentEventWaitsForTerminalConsumerAfterExecutionCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	ctx = WithTerminalEventDeliveryTimeout(ctx, time.Second)
	cancel()
	eventCh := make(chan WSStreamEvent)
	event := agentpkg.StreamEvent{Type: agentpkg.EventAgentAbort, HistoryCommitted: true}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal terminal event: %v", err)
	}

	delivered := make(chan bool, 1)
	go func() {
		delivered <- sendWSAgentEvent(ctx, eventCh, event, data)
	}()
	select {
	case result := <-delivered:
		t.Fatalf("terminal delivery returned before a consumer was available: %v", result)
	case <-time.After(25 * time.Millisecond):
	}
	var got agentpkg.StreamEvent
	if err := json.Unmarshal(<-eventCh, &got); err != nil {
		t.Fatalf("unmarshal delivered terminal event: %v", err)
	}
	if !<-delivered {
		t.Fatal("terminal event was dropped after execution cancellation")
	}
	if got.Type != agentpkg.EventAgentAbort || !got.HistoryCommitted {
		t.Fatalf("delivered terminal event = %#v", got)
	}
}

func TestSendWSAgentEventBoundsTerminalDeliveryWithoutConsumer(t *testing.T) {
	t.Parallel()

	ctx := WithTerminalEventDeliveryTimeout(context.Background(), 25*time.Millisecond)
	event := agentpkg.StreamEvent{Type: agentpkg.EventAgentEnd, HistoryCommitted: true}
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal terminal event: %v", err)
	}
	started := time.Now()
	if sendWSAgentEvent(ctx, make(chan WSStreamEvent), event, data) {
		t.Fatal("terminal event unexpectedly reached a missing consumer")
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond || elapsed > time.Second {
		t.Fatalf("terminal delivery bound elapsed = %s", elapsed)
	}
}

func TestPrepareRunConfigCarriesTerminalHookAuthority(t *testing.T) {
	t.Parallel()

	authorityCtx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	validated := false
	authority := agentpkg.TerminalHookAuthority{
		Context: authorityCtx,
		Validate: func(context.Context) error {
			validated = true
			return nil
		},
	}
	resolver := &Resolver{}
	got := resolver.prepareRunConfig(WithTerminalHookAuthority(context.Background(), authority), agentpkg.RunConfig{})
	if got.TerminalHookAuthority.Context != authorityCtx || got.TerminalHookAuthority.Validate == nil {
		t.Fatalf("terminal hook authority = %#v", got.TerminalHookAuthority)
	}
	if err := got.TerminalHookAuthority.Validate(context.Background()); err != nil {
		t.Fatalf("validate terminal hook authority: %v", err)
	}
	if !validated {
		t.Fatal("prepared terminal hook authority did not retain its validator")
	}
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

func (*recordingMessageService) ListVisibleFromBySession(context.Context, string, string, int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) ListVisibleMessagesByTurnIDBySession(context.Context, string, string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (*recordingMessageService) GetVisibleTurnByMessage(context.Context, string, string) (messagepkg.HistoryTurn, error) {
	return messagepkg.HistoryTurn{}, nil
}

func (*recordingMessageService) GetLatestVisibleTurnBySession(context.Context, string) (messagepkg.HistoryTurn, error) {
	return messagepkg.HistoryTurn{}, nil
}

func (*recordingMessageService) ReplaceTurn(context.Context, string, string, string, string, string) (messagepkg.HistoryTurn, error) {
	return messagepkg.HistoryTurn{}, nil
}

func (s *recordingMessageService) DeleteByIDs(_ context.Context, ids []string) error {
	s.deleted = append(s.deleted, append([]string(nil), ids...))
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

func TestPersistPartialResultDoesNotStoreUserOnlyFailure(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	_, _ = resolver.persistPartialResult(
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

	if err := resolver.persistTerminalSnapshot(
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

	if err := resolver.persistTerminalSnapshot(
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
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected empty assistant terminal snapshot not to persist, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotSkipsAbortedSnapshotBeforeVisibleOutput(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "hello",
		},
		resolvedContext{},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("partial answer")},
			aborted:     true,
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 0 {
		t.Fatalf("expected pre-output abort not to persist, got %#v", messages.persisted)
	}
}

func TestPersistTerminalSnapshotStoresAssistantOutput(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	if err := resolver.persistTerminalSnapshot(
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
	}

	if len(messages.persisted) != 2 {
		t.Fatalf("expected user and assistant messages to persist, got %#v", messages.persisted)
	}
	if messages.persisted[0].Role != "user" || messages.persisted[1].Role != "assistant" {
		t.Fatalf("unexpected persisted roles: %q, %q", messages.persisted[0].Role, messages.persisted[1].Role)
	}
}

func TestRuntimeInjectedMessagePersistsInReservedTurnOrder(t *testing.T) {
	t.Parallel()

	turn := runtimeRowTestTurn()
	tracker := newRuntimeRowTracker(turn)
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd})

	records := make([]conversation.InjectedMessageRecord, 0, 1)
	rc := resolvedContext{
		injectedRecords: &records,
		recordInjectedMessage: func(record conversation.InjectedMessageRecord) {
			records = append(records, record)
		},
	}
	cfg := agentpkg.RunConfig{
		InjectedRecorder: func(string, int) {
			t.Fatal("runtime recorder did not replace the unreserved recorder")
		},
	}
	bindRuntimeInjectedRecorder(&cfg, rc, tracker)
	cfg.InjectedRecorder("---\nsource: web\n---\nchange direction", 2)

	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	sdkMessages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "read-1",
				ToolName:   "read",
				Input:      map[string]any{"path": "README.md"},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{
			ToolCallID: "read-1",
			ToolName:   "read",
			Result:     "contents",
		}),
		sdk.AssistantMessage("updated answer"),
	}
	runtimeRows := tracker.bindTerminalRows(sdkMessages)

	messages := &atomicRecordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	_, err := resolver.persistTerminalSnapshotResult(
		context.Background(),
		conversation.ChatRequest{
			BotID:                storeRoundBotID,
			SessionID:            "33333333-3333-3333-3333-333333333333",
			Query:                "initial request",
			RuntimeTurn:          turn,
			SkipMemoryExtraction: true,
		},
		rc,
		terminalSnapshot{
			sdkMessages:   sdkMessages,
			runtimeRows:   runtimeRows,
			visibleOutput: true,
		},
	)
	if err != nil {
		t.Fatalf("persistTerminalSnapshotResult() error = %v", err)
	}

	if len(messages.roundInputs) != 5 {
		t.Fatalf("persisted rows = %d, want 5", len(messages.roundInputs))
	}
	wantRoles := []string{"user", "assistant", "tool", "user", "assistant"}
	for i, input := range messages.roundInputs {
		if input.Role != wantRoles[i] {
			t.Fatalf("row %d role = %q, want %q", i, input.Role, wantRoles[i])
		}
		if input.MessageID == "" || input.TurnID != turn.TurnID || input.TurnPosition != turn.TurnPosition || input.TurnMessageSeq != int64(i+1) {
			t.Fatalf("row %d reservation = %#v, want complete sequence %d", i, input, i+1)
		}
	}
}

func TestRuntimeReadMediaSyntheticUserPersistsInReservedTurnOrder(t *testing.T) {
	t.Parallel()

	turn := runtimeRowTestTurn()
	tracker := newRuntimeRowTracker(turn)
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventToolCallEnd})
	cfg := agentpkg.RunConfig{}
	bindRuntimeSyntheticRowRecorder(&cfg, tracker)
	cfg.SyntheticRowRecorder("user")
	tracker.annotate(&agentpkg.StreamEvent{Type: agentpkg.EventModelStepStart})

	sdkMessages := []sdk.Message{
		{
			Role: sdk.MessageRoleAssistant,
			Content: []sdk.MessagePart{sdk.ToolCallPart{
				ToolCallID: "read-1",
				ToolName:   "read",
				Input:      map[string]any{"path": "diagram.png"},
			}},
		},
		sdk.ToolMessage(sdk.ToolResultPart{
			ToolCallID: "read-1",
			ToolName:   "read",
			Result:     map[string]any{"ok": true},
		}),
		{
			Role: sdk.MessageRoleUser,
			Content: []sdk.MessagePart{sdk.ImagePart{
				Image:     "data:image/png;base64,cGl4ZWxz",
				MediaType: "image/png",
			}},
		},
		sdk.AssistantMessage("the diagram is valid"),
	}
	runtimeRows := tracker.bindTerminalRows(sdkMessages)
	messages := &atomicRecordingMessageService{}
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	_, err := resolver.persistTerminalSnapshotResult(
		context.Background(),
		conversation.ChatRequest{
			BotID: storeRoundBotID, SessionID: "33333333-3333-3333-3333-333333333333",
			Query: "inspect the diagram", RuntimeTurn: turn, SkipMemoryExtraction: true,
		},
		resolvedContext{},
		terminalSnapshot{sdkMessages: sdkMessages, runtimeRows: runtimeRows, visibleOutput: true},
	)
	if err != nil {
		t.Fatalf("persistTerminalSnapshotResult() error = %v", err)
	}

	if len(messages.roundInputs) != 5 {
		t.Fatalf("persisted rows = %d, want 5", len(messages.roundInputs))
	}
	wantRoles := []string{"user", "assistant", "tool", "user", "assistant"}
	for i, input := range messages.roundInputs {
		if input.Role != wantRoles[i] || input.MessageID == "" || input.TurnMessageSeq != int64(i+1) {
			t.Fatalf("persisted row %d = %#v, want %s sequence %d", i, input, wantRoles[i], i+1)
		}
	}
}

func TestBindRuntimeInjectedRecorderLeavesNonRuntimeRecorderUntouched(t *testing.T) {
	t.Parallel()

	called := false
	cfg := agentpkg.RunConfig{InjectedRecorder: func(string, int) { called = true }}
	bindRuntimeInjectedRecorder(&cfg, resolvedContext{}, nil)
	cfg.InjectedRecorder("ordinary injection", 0)
	if !called {
		t.Fatal("non-runtime injection recorder was replaced")
	}
}

func TestPersistTerminalSnapshotStopsBeforeWriteWhenPersistenceGuardFails(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	ownershipLost := errors.New("runtime ownership lost")
	ctx := WithPersistenceGuard(context.Background(), func(context.Context) error {
		return ownershipLost
	})

	err := resolver.persistTerminalSnapshot(
		ctx,
		conversation.ChatRequest{BotID: "bot-1", SessionID: "session-1", Query: "hello"},
		resolvedContext{},
		terminalSnapshot{
			sdkMessages:   []sdk.Message{sdk.AssistantMessage("late output")},
			visibleOutput: true,
		},
	)
	if !errors.Is(err, ownershipLost) {
		t.Fatalf("persist terminal snapshot error = %v, want ownership loss", err)
	}
	if len(messages.persisted) != 0 {
		t.Fatalf("ownership-lost stream persisted messages: %#v", messages.persisted)
	}
}

func TestHasVisibleAgentStreamOutputIgnoresLifecycleOnlyEvents(t *testing.T) {
	t.Parallel()

	cases := []agentpkg.StreamEvent{
		{Type: agentpkg.EventTextStart},
		{Type: agentpkg.EventTextDelta, Delta: "  \n\t"},
		{Type: agentpkg.EventTextEnd},
		{Type: agentpkg.EventReasoningStart},
		{Type: agentpkg.EventReasoningDelta, Delta: ""},
		{Type: agentpkg.EventReasoningEnd},
		{Type: agentpkg.EventAttachment},
		{Type: agentpkg.EventAgentAbort},
	}
	for _, event := range cases {
		if hasVisibleAgentStreamOutput(event) {
			t.Fatalf("event %q unexpectedly counted as visible", event.Type)
		}
	}

	visible := []agentpkg.StreamEvent{
		{Type: agentpkg.EventTextDelta, Delta: "hello"},
		{Type: agentpkg.EventReasoningDelta, Delta: "thinking"},
		{Type: agentpkg.EventAttachment, Attachments: []agentpkg.FileAttachment{{Path: "/tmp/a.png"}}},
		{Type: agentpkg.EventToolCallStart},
		{Type: agentpkg.EventUserInputRequest},
	}
	for _, event := range visible {
		if !hasVisibleAgentStreamOutput(event) {
			t.Fatalf("event %q unexpectedly counted as invisible", event.Type)
		}
	}
}

func TestPersistTerminalSnapshotPersistsUserWhenPipelineContextContainsCurrentMessage(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		conversation.ChatRequest{
			BotID:     "bot-1",
			SessionID: "session-1",
			Query:     "---\nmessage-id: tg-1\nchannel: telegram\n---\n@memoh1bot ping",
		},
		resolvedContext{
			userMessageAlreadyInContext: true,
		},
		terminalSnapshot{
			sdkMessages: []sdk.Message{sdk.AssistantMessage("pong")},
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 2 {
		t.Fatalf("expected user and assistant output to persist, got %#v", messages.persisted)
	}
	if messages.persisted[0].Role != "user" {
		t.Fatalf("unexpected first persisted role: %q", messages.persisted[0].Role)
	}
	if messages.persisted[1].Role != "assistant" {
		t.Fatalf("unexpected second persisted role: %q", messages.persisted[1].Role)
	}
}

func TestPersistTerminalSnapshotHonorsSkipMemoryExtraction(t *testing.T) {
	t.Parallel()

	memory := &storeRoundMemoryProvider{afterChat: make(chan memprovider.AfterChatRequest, 2)}
	registry := memprovider.NewRegistry(slog.New(slog.DiscardHandler))
	registry.Register(storeRoundMemoryProviderID, memory)
	resolver := &Resolver{
		messageService:  &recordingMessageService{},
		memoryRegistry:  registry,
		settingsService: settings.NewService(slog.New(slog.DiscardHandler), &storeRoundSettingsQueries{}, nil, nil),
		logger:          slog.New(slog.DiscardHandler),
	}

	req := conversation.ChatRequest{
		BotID:     storeRoundBotID,
		SessionID: "session-1",
		Query:     "hello",
	}
	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		req,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages:   []sdk.Message{sdk.AssistantMessage("pong")},
			visibleOutput: true,
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}
	select {
	case <-memory.afterChat:
	case <-time.After(time.Second):
		t.Fatal("expected ordinary terminal snapshot to write memory")
	}

	req.SkipMemoryExtraction = true
	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		req,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages:   []sdk.Message{sdk.AssistantMessage("done")},
			visibleOutput: true,
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error with skip memory: %v", err)
	}
	select {
	case got := <-memory.afterChat:
		t.Fatalf("expected skip memory extraction to suppress memory write, got %#v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestPersistTerminalSnapshotSkillActivationWithoutPromptDoesNotStoreModelMarker(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	activation := &conversation.SkillActivation{
		Skills: []conversation.SkillActivationSkill{{Name: "alpha", DisplayName: "Alpha", State: "effective"}},
	}
	req := conversation.ChatRequest{
		BotID:                "bot-1",
		SessionID:            "session-1",
		ModelQuery:           conversation.SkillActivationModelQuery(activation),
		UserMessageKind:      conversation.UserMessageKindSkillActivation,
		SkillActivation:      activation,
		SkipMemoryExtraction: true,
	}

	if err := resolver.persistTerminalSnapshot(
		context.Background(),
		req,
		resolvedContext{},
		terminalSnapshot{
			sdkMessages:   []sdk.Message{sdk.AssistantMessage("done")},
			visibleOutput: true,
		},
	); err != nil {
		t.Fatalf("persistTerminalSnapshot returned error: %v", err)
	}

	if len(messages.persisted) != 2 {
		t.Fatalf("persisted messages = %d, want user + assistant", len(messages.persisted))
	}
	user := messages.persisted[0]
	if user.Role != "user" {
		t.Fatalf("first persisted role = %q, want user", user.Role)
	}
	if got := persistedTextContent(t, user.Content); got != "" {
		t.Fatalf("persisted user content = %q, want empty", got)
	}
	if user.DisplayText != "" {
		t.Fatalf("display text = %q, want empty", user.DisplayText)
	}
	if user.Metadata["user_message_kind"] != conversation.UserMessageKindSkillActivation {
		t.Fatalf("metadata kind = %#v, want skill_activation", user.Metadata["user_message_kind"])
	}
}
