package flow

import (
	"reflect"
	"testing"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

func TestDedupePersistedCurrentUserMessageUsesHistoryRecordProvenance(t *testing.T) {
	t.Parallel()

	history := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{
			Role:    "user",
			Content: conversation.NewTextContent("---\nmessage-id: qq-msg-1\nchannel: qq\n---\nhello"),
		}, func(record *historyfrag.HistoryRecord) {
			record.ExternalMessageID = "qq-msg-1"
			record.Platform = "qq"
			record.SenderChannelIdentityID = "channel-identity-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{
			Role:    "assistant",
			Content: conversation.NewTextContent("ok"),
		}, nil),
	}

	got := dedupePersistedCurrentUserMessage(history, conversation.ChatRequest{
		UserMessagePersisted:    true,
		RouteID:                 "route-1",
		ExternalMessageID:       "qq-msg-1",
		CurrentChannel:          "qq",
		SourceChannelIdentityID: "channel-identity-1",
	})

	if len(got) != 1 {
		t.Fatalf("expected 1 message after dedupe, got %d", len(got))
	}
	if got[0].ModelMessage.Role != "assistant" {
		t.Fatalf("unexpected remaining role: %s", got[0].ModelMessage.Role)
	}
}

func TestReplaceCompactedHistoryRecordsUsesLegacySummaryRecord(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old 1")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old 2")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
		historyRecord("row-3", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("new")}, nil),
	}

	got := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": "condensed"})
	wantMessages := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("<summary>\ncondensed\n</summary>")},
		{Role: "user", Content: conversation.NewTextContent("new")},
	}
	if gotMessages := historyfrag.ToModelMessages(got); !reflect.DeepEqual(gotMessages, wantMessages) {
		t.Fatalf("replacement messages mismatch:\ngot  %#v\nwant %#v", gotMessages, wantMessages)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	summary := got[0]
	if summary.SourceKind != historyfrag.SourceCompactionLog || summary.Lifecycle != historyfrag.LifecycleLegacySummary {
		t.Fatalf("summary record source/lifecycle mismatch: %#v", summary)
	}
	if summary.Kind != contextfrag.KindConversationEvent {
		t.Fatalf("summary should remain conversation_event in PR1, got %s", summary.Kind)
	}
	if summary.Ref.Namespace != "compaction_log" || summary.Ref.ID != "compact-1" || summary.Ref.Durability != contextfrag.RefDurable {
		t.Fatalf("summary ref should be durable compaction log identity: %#v", summary.Ref)
	}
	if frag := historyfrag.ToFrag(summary); frag.Kind != contextfrag.KindConversationEvent || frag.Slot != contextfrag.SlotHistory {
		t.Fatalf("summary frag should stay history conversation event in PR1: %#v", frag)
	}
}

func TestReplaceCompactedHistoryRecordsKeepsOriginalGroupWithoutSummary(t *testing.T) {
	t.Parallel()

	records := []historyfrag.HistoryRecord{
		historyRecord("row-1", conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("old 1")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
		historyRecord("row-2", conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("old 2")}, func(record *historyfrag.HistoryRecord) {
			record.CompactID = "compact-1"
		}),
	}

	gotMissing := replaceCompactedHistoryRecords(records, map[string]string{})
	if gotMessages := historyfrag.ToModelMessages(gotMissing); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("missing summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
	}

	gotEmpty := replaceCompactedHistoryRecords(records, map[string]string{"compact-1": ""})
	if gotMessages := historyfrag.ToModelMessages(gotEmpty); !reflect.DeepEqual(gotMessages, historyfrag.ToModelMessages(records)) {
		t.Fatalf("empty summary should keep original group:\ngot  %#v\nwant %#v", gotMessages, historyfrag.ToModelMessages(records))
	}
}

func TestHistoryScopeFallbackFromChatRequestUsesRequestTopology(t *testing.T) {
	t.Parallel()

	got := historyScopeFallbackFromChatRequest(conversation.ChatRequest{
		ChatID:           " chat-1 ",
		ConversationType: " group ",
		ConversationName: " Dev Chat ",
		ReplyTarget:      " target-1 ",
	})

	if got.ChatID != "chat-1" ||
		got.ConversationType != "group" ||
		got.ConversationName != "Dev Chat" ||
		got.ReplyTarget != "target-1" {
		t.Fatalf("unexpected fallback: %#v", got)
	}
}

func TestResumeHistoryFallbackDoesNotUseBotIDAsChatID(t *testing.T) {
	t.Parallel()

	userInputFallback := historyScopeFallbackFromUserInputRequest(userinput.Request{
		BotID:            "bot-1",
		ConversationType: "group",
		ReplyTarget:      "target-1",
	})
	if userInputFallback.ChatID != "" {
		t.Fatalf("user input fallback ChatID = %q, want empty", userInputFallback.ChatID)
	}
	if userInputFallback.ConversationType != "group" || userInputFallback.ReplyTarget != "target-1" {
		t.Fatalf("user input fallback lost topology: %#v", userInputFallback)
	}

	approvalFallback := historyScopeFallbackFromToolApprovalRequest(toolapproval.Request{
		BotID:            "bot-1",
		ConversationType: "direct",
		ReplyTarget:      "target-2",
	})
	if approvalFallback.ChatID != "" {
		t.Fatalf("approval fallback ChatID = %q, want empty", approvalFallback.ChatID)
	}
	if approvalFallback.ConversationType != "direct" || approvalFallback.ReplyTarget != "target-2" {
		t.Fatalf("approval fallback lost topology: %#v", approvalFallback)
	}
}

func historyRecord(id string, msg conversation.ModelMessage, mutate func(*historyfrag.HistoryRecord)) historyfrag.HistoryRecord {
	record := historyfrag.HistoryRecord{
		Ref: contextfrag.ContextRef{
			Namespace:  "bot_history_message",
			ID:         id,
			Version:    1,
			Schema:     contextfrag.SchemaContextRef,
			Durability: contextfrag.RefDurable,
		},
		Kind:         contextfrag.KindConversationEvent,
		SourceKind:   historyfrag.SourceDBMessage,
		Lifecycle:    historyfrag.LifecyclePersisted,
		ModelMessage: msg,
		DBMessageID:  id,
	}
	if mutate != nil {
		mutate(&record)
	}
	return record
}
