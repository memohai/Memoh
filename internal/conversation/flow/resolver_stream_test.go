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
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/settings"
)

func TestResolvedContextCompactionPressureUsesFinalRawReceipt(t *testing.T) {
	t.Parallel()

	state := &initialPromptState{}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation: contextbudget.Allocation{
			CompactableTokens: 0,
		},
		TotalTokens: 12000,
	}, nil)
	rc := resolvedContext{
		compactableTokens:      9000,
		compactableTokensKnown: true,
		promptState:            state,
	}

	if got, known := rc.compactionPressure(); !known || got != 0 {
		t.Fatalf("compactionPressure() = %d known=%v, want summary-only final raw pressure 0/true", got, known)
	}
	state.Store(initialPromptResult{
		AccountingReady: true,
		Allocation: contextbudget.Allocation{
			CompactableTokens: 91,
		},
	}, nil)
	if got, known := rc.compactionPressure(); !known || got != 91 {
		t.Fatalf("compactionPressure() = %d known=%v, want raw pressure 91/true", got, known)
	}
}

func TestResolvedContextCompactionPressureFallsBackBeforeAccounting(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("tool schema failed")
	state := &initialPromptState{}
	state.Store(initialPromptResult{}, sentinel)
	rc := resolvedContext{
		compactableTokens:      23,
		compactableTokensKnown: true,
		promptState:            state,
	}

	if got, known := rc.compactionPressure(); !known || got != 23 {
		t.Fatalf("compactionPressure() = %d known=%v, want pre-materialization raw pressure 23/true", got, known)
	}
	if !errors.Is(rc.promptMaterializationError(), sentinel) {
		t.Fatalf("promptMaterializationError() = %v, want %v", rc.promptMaterializationError(), sentinel)
	}
}

func TestResolvedContextCompactionPressureRemainsUnknownWithoutReceipt(t *testing.T) {
	t.Parallel()

	if got, known := (resolvedContext{}).compactionPressure(); known || got != 0 {
		t.Fatalf("compactionPressure() = %d known=%v, want unknown zero", got, known)
	}
}

func TestFinishStreamPostPersistPrioritizesMaterializationError(t *testing.T) {
	t.Parallel()

	overflow := &PromptEnvelopeOverflowError{ContextBudget: 10, TotalTokens: 11}
	state := &initialPromptState{}
	state.Store(initialPromptResult{}, overflow)
	rc := resolvedContext{promptState: state}
	postPersistCalls := 0

	err := finishStreamPostPersist(context.Background(), rc, nil, false, func(context.Context, []messagepkg.Message) error {
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

func TestReplacePersistedTurnErrorsWhenNoReplacementPersisted(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}

	err := resolver.replacePersistedTurn(
		context.Background(),
		conversation.ChatRequest{SessionID: "session-1"},
		"turn-1",
		"request-1",
		"retry",
		nil,
	)
	if err == nil {
		t.Fatal("expected replacement error, got nil")
	}
	if messages.replaced != 0 {
		t.Fatalf("ReplaceTurn called %d times, want 0", messages.replaced)
	}
}

func TestInterleaveInjectedMessagesKeepsSameTextReceiptsDistinct(t *testing.T) {
	t.Parallel()

	round := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("initial")},
		{Role: "assistant", Content: conversation.NewTextContent("working")},
	}
	injections := []conversation.InjectedMessageRecord{
		{
			ModelText: "same text",
			Receipt: conversation.UserMessageReceipt{
				ID:                      "receipt-a",
				SenderChannelIdentityID: "identity-a",
				ExternalMessageID:       "external-a",
			},
			InsertAfter: 1,
		},
		{
			ModelText: "same text",
			Receipt: conversation.UserMessageReceipt{
				ID:                      "receipt-b",
				SenderChannelIdentityID: "identity-b",
				ExternalMessageID:       "external-b",
			},
			InsertAfter: 1,
		},
	}

	got := interleaveInjectedMessages(round, injections)
	if len(got) != 4 {
		t.Fatalf("interleaved messages = %d, want 4", len(got))
	}
	for i, want := range injections {
		message := got[i+2]
		if message.TextContent() != "same text" {
			t.Fatalf("injected message %d text = %q", i, message.TextContent())
		}
		if message.UserReceipt == nil ||
			message.UserReceipt.ID != want.Receipt.ID ||
			message.UserReceipt.SenderChannelIdentityID != want.Receipt.SenderChannelIdentityID ||
			message.UserReceipt.ExternalMessageID != want.Receipt.ExternalMessageID {
			t.Fatalf("injected message %d receipt = %#v, want %#v", i, message.UserReceipt, want.Receipt)
		}
	}
}

func TestInjectionReceiptRegistryCorrelatesOutOfOrderSameTextReceipts(t *testing.T) {
	t.Parallel()

	registry := newInjectionReceiptRegistry()
	firstRaw := json.RawMessage(`{"value":"raw-a"}`)
	firstInput := conversation.InjectMessage{
		Text: "same text",
		Attachments: []conversation.ChatAttachment{{ContentHash: "asset-a", Metadata: map[string]any{
			"nested": map[string]any{"key": "value-a"},
		}}},
		Receipt: conversation.UserMessageReceipt{
			ID:                      "receipt-a",
			SenderChannelIdentityID: "identity-a",
			ExternalMessageID:       "external-a",
			Metadata: map[string]any{
				"reply": map[string]any{"message_id": "reply-a"},
				"items": []any{"item-a"},
				"raw":   firstRaw,
			},
		},
	}
	firstID, first, err := registry.admit(firstInput)
	if err != nil {
		t.Fatalf("admit first receipt: %v", err)
	}
	firstInput.Text = "mutated"
	firstInput.Attachments[0].ContentHash = "mutated"
	firstInput.Attachments[0].Metadata["nested"].(map[string]any)["key"] = "mutated"
	firstInput.Receipt.Metadata["reply"].(map[string]any)["message_id"] = "mutated"
	firstInput.Receipt.Metadata["items"].([]any)[0] = "mutated"
	firstRaw[10] = 'X'
	secondID, second, err := registry.admit(conversation.InjectMessage{
		Text:        "same text",
		Attachments: []conversation.ChatAttachment{{ContentHash: "asset-b"}},
		Receipt: conversation.UserMessageReceipt{
			ID:                      "receipt-b",
			SenderChannelIdentityID: "identity-b",
			ExternalMessageID:       "external-b",
		},
	})
	if err != nil {
		t.Fatalf("admit second receipt: %v", err)
	}
	firstAttachmentNested := first.Attachments[0].Metadata["nested"].(map[string]any)
	firstReply := first.Metadata["reply"].(map[string]any)
	firstItems := first.Metadata["items"].([]any)
	firstRawSnapshot := first.Metadata["raw"].(map[string]any)
	if first.DisplayText != "same text" || first.Attachments[0].ContentHash != "asset-a" ||
		firstAttachmentNested["key"] != "value-a" || firstReply["message_id"] != "reply-a" ||
		firstItems[0] != "item-a" || firstRawSnapshot["value"] != "raw-a" ||
		second.DisplayText != "same text" || second.Attachments[0].ContentHash != "asset-b" {
		t.Fatalf("admission snapshot mismatch: first=%#v second=%#v", first, second)
	}

	if !registry.record(agentpkg.InjectedReceipt{ID: secondID, ModelText: "same text", InsertAfter: 2}) ||
		!registry.record(agentpkg.InjectedReceipt{ID: firstID, ModelText: "same text", InsertAfter: 3}) {
		t.Fatal("known receipt was rejected")
	}
	records := registry.recordsSnapshot()
	if len(records) != 2 {
		t.Fatalf("recorded receipts = %d, want 2", len(records))
	}
	if records[0].Receipt.SenderChannelIdentityID != "identity-b" ||
		records[0].Receipt.ExternalMessageID != "external-b" ||
		records[0].Receipt.Attachments[0].ContentHash != "asset-b" ||
		records[1].Receipt.SenderChannelIdentityID != "identity-a" ||
		records[1].Receipt.ExternalMessageID != "external-a" ||
		records[1].Receipt.Attachments[0].ContentHash != "asset-a" {
		t.Fatalf("out-of-order correlation swapped receipts: %#v", records)
	}

	unknown := agentpkg.InjectedReceipt{ID: "unknown", ModelText: "same text", InsertAfter: 4}
	if registry.record(unknown) || registry.record(agentpkg.InjectedReceipt{ID: firstID, ModelText: "same text", InsertAfter: 5}) {
		t.Fatal("unknown or duplicate receipt was accepted")
	}
	if _, _, err := registry.admit(firstInput); !errors.Is(err, errDuplicateInjectionReceipt) {
		t.Fatal("duplicate admission was accepted")
	}
	if _, _, err := registry.admit(conversation.InjectMessage{
		Receipt: conversation.UserMessageReceipt{Metadata: map[string]any{"invalid": func() {}}},
	}); err == nil {
		t.Fatal("uncloneable metadata was accepted")
	}
	if got := len(registry.recordsSnapshot()); got != 2 {
		t.Fatalf("fail-closed recorder appended %d receipts", got)
	}
}
