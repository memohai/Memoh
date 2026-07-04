package flow

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	memprovider "github.com/memohai/memoh/internal/memory/adapters"
	"github.com/memohai/memoh/internal/settings"
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

func TestLoadMemoryContextMessageSkipsEmptyQuery(t *testing.T) {
	t.Parallel()

	registry := memprovider.NewRegistry(slog.New(slog.DiscardHandler))
	registry.Register(storeRoundMemoryProviderID, &storeRoundMemoryProvider{afterChat: make(chan memprovider.AfterChatRequest, 1)})
	resolver := &Resolver{
		memoryRegistry:  registry,
		settingsService: settings.NewService(slog.New(slog.DiscardHandler), &storeRoundSettingsQueries{}, nil, nil),
		logger:          slog.New(slog.DiscardHandler),
	}
	msg := resolver.loadMemoryContextMessage(context.Background(), conversation.ChatRequest{
		Query:      "",
		ModelQuery: "The user activated the following skill for this turn without an additional prompt: alpha.",
		BotID:      storeRoundBotID,
		ChatID:     "chat-1",
	})
	if msg != nil {
		t.Fatalf("expected nil message for empty visible query, got %#v", msg)
	}
}
