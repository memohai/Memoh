package flow

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type blockingMessageService struct {
	persistCalled   chan struct{}
	persistContinue chan struct{}
}

func (s *blockingMessageService) Persist(ctx context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	select {
	case <-s.persistCalled:
	default:
		close(s.persistCalled)
	}
	<-s.persistContinue
	return messagepkg.Message{}, nil
}

func (s *blockingMessageService) List(ctx context.Context, botID string) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *blockingMessageService) ListSince(ctx context.Context, botID string, since time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *blockingMessageService) ListActiveSince(ctx context.Context, botID string, since time.Time) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *blockingMessageService) ListLatest(ctx context.Context, botID string, limit int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *blockingMessageService) ListBefore(ctx context.Context, botID string, before time.Time, limit int32) ([]messagepkg.Message, error) {
	return nil, nil
}

func (s *blockingMessageService) DeleteByBot(ctx context.Context, botID string) error {
	return nil
}

func TestStreamChat_PersistsFinalMessagesBeforeForwardingDoneEvent(t *testing.T) {
	t.Parallel()

	msgSvc := &blockingMessageService{
		persistCalled:   make(chan struct{}),
		persistContinue: make(chan struct{}),
	}

	doneResp := gatewayResponse{
		Messages: []conversation.ModelMessage{
			{Role: "assistant", Content: conversation.NewTextContent("ok")},
		},
	}
	doneData, err := json.Marshal(doneResp)
	if err != nil {
		t.Fatalf("marshal done response: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/stream" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write([]byte("event: done\n"))
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(doneData)
		_, _ = w.Write([]byte("\n\n"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)

	r := &Resolver{
		messageService:  msgSvc,
		gatewayBaseURL:  srv.URL,
		logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		streamingClient: srv.Client(),
		httpClient:      srv.Client(),
	}

	chunkCh := make(chan conversation.StreamChunk, 10)
	req := conversation.ChatRequest{BotID: "bot-test", ChatID: "chat-test"}
	payload := gatewayRequest{}

	streamDone := make(chan error, 1)
	go func() {
		streamDone <- r.streamChat(context.Background(), payload, req, chunkCh)
		close(chunkCh)
	}()

	select {
	case <-msgSvc.persistCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Persist to be called")
	}

	select {
	case got := <-chunkCh:
		t.Fatalf("done event forwarded before persistence finished: %s", string(got))
	default:
	}

	close(msgSvc.persistContinue)

	select {
	case err := <-streamDone:
		if err != nil {
			t.Fatalf("streamChat returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for streamChat to finish")
	}

	select {
	case got := <-chunkCh:
		if len(got) == 0 {
			t.Fatal("expected forwarded done event data")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for forwarded done event data")
	}
}
