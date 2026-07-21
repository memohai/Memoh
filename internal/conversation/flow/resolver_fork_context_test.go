package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
)

func TestHistorySourceMessageIDsFollowRetainedDatabaseMessages(t *testing.T) {
	records := []historyfrag.HistoryRecord{
		{
			DBMessageID: "00000000-0000-0000-0000-000000000001",
			ModelMessage: conversation.ModelMessage{
				Role:    "user",
				Content: conversation.NewTextContent("first"),
			},
		},
		{
			DBMessageID: "00000000-0000-0000-0000-000000000002",
			ModelMessage: conversation.ModelMessage{
				Role:    "assistant",
				Content: conversation.NewTextContent("second"),
			},
		},
	}
	messages := []conversation.ModelMessage{
		{Role: "system", Content: conversation.NewTextContent("runtime workspace context")},
		records[1].ModelMessage,
		{Role: "user", Content: conversation.NewTextContent("current query")},
	}

	got := historySourceMessageIDsForMessages(messages, records)
	want := []string{"", records[1].DBMessageID, ""}
	if len(got) != len(want) {
		t.Fatalf("source IDs = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("source ID %d = %q, want %q", i, got[i], want[i])
		}
	}
}
