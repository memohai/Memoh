package flow

import (
	"context"
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
)

type orderedReplacementMessageService struct {
	recordingMessageService
	replacementMessageIDs []string
}

func (s *orderedReplacementMessageService) ReplaceTurnOrdered(_ context.Context, _, _, _, _ string, messageIDs []string, _ string) (messagepkg.HistoryTurn, error) {
	s.replaced++
	s.replacementMessageIDs = append([]string(nil), messageIDs...)
	return messagepkg.HistoryTurn{}, nil
}

func TestReplacePersistedTurnPassesExactPersistedOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		requestMessageID string
		persisted        []messagepkg.Message
		want             []string
	}{
		{
			name:             "retry prepends existing request",
			requestMessageID: "request-old",
			persisted: []messagepkg.Message{
				{ID: "assistant-tool-call", Role: "assistant"},
				{ID: "tool-result", Role: "tool"},
				{ID: "assistant-final", Role: "assistant"},
			},
			want: []string{"request-old", "assistant-tool-call", "tool-result", "assistant-final"},
		},
		{
			name:             "edit keeps mid-turn user decoration",
			requestMessageID: "request-new",
			persisted: []messagepkg.Message{
				{ID: "request-new", Role: "user"},
				{ID: "assistant-before-decoration", Role: "assistant"},
				{ID: "user-decoration", Role: "user"},
				{ID: "assistant-final", Role: "assistant"},
			},
			want: []string{"request-new", "assistant-before-decoration", "user-decoration", "assistant-final"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			messages := &orderedReplacementMessageService{}
			resolver := &Resolver{
				messageService: messages,
				logger:         slog.New(slog.DiscardHandler),
			}
			if err := resolver.replacePersistedTurn(
				context.Background(),
				conversation.ChatRequest{SessionID: "session-1"},
				"turn-1",
				tt.requestMessageID,
				"retry",
				tt.persisted,
			); err != nil {
				t.Fatalf("replacePersistedTurn() error = %v", err)
			}
			assertReplacementMessageIDs(t, messages.replacementMessageIDs, tt.want)
		})
	}
}

func TestReplacePersistedTurnRejectsInvalidOrderedBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		persisted []messagepkg.Message
	}{
		{
			name: "duplicate message id",
			persisted: []messagepkg.Message{
				{ID: "assistant-1", Role: "assistant"},
				{ID: "assistant-1", Role: "assistant"},
			},
		},
		{
			name:      "missing assistant anchor",
			persisted: []messagepkg.Message{{ID: "tool-1", Role: "tool"}},
		},
		{
			name: "blank message id",
			persisted: []messagepkg.Message{
				{ID: "assistant-1", Role: "assistant"},
				{Role: "tool"},
			},
		},
		{
			name: "request anchor is not first",
			persisted: []messagepkg.Message{
				{ID: "assistant-1", Role: "assistant"},
				{ID: "request-1", Role: "user"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			messages := &orderedReplacementMessageService{}
			resolver := &Resolver{
				messageService: messages,
				logger:         slog.New(slog.DiscardHandler),
			}
			err := resolver.replacePersistedTurn(
				context.Background(),
				conversation.ChatRequest{SessionID: "session-1"},
				"turn-1",
				"request-1",
				"retry",
				tt.persisted,
			)
			if err == nil {
				t.Fatal("replacePersistedTurn() error = nil, want invalid batch error")
			}
			if messages.replaced != 0 {
				t.Fatalf("ReplaceTurnOrdered called %d times, want 0", messages.replaced)
			}
		})
	}
}

func TestReplacePersistedTurnFailsClosedWithoutOrderedCapability(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	resolver := &Resolver{
		messageService: messages,
		logger:         slog.New(slog.DiscardHandler),
	}
	err := resolver.replacePersistedTurn(
		context.Background(),
		conversation.ChatRequest{SessionID: "session-1"},
		"turn-1",
		"request-1",
		"retry",
		[]messagepkg.Message{
			{ID: "assistant-1", Role: "assistant"},
			{ID: "tool-1", Role: "tool"},
		},
	)
	if err == nil {
		t.Fatal("replacePersistedTurn() error = nil, want missing ordered capability error")
	}
	if messages.replaced != 0 {
		t.Fatalf("ReplaceTurn called %d times, want 0", messages.replaced)
	}
}

func assertReplacementMessageIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("replacement message ids = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("replacement message ids = %#v, want %#v", got, want)
		}
	}
}
