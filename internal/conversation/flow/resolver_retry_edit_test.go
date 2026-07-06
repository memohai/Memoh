package flow

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
)

type forkAnchorMessageService struct {
	recordingMessageService
	visibleFrom []messagepkg.Message
	before      []messagepkg.Message
}

func (s *forkAnchorMessageService) ListVisibleFromBySession(context.Context, string, string) ([]messagepkg.Message, error) {
	return append([]messagepkg.Message(nil), s.visibleFrom...), nil
}

func (s *forkAnchorMessageService) ListBeforeMessageBySession(context.Context, string, string, int32) ([]messagepkg.Message, error) {
	return append([]messagepkg.Message(nil), s.before...), nil
}

func TestReplacePersistedTurnMovesForkAnchorMetadata(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	messages := &forkAnchorMessageService{
		visibleFrom: []messagepkg.Message{
			{ID: "assistant-old", Role: "assistant", CreatedAt: createdAt.Add(-time.Minute)},
		},
		before: []messagepkg.Message{
			{ID: "user-1", Role: "user", CreatedAt: createdAt.Add(-4 * time.Minute)},
			{ID: "assistant-prev", Role: "assistant", CreatedAt: createdAt.Add(-3 * time.Minute)},
			{ID: "user-2", Role: "user", CreatedAt: createdAt.Add(-2 * time.Minute)},
		},
	}
	var updated map[string]any
	resolver := &Resolver{
		messageService: messages,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(context.Context, string) (session.Session, error) {
				return session.Session{
					ID:        "fork-session",
					CreatedAt: createdAt,
					Metadata: map[string]any{
						"forked_from": map[string]any{
							"session_id":       "source-session",
							"message_id":       "source-assistant",
							"fork_message_id":  "assistant-old",
							"source_extra_key": "kept",
						},
					},
				}, nil
			},
			updateMetadataFn: func(_ context.Context, _ string, metadata map[string]any) (session.Session, error) {
				updated = metadata
				return session.Session{Metadata: metadata}, nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	if err := resolver.replacePersistedTurn(
		context.Background(),
		conversation.ChatRequest{SessionID: "fork-session", HistoryCutoffBeforeMessageID: "assistant-old"},
		"old-turn",
		"request-2",
		"retry",
		[]messagepkg.Message{{ID: "assistant-new", Role: "assistant"}},
	); err != nil {
		t.Fatalf("replacePersistedTurn() error = %v", err)
	}

	fork, ok := updated["forked_from"].(map[string]any)
	if !ok {
		t.Fatalf("updated fork metadata missing: %#v", updated)
	}
	if got := fork["fork_message_id"]; got != "assistant-prev" {
		t.Fatalf("fork_message_id = %#v, want assistant-prev", got)
	}
	if got := fork["source_extra_key"]; got != "kept" {
		t.Fatalf("source_extra_key = %#v, want kept", got)
	}
}

func TestReplacePersistedTurnClearsForkAnchorWhenNoInheritedAssistantRemains(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	messages := &forkAnchorMessageService{
		visibleFrom: []messagepkg.Message{
			{ID: "assistant-old", Role: "assistant", CreatedAt: createdAt.Add(-time.Minute)},
		},
		before: []messagepkg.Message{
			{ID: "user-1", Role: "user", CreatedAt: createdAt.Add(-2 * time.Minute)},
		},
	}
	var updated map[string]any
	resolver := &Resolver{
		messageService: messages,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(context.Context, string) (session.Session, error) {
				return session.Session{
					ID:        "fork-session",
					CreatedAt: createdAt,
					Metadata: map[string]any{
						"forked_from": map[string]any{
							"session_id":      "source-session",
							"message_id":      "source-assistant",
							"fork_message_id": "assistant-old",
						},
					},
				}, nil
			},
			updateMetadataFn: func(_ context.Context, _ string, metadata map[string]any) (session.Session, error) {
				updated = metadata
				return session.Session{Metadata: metadata}, nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	if err := resolver.replacePersistedTurn(
		context.Background(),
		conversation.ChatRequest{SessionID: "fork-session", HistoryCutoffBeforeMessageID: "assistant-old"},
		"old-turn",
		"request-1",
		"retry",
		[]messagepkg.Message{{ID: "assistant-new", Role: "assistant"}},
	); err != nil {
		t.Fatalf("replacePersistedTurn() error = %v", err)
	}

	fork, ok := updated["forked_from"].(map[string]any)
	if !ok {
		t.Fatalf("updated fork metadata missing: %#v", updated)
	}
	if _, ok := fork["fork_message_id"]; ok {
		t.Fatalf("fork_message_id was not cleared: %#v", fork)
	}
}
