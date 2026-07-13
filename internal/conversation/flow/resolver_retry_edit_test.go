package flow

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/session"
)

type forkAnchorMessageService struct {
	recordingMessageService
	visibleFrom []messagepkg.Message
	before      []messagepkg.Message
}

type replacementPreparationMessageService struct {
	recordingMessageService
	messages map[string]messagepkg.Message
	turn     messagepkg.HistoryTurn
	latest   *messagepkg.HistoryTurn
}

func (s *replacementPreparationMessageService) GetByIDBySession(_ context.Context, _ string, messageID string) (messagepkg.Message, error) {
	message, ok := s.messages[messageID]
	if !ok {
		return messagepkg.Message{}, errors.New("message not found")
	}
	return message, nil
}

func (s *replacementPreparationMessageService) GetVisibleTurnByMessage(context.Context, string, string) (messagepkg.HistoryTurn, error) {
	return s.turn, nil
}

func (s *replacementPreparationMessageService) GetLatestVisibleTurnBySession(context.Context, string) (messagepkg.HistoryTurn, error) {
	if s.latest != nil {
		return *s.latest, nil
	}
	return s.turn, nil
}

func TestAdmitPreparedReplacementWSRejectsConcurrentLatestTurnChange(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"assistant-old": {ID: "assistant-old", Role: "assistant"},
			"user-request":  {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-request", AssistantMessageID: "assistant-old"},
	}
	resolver := &Resolver{messageService: messages}
	prepared, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID: "bot-1", SessionID: "session-1", StreamID: "stream-retry", MessageID: "assistant-old",
	})
	if err != nil {
		t.Fatalf("prepare retry: %v", err)
	}
	messages.latest = &messagepkg.HistoryTurn{ID: "turn-new"}

	if _, release, err := resolver.AdmitPreparedReplacementWS(context.Background(), prepared); err == nil {
		release()
		t.Fatal("stale prepared replacement was admitted")
	}
	if resolver.SessionTurnActive("bot-1", "session-1") {
		t.Fatal("failed replacement admission leaked the session-turn lock")
	}
}

func TestAdmitPreparedReplacementWSRejectsBusySessionWithoutBlocking(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"assistant-old": {ID: "assistant-old", Role: "assistant"},
			"user-request":  {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-request", AssistantMessageID: "assistant-old"},
	}
	resolver := &Resolver{messageService: messages}
	prepared, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID: "bot-1", SessionID: "session-1", StreamID: "stream-retry", MessageID: "assistant-old",
	})
	if err != nil {
		t.Fatalf("prepare retry: %v", err)
	}
	release, admitted := resolver.tryEnterIdleSessionTurn(context.Background(), "bot-1", "session-1")
	if !admitted {
		t.Fatal("failed to reserve active test turn")
	}
	defer release()

	done := make(chan error, 1)
	go func() {
		_, rejectedRelease, err := resolver.AdmitPreparedReplacementWS(context.Background(), prepared)
		rejectedRelease()
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "active turn") {
			t.Fatalf("busy admission error = %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("busy replacement admission blocked")
	}
}

func TestValidatePreparedReplacementWSRejectsChangeAfterLocalAdmission(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"assistant-old": {ID: "assistant-old", Role: "assistant"},
			"user-request":  {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-request", AssistantMessageID: "assistant-old"},
	}
	resolver := &Resolver{messageService: messages}
	prepared, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID: "bot-1", SessionID: "session-1", StreamID: "stream-retry", MessageID: "assistant-old",
	})
	if err != nil {
		t.Fatalf("prepare retry: %v", err)
	}
	prepared, release, err := resolver.AdmitPreparedReplacementWS(context.Background(), prepared)
	if err != nil {
		t.Fatalf("local admission: %v", err)
	}
	defer release()
	messages.latest = &messagepkg.HistoryTurn{ID: "turn-new"}
	if err := resolver.ValidatePreparedReplacementWS(context.Background(), prepared); err == nil {
		t.Fatal("post-reservation validation accepted stale replacement")
	}
}

func TestStreamPreparedReplacementWSAppliesPersistedTurnReplacement(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"assistant-old": {ID: "assistant-old", Role: "assistant"},
			"user-request":  {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-request", AssistantMessageID: "assistant-old"},
	}
	resolver := &Resolver{messageService: messages, logger: slog.Default()}
	prepared, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID: "bot-1", SessionID: "session-1", StreamID: "stream-retry", MessageID: "assistant-old",
	})
	if err != nil {
		t.Fatalf("prepare retry: %v", err)
	}
	prepared, release, err := resolver.AdmitPreparedReplacementWS(context.Background(), prepared)
	if err != nil {
		t.Fatalf("admit retry: %v", err)
	}
	defer release()
	resolver.streamReplacementFn = func(
		ctx context.Context,
		_ conversation.ChatRequest,
		_ chan<- WSStreamEvent,
		_ <-chan struct{},
		preflight func(context.Context) error,
		postPersist func(context.Context, []messagepkg.Message) error,
		_ bool,
	) ([]messagepkg.Message, error) {
		if preflight != nil {
			t.Fatal("admitted replacement unexpectedly installed a second preflight")
		}
		persisted := []messagepkg.Message{{ID: "assistant-new", BotID: "bot-1", SessionID: "session-1", Role: "assistant"}}
		return persisted, postPersist(ctx, persisted)
	}

	if err := resolver.StreamPreparedReplacementWS(context.Background(), prepared, make(chan WSStreamEvent, 1), make(chan struct{})); err != nil {
		t.Fatalf("stream prepared retry: %v", err)
	}
	if messages.replaced != 1 {
		t.Fatalf("replacement writes = %d, want 1", messages.replaced)
	}
}

func TestPrepareRetryLatestMessageWSUsesCanonicalAssistantTurnAnchor(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"assistant-fragment": {ID: "assistant-fragment", Role: "assistant"},
			"user-request":       {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{
			ID:                 "turn-old",
			RequestMessageID:   "user-request",
			AssistantMessageID: "assistant-first",
		},
	}
	resolver := &Resolver{messageService: messages}
	prepared, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		StreamID:  "stream-retry",
		MessageID: "assistant-fragment",
	})
	if err != nil {
		t.Fatalf("prepare retry: %v", err)
	}
	if prepared.Operation.Kind != "retry" || prepared.Operation.ReplaceFromMessageID != "assistant-first" {
		t.Fatalf("operation = %#v", prepared.Operation)
	}
	if prepared.OldTurnID != "turn-old" || prepared.RequestMessageID != "user-request" {
		t.Fatalf("prepared replacement = %#v", prepared)
	}
}

func TestPrepareEditLatestMessageWSBuildsCanonicalReplacementUserTurn(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"user-request": {ID: "user-request", Role: "user"},
		},
		turn: messagepkg.HistoryTurn{ID: "turn-old", RequestMessageID: "user-request"},
	}
	resolver := &Resolver{messageService: messages}
	prepared, err := resolver.PrepareEditLatestMessageWS(context.Background(), EditLatestMessageInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		StreamID:  "stream-edit",
		MessageID: "user-request",
		Text:      "edited prompt",
		Attachments: []conversation.ChatAttachment{{
			Type:        "image",
			ContentHash: "sha256:image",
			Name:        "diagram.png",
		}},
	})
	if err != nil {
		t.Fatalf("prepare edit: %v", err)
	}
	if prepared.Operation.Kind != "edit" || prepared.Operation.ReplaceFromMessageID != "user-request" {
		t.Fatalf("operation = %#v", prepared.Operation)
	}
	turn := prepared.Operation.ReplacementUserTurn
	if turn == nil || turn.Role != "user" || turn.Text != "edited prompt" || turn.Platform != "local" {
		t.Fatalf("replacement user turn = %#v", turn)
	}
	if len(turn.Attachments) != 1 || turn.Attachments[0].ContentHash != "sha256:image" {
		t.Fatalf("replacement attachments = %#v", turn.Attachments)
	}
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

type recordingEventPublisher struct {
	events []messageevent.Event
}

func (p *recordingEventPublisher) Publish(event messageevent.Event) {
	p.events = append(p.events, event)
}

func TestReplacePersistedTurnPublishesReplacementMessageEvent(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	events := &recordingEventPublisher{}
	resolver := &Resolver{
		messageService: messages,
		eventPublisher: events,
		logger:         slog.Default(),
	}

	err := resolver.replacePersistedTurn(context.Background(), conversation.ChatRequest{
		BotID:     "bot-1",
		SessionID: "session-1",
	}, "old-turn", "user-new", "retry", []messagepkg.Message{
		{ID: "user-new", BotID: "bot-1", SessionID: "session-1", Role: "user"},
		{ID: "assistant-new", BotID: "bot-1", SessionID: "session-1", Role: "assistant", CreatedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("replace persisted turn: %v", err)
	}
	if len(events.events) != 1 {
		t.Fatalf("published events = %d, want 1", len(events.events))
	}
	event := events.events[0]
	if event.Type != messageevent.EventTypeMessageCreated {
		t.Fatalf("event type = %q, want %q", event.Type, messageevent.EventTypeMessageCreated)
	}
	if event.BotID != "bot-1" {
		t.Fatalf("event bot id = %q, want bot-1", event.BotID)
	}
	var payload messagepkg.Message
	if err := json.Unmarshal(event.Data, &payload); err != nil {
		t.Fatalf("unmarshal event payload: %v", err)
	}
	if payload.ID != "assistant-new" || payload.SessionID != "session-1" {
		t.Fatalf("payload = %#v, want assistant-new in session-1", payload)
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
