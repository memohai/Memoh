package flow

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/memory"
)

func TestLoadMemoryContextMessage_NoMemoryService(t *testing.T) {
	resolver := &Resolver{
		memoryService: nil,
		logger:        slog.Default(),
	}
	msg := resolver.loadMemoryContextMessage(context.Background(), conversation.ChatRequest{
		Query:  "hello",
		BotID:  "bot-1",
		ChatID: "chat-1",
	})
	if msg != nil {
		t.Fatalf("expected nil message when memory service is nil")
	}
}

func TestLoadMemoryContextMessage_SearchFailureFallback(t *testing.T) {
	resolver := &Resolver{
		memoryService: &memory.Service{},
		logger:        slog.Default(),
	}
	msg := resolver.loadMemoryContextMessage(context.Background(), conversation.ChatRequest{
		Query:  "hello",
		BotID:  "bot-1",
		ChatID: "chat-1",
		UserID: "user-1",
	})
	if msg != nil {
		t.Fatalf("expected nil message when memory search cannot return results")
	}
}

func TestTruncateMemorySnippet(t *testing.T) {
	longText := strings.Repeat("a", 20) + "  "
	got := truncateMemorySnippet(longText, 10)
	if got != strings.Repeat("a", 10)+"..." {
		t.Fatalf("unexpected truncated value: %q", got)
	}

	shortText := "  short  "
	got = truncateMemorySnippet(shortText, 10)
	if got != "short" {
		t.Fatalf("unexpected trimmed short value: %q", got)
	}
}
