package flow

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestLoadMemoryContextMessage_NoProvider(t *testing.T) {
	resolver := &Resolver{
		logger: slog.Default(),
	}
	msg := resolver.loadMemoryContextMessage(context.Background(), conversation.ChatRequest{
		Query:  "hello",
		BotID:  "bot-1",
		ChatID: "chat-1",
	})
	if msg != nil {
		t.Fatalf("expected nil message when no memory provider is configured")
	}
}
