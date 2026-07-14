package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/messagesource"
)

type failingBatchMessageService struct {
	recordingMessageService
	batchInputs []messagepkg.PersistInput
}

func TestWithoutInjectionCapabilitiesPreservesRequestData(t *testing.T) {
	t.Parallel()

	source := make(chan conversation.InjectMessage)
	req := conversation.ChatRequest{
		BotID: "bot-1",
		InjectionFeed: conversation.InjectionFeed{
			Messages: source,
			CommitPersisted: func(string) bool {
				return true
			},
		},
	}
	clean := withoutInjectionCapabilities(req)
	if clean.BotID != req.BotID || clean.InjectionFeed.Messages != nil ||
		clean.InjectionFeed.CommitPersisted != nil {
		t.Fatalf("sanitized request = %#v", clean)
	}

	runtime := withoutInjectionRuntime(resolvedContext{
		injectionReceipts: newInjectionReceiptRegistry(),
		injectionBridge:   &injectionBridge{},
		runConfig: agentpkg.RunConfig{
			InjectCh:         make(chan agentpkg.InjectMessage),
			InjectedRecorder: func(agentpkg.InjectedReceipt) {},
		},
	})
	if runtime.injectionReceipts != nil || runtime.injectionBridge != nil || runtime.runConfig.InjectCh != nil ||
		runtime.runConfig.InjectedRecorder != nil {
		t.Fatalf("sanitized runtime = %#v", runtime)
	}
}

func (s *failingBatchMessageService) PersistToolTailRound(_ context.Context, inputs []messagepkg.PersistInput) ([]messagepkg.Message, bool, error) {
	s.batchInputs = append(s.batchInputs, inputs...)
	return nil, true, errors.New("batch failed")
}

func TestInjectionBridgeCancellationUnblocksDeliveryAndJoins(t *testing.T) {
	t.Parallel()

	source := make(chan conversation.InjectMessage)
	resolver := &Resolver{logger: slog.New(slog.DiscardHandler)}
	runConfig, _, bridge := resolver.startInjectionBridge(context.Background(), conversation.ChatRequest{
		InjectionFeed: conversation.InjectionFeed{Messages: source},
	}, agentpkg.RunConfig{})
	if bridge == nil || runConfig.InjectCh == nil {
		t.Fatal("injection bridge was not created")
	}

	sourceDelivered := make(chan struct{})
	go func() {
		source <- conversation.InjectMessage{Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"}}
		close(sourceDelivered)
	}()
	select {
	case <-sourceDelivered:
	case <-time.After(time.Second):
		t.Fatal("bridge did not receive source message")
	}

	joined := make(chan struct{})
	var wait sync.WaitGroup
	for i := 0; i < 20; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			bridge.Close()
		}()
	}
	go func() {
		wait.Wait()
		close(joined)
	}()
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("bridge cancellation did not join blocked delivery")
	}
	if _, open := <-runConfig.InjectCh; open {
		t.Fatal("agent injection channel remained open after bridge join")
	}
}

func TestStoreMessagesCommitsOnlyInjectedReceiptAfterSequentialPersist(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	committed := make([]string, 0, 1)
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	leading := &conversation.UserMessageReceipt{ID: "receipt-leading", DisplayText: "leading"}
	leadingCopy := *leading
	injected := &conversation.UserMessageReceipt{ID: "receipt-injected", DisplayText: "injected"}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID, UserReceipt: leading,
		InjectionFeed: conversation.InjectionFeed{CommitPersisted: func(receiptID string) bool {
			if len(messages.persisted) == 0 {
				t.Fatal("commit callback ran before Persist returned")
			}
			committed = append(committed, receiptID)
			return true
		}},
	}, []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("leading"), UserReceipt: &leadingCopy},
		{Role: "user", Content: conversation.NewTextContent("injected"), UserReceipt: injected},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
	}, "", storeRoundOptions{})

	if len(committed) != 1 || committed[0] != injected.ID {
		t.Fatalf("committed receipts = %#v", committed)
	}
	if len(messages.persisted) != 3 {
		t.Fatalf("persist inputs = %#v", messages.persisted)
	}
}

func TestStoreMessagesDoesNotCommitInjectionReceiptAfterPersistFailure(t *testing.T) {
	t.Parallel()

	messages := &failingRecordingMessageService{}
	committed := make([]string, 0, 1)
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID,
		InjectionFeed: conversation.InjectionFeed{CommitPersisted: func(receiptID string) bool {
			committed = append(committed, receiptID)
			return true
		}},
	}, []conversation.ModelMessage{{
		Role: "user", Content: conversation.NewTextContent("injected"),
		UserReceipt: &conversation.UserMessageReceipt{ID: "receipt-failed", DisplayText: "injected"},
	}}, "", storeRoundOptions{})

	if len(committed) != 0 {
		t.Fatalf("failed persist committed receipts = %#v", committed)
	}
}

func TestStoreMessagesCommitsInjectionReceiptAfterAtomicBatch(t *testing.T) {
	t.Parallel()

	messages := &batchRecordingMessageService{}
	committed := make([]string, 0, 1)
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	receipt := &conversation.UserMessageReceipt{ID: "receipt-batch", DisplayText: "injected"}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID, SessionID: "33333333-3333-3333-3333-333333333333",
		SessionType: "chat", RuntimeType: "model",
		InjectionFeed: conversation.InjectionFeed{CommitPersisted: func(receiptID string) bool {
			if len(messages.batchInputs) != 4 {
				t.Fatal("commit callback ran before atomic batch returned")
			}
			committed = append(committed, receiptID)
			return true
		}},
	}, injectionToolTailMessages(receipt), "", storeRoundOptions{})

	if len(committed) != 1 || committed[0] != receipt.ID {
		t.Fatalf("batch committed receipts = %#v", committed)
	}
}

func TestStoreMessagesDoesNotCommitInjectionReceiptAfterAtomicBatchFailure(t *testing.T) {
	t.Parallel()

	messages := &failingBatchMessageService{}
	committed := make([]string, 0, 1)
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	receipt := &conversation.UserMessageReceipt{ID: "receipt-batch-failed", DisplayText: "injected"}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID, SessionID: "33333333-3333-3333-3333-333333333333",
		SessionType: "chat", RuntimeType: "model",
		InjectionFeed: conversation.InjectionFeed{CommitPersisted: func(receiptID string) bool {
			committed = append(committed, receiptID)
			return true
		}},
	}, injectionToolTailMessages(receipt), "", storeRoundOptions{})

	if len(committed) != 0 || len(messages.persisted) != 0 {
		t.Fatalf("failed batch commits=%#v sequential=%d", committed, len(messages.persisted))
	}
}

func TestStoreMessagesCommitsInjectionReceiptOnceAfterBatchDeclines(t *testing.T) {
	t.Parallel()

	messages := &decliningBatchMessageService{}
	committed := make([]string, 0, 1)
	resolver := &Resolver{messageService: messages, logger: slog.New(slog.DiscardHandler)}
	receipt := &conversation.UserMessageReceipt{ID: "receipt-batch-declined", DisplayText: "injected"}
	resolver.storeMessages(context.Background(), conversation.ChatRequest{
		BotID: storeRoundBotID, SessionID: "33333333-3333-3333-3333-333333333333",
		SessionType: "chat", RuntimeType: "model",
		InjectionFeed: conversation.InjectionFeed{CommitPersisted: func(receiptID string) bool {
			committed = append(committed, receiptID)
			return true
		}},
	}, injectionToolTailMessages(receipt), "", storeRoundOptions{})

	if len(messages.persisted) != 4 || len(committed) != 1 || committed[0] != receipt.ID {
		t.Fatalf("declined batch sequential=%d commits=%#v", len(messages.persisted), committed)
	}
}

func injectionToolTailMessages(receipt *conversation.UserMessageReceipt) []conversation.ModelMessage {
	return []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("injected"), UserReceipt: receipt},
		{Role: "assistant", Content: conversation.NewTextContent("call")},
		{Role: "tool", Content: conversation.NewTextContent("result")},
		{Role: "assistant", Content: conversation.NewTextContent("done")},
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
				ID: "receipt-a",
				Origin: mustInjectionEnvelope(t, messagesource.EnvelopeInput{
					SenderChannelIdentityID: "11111111-1111-1111-1111-111111111111",
					ExternalMessageID:       "external-a",
				}),
			},
			InsertAfter: 1,
		},
		{
			ModelText: "same text",
			Receipt: conversation.UserMessageReceipt{
				ID: "receipt-b",
				Origin: mustInjectionEnvelope(t, messagesource.EnvelopeInput{
					SenderChannelIdentityID: "22222222-2222-2222-2222-222222222222",
					ExternalMessageID:       "external-b",
				}),
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
		if message.UserReceipt == nil {
			t.Fatalf("injected message %d receipt is nil", i)
		}
		gotOrigin := message.UserReceipt.Origin.Values()
		wantOrigin := want.Receipt.Origin.Values()
		if message.TextContent() != "same text" {
			t.Fatalf("injected message %d text = %q", i, message.TextContent())
		}
		if message.UserReceipt.ID != want.Receipt.ID ||
			gotOrigin.SenderChannelIdentityID != wantOrigin.SenderChannelIdentityID ||
			gotOrigin.ExternalMessageID != wantOrigin.ExternalMessageID {
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
			ID: "receipt-a",
			Origin: mustInjectionEnvelope(t, messagesource.EnvelopeInput{
				SenderChannelIdentityID: "11111111-1111-1111-1111-111111111111",
				ExternalMessageID:       "external-a",
			}),
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
			ID: "receipt-b",
			Origin: mustInjectionEnvelope(t, messagesource.EnvelopeInput{
				SenderChannelIdentityID: "22222222-2222-2222-2222-222222222222",
				ExternalMessageID:       "external-b",
			}),
		},
	})
	if err != nil {
		t.Fatalf("admit second receipt: %v", err)
	}
	firstAttachmentNested := first.Attachments[0].Metadata["nested"].(map[string]any)
	firstReply := first.Metadata["reply"].(map[string]any)
	firstItems := first.Metadata["items"].([]any)
	firstRawSnapshot, err := json.Marshal(first.Metadata["raw"])
	if err != nil {
		t.Fatalf("marshal raw metadata snapshot: %v", err)
	}
	var firstRawValue map[string]string
	if err := json.Unmarshal(firstRawSnapshot, &firstRawValue); err != nil {
		t.Fatalf("decode raw metadata snapshot: %v", err)
	}
	if first.DisplayText != "same text" || first.Attachments[0].ContentHash != "asset-a" ||
		firstAttachmentNested["key"] != "value-a" || firstReply["message_id"] != "reply-a" ||
		firstItems[0] != "item-a" || firstRawValue["value"] != "raw-a" ||
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
	firstRecordedOrigin := records[0].Receipt.Origin.Values()
	secondRecordedOrigin := records[1].Receipt.Origin.Values()
	if firstRecordedOrigin.SenderChannelIdentityID != "22222222-2222-2222-2222-222222222222" ||
		firstRecordedOrigin.ExternalMessageID != "external-b" ||
		records[0].Receipt.Attachments[0].ContentHash != "asset-b" ||
		secondRecordedOrigin.SenderChannelIdentityID != "11111111-1111-1111-1111-111111111111" ||
		secondRecordedOrigin.ExternalMessageID != "external-a" ||
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

func mustInjectionEnvelope(t *testing.T, input messagesource.EnvelopeInput) messagesource.Envelope {
	t.Helper()
	envelope, err := messagesource.NewEnvelope(input)
	if err != nil {
		t.Fatalf("new source envelope: %v", err)
	}
	return envelope
}
