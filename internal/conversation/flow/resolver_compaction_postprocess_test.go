package flow

import (
	"reflect"
	"testing"

	"github.com/memohai/memoh/internal/contextfrag"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
)

func TestStripToolMessagesWhenCompactionSummaryIsActive(t *testing.T) {
	messages := []conversation.ModelMessage{
		{Role: "user", Content: conversation.NewTextContent("question")},
		{Role: "tool", Content: conversation.NewTextContent("large tool output")},
	}

	t.Run("raw history keeps tool messages", func(t *testing.T) {
		records := []historyfrag.HistoryRecord{{
			Kind:       contextfrag.KindConversationEvent,
			SourceKind: historyfrag.SourceDBMessage,
		}}
		got := stripToolMessagesWhenCompactionSummaryIsActive(messages, records)
		if !reflect.DeepEqual(got, messages) {
			t.Fatalf("raw history messages = %#v, want %#v", got, messages)
		}
	})

	t.Run("active summary strips tool messages", func(t *testing.T) {
		records := []historyfrag.HistoryRecord{{
			Kind:       contextfrag.KindConversationSummary,
			SourceKind: historyfrag.SourceCompactionLog,
			Lifecycle:  historyfrag.LifecycleActiveSummary,
		}}
		got := stripToolMessagesWhenCompactionSummaryIsActive(messages, records)
		if len(got) != 1 || got[0].Role != "user" {
			t.Fatalf("summarized history messages = %#v, want only user message", got)
		}
	})
}
