package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/memohai/memoh/internal/agent"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/models"
	"github.com/memohai/memoh/internal/settings"
)

func TestStreamChatReportsIdleTimeoutOnlyThroughErrorChannel(t *testing.T) {
	resolver, modelID := newIdleTimeoutTestResolver(t)

	chunks, errs := resolver.StreamChat(context.Background(), idleTimeoutChatRequest(modelID))
	for chunk := range chunks {
		var event agentpkg.StreamEvent
		if err := json.Unmarshal(chunk, &event); err != nil {
			t.Fatalf("decode stream event: %v", err)
		}
		if event.Type == agentpkg.EventError {
			t.Fatalf("idle timeout was also published as a stream event: %#v", event)
		}
	}
	if err := <-errs; err == nil || !strings.Contains(err.Error(), "stream timeout") {
		t.Fatalf("StreamChat() error = %v, want idle timeout", err)
	}
}

func TestStreamChatWSReportsIdleTimeoutOnlyThroughReturnError(t *testing.T) {
	resolver, modelID := newIdleTimeoutTestResolver(t)
	eventCh := make(chan WSStreamEvent, 16)

	err := resolver.StreamChatWS(context.Background(), idleTimeoutChatRequest(modelID), eventCh, nil)
	close(eventCh)
	for data := range eventCh {
		var event agentpkg.StreamEvent
		if decodeErr := json.Unmarshal(data, &event); decodeErr != nil {
			t.Fatalf("decode WS event: %v", decodeErr)
		}
		if event.Type == agentpkg.EventError {
			t.Fatalf("idle timeout was also published as a WS event: %#v", event)
		}
	}
	if err == nil || !strings.Contains(err.Error(), "stream timeout") {
		t.Fatalf("StreamChatWS() error = %v, want idle timeout", err)
	}
}

func newIdleTimeoutTestResolver(t *testing.T) (*Resolver, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-req.Context().Done()
	}))
	t.Cleanup(server.Close)

	provider := modelSelectionProviderRow(t, "00000000-0000-0000-0000-000000000741", string(models.ClientTypeOpenAICompletions), true)
	provider.Config = []byte(fmt.Sprintf(`{"api_key":"test-key","base_url":%q}`, server.URL))
	model := modelSelectionModelRow(
		t,
		"00000000-0000-0000-0000-000000000742",
		"idle-timeout-model",
		provider.ID,
		models.ModelTypeChat,
		true,
	)
	queries := &modelStreamQueries{modelSelectionFakeQueries: &modelSelectionFakeQueries{
		models:   map[string]sqlc.Model{model.ModelID: model},
		provider: provider,
	}}
	logger := slog.New(slog.DiscardHandler)
	resolver := NewResolver(
		logger,
		models.NewService(logger, queries),
		queries,
		nil,
		&modelStreamMessageService{recordingMessageService: &recordingMessageService{}},
		settings.NewService(logger, queries, nil, nil),
		nil,
		agentpkg.New(agentpkg.Deps{}),
		time.UTC,
		time.Second,
	)
	resolver.idleTimeoutOptions = &idleTimeoutOptions{baseTimeout: 5 * time.Millisecond}
	return resolver, model.ModelID
}

func idleTimeoutChatRequest(modelID string) conversation.ChatRequest {
	return conversation.ChatRequest{
		BotID:                storeRoundBotID,
		ChatID:               "chat-1",
		SessionID:            "33333333-3333-3333-3333-333333333333",
		Query:                "wait forever",
		Model:                modelID,
		SessionType:          "chat",
		UserMessagePersisted: true,
		SkipTitleGeneration:  true,
	}
}
