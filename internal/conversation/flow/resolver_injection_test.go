package flow

import (
	"encoding/json"
	"errors"
	"testing"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
)

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
