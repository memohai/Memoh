package flow

import (
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/historyfrag"
)

func TestMemoryQueryBuilderCombinesRecentUserMessages(t *testing.T) {
	t.Parallel()

	builder := memoryQueryBuilder{MaxRecentMessages: 2, MaxBytes: 2000, MaxLines: 20, MaxMessageRunes: 200}
	query := builder.Build(conversation.ChatRequest{Query: "What should I pack?"}, []historyfrag.HistoryRecord{
		{ModelMessage: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("I am visiting Berlin next week")}},
		{ModelMessage: conversation.ModelMessage{Role: "assistant", Content: conversation.NewTextContent("Sounds fun.")}},
		{ModelMessage: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("I prefer tea over coffee")}},
		{ModelMessage: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("What should I pack?")}},
	})

	if query.Source != "current_query_with_history" {
		t.Fatalf("Source = %q, want current_query_with_history", query.Source)
	}
	if query.RecentMessages != 2 {
		t.Fatalf("RecentMessages = %d, want 2", query.RecentMessages)
	}
	for _, want := range []string{"Current user request:", "What should I pack?", "I am visiting Berlin", "I prefer tea"} {
		if !strings.Contains(query.Query, want) {
			t.Fatalf("query missing %q: %s", want, query.Query)
		}
	}
	if strings.Count(query.Query, "What should I pack?") != 1 {
		t.Fatalf("expected current query to be deduplicated, got: %s", query.Query)
	}
}

func TestMemoryQueryBuilderTruncates(t *testing.T) {
	t.Parallel()

	builder := memoryQueryBuilder{MaxRecentMessages: 1, MaxBytes: 120, MaxLines: 6, MaxMessageRunes: 500}
	query := builder.Build(conversation.ChatRequest{Query: strings.Repeat("current ", 30)}, []historyfrag.HistoryRecord{
		{ModelMessage: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent(strings.Repeat("history ", 30))}},
	})

	if !query.Truncated {
		t.Fatal("expected query to be marked truncated")
	}
	if len(query.Query) > 120 {
		t.Fatalf("query length = %d, want <= 120: %q", len(query.Query), query.Query)
	}
}

func TestMemoryQueryBuilderSkipsEmptyVisibleQuery(t *testing.T) {
	t.Parallel()

	query := defaultMemoryQueryBuilder().Build(conversation.ChatRequest{Query: "   "}, []historyfrag.HistoryRecord{
		{ModelMessage: conversation.ModelMessage{Role: "user", Content: conversation.NewTextContent("history")}},
	})
	if query.Query != "" {
		t.Fatalf("expected empty query, got %+v", query)
	}
}
