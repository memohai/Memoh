package flow

import (
	"context"
	"errors"
	"log/slog"
	"strings"
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

func TestStreamPreparedReplacementWSRequiresAtomicPersistence(t *testing.T) {
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
		_ bool,
	) ([]messagepkg.Message, error) {
		if preflight != nil {
			t.Fatal("admitted replacement unexpectedly installed a second preflight")
		}
		state := replacementPersistenceFromContext(ctx)
		if state == nil {
			t.Fatal("replacement persistence state missing from stream context")
		}
		state.atomicCommitted = true
		persisted := []messagepkg.Message{{ID: "assistant-new", BotID: "bot-1", SessionID: "session-1", Role: "assistant"}}
		return persisted, nil
	}

	if err := resolver.StreamPreparedReplacementWS(context.Background(), prepared, make(chan WSStreamEvent, 1), make(chan struct{})); err != nil {
		t.Fatalf("stream prepared retry: %v", err)
	}
}

func TestStreamPreparedReplacementWSKeepsOriginalWhenNothingWasPersisted(t *testing.T) {
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
		BotID: "bot-1", SessionID: "session-1", StreamID: "stream-retry-empty", MessageID: "assistant-old",
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
		_ context.Context,
		_ conversation.ChatRequest,
		_ chan<- WSStreamEvent,
		_ <-chan struct{},
		_ func(context.Context) error,
		_ bool,
	) ([]messagepkg.Message, error) {
		return nil, nil
	}

	if err := resolver.StreamPreparedReplacementWS(context.Background(), prepared, make(chan WSStreamEvent, 1), make(chan struct{})); err != nil {
		t.Fatalf("empty replacement stream: %v", err)
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
	if prepared.Request.ExternalMessageID != "stream-retry" {
		t.Fatalf("retry external message id = %q, want stream-retry", prepared.Request.ExternalMessageID)
	}
}

func TestPrepareRetryLatestMessageWSAcceptsInterruptedUserOnlyTurn(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"user-request": {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{
			ID:               "turn-interrupted",
			RequestMessageID: "user-request",
		},
	}
	resolver := &Resolver{messageService: messages}
	prepared, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		StreamID:  "stream-retry",
		MessageID: "user-request",
	})
	if err != nil {
		t.Fatalf("prepare user-only retry: %v", err)
	}
	if prepared.OldTurnID != "turn-interrupted" || prepared.RequestMessageID != "user-request" {
		t.Fatalf("prepared replacement = %#v", prepared)
	}
	if prepared.Operation.Kind != "retry" || prepared.Operation.ReplaceFromMessageID != "user-request" {
		t.Fatalf("operation = %#v", prepared.Operation)
	}
}

func TestPrepareRetryLatestMessageWSRejectsUserTargetWhenTurnHasAssistant(t *testing.T) {
	t.Parallel()

	messages := &replacementPreparationMessageService{
		messages: map[string]messagepkg.Message{
			"user-request": {ID: "user-request", Role: "user", DisplayContent: "original prompt"},
		},
		turn: messagepkg.HistoryTurn{
			ID:                 "turn-completed",
			RequestMessageID:   "user-request",
			AssistantMessageID: "assistant-response",
		},
	}
	resolver := &Resolver{messageService: messages}
	if _, err := resolver.PrepareRetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID:     "bot-1",
		SessionID: "session-1",
		StreamID:  "stream-retry",
		MessageID: "user-request",
	}); err == nil {
		t.Fatal("user target for a turn with an assistant was accepted")
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
	if prepared.Request.ExternalMessageID != "stream-edit" {
		t.Fatalf("edit external message id = %q, want stream-edit", prepared.Request.ExternalMessageID)
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

func TestPrepareForkAnchorUpdateMovesForkAnchorMetadata(t *testing.T) {
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
		},
		logger: slog.New(slog.DiscardHandler),
	}

	update, err := resolver.prepareForkAnchorUpdate(context.Background(), "fork-session", "assistant-old")
	if err != nil {
		t.Fatalf("prepareForkAnchorUpdate() error = %v", err)
	}
	if update == nil {
		t.Fatal("expected fork anchor metadata update")
	}

	fork, ok := update.metadata["forked_from"].(map[string]any)
	if !ok {
		t.Fatalf("updated fork metadata missing: %#v", update.metadata)
	}
	if got := fork["fork_message_id"]; got != "assistant-prev" {
		t.Fatalf("fork_message_id = %#v, want assistant-prev", got)
	}
	if got := fork["source_extra_key"]; got != "kept" {
		t.Fatalf("source_extra_key = %#v, want kept", got)
	}
}

func TestPrepareForkAnchorUpdateClearsAnchorWhenNoInheritedAssistantRemains(t *testing.T) {
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
		},
		logger: slog.New(slog.DiscardHandler),
	}

	update, err := resolver.prepareForkAnchorUpdate(context.Background(), "fork-session", "assistant-old")
	if err != nil {
		t.Fatalf("prepareForkAnchorUpdate() error = %v", err)
	}
	if update == nil {
		t.Fatal("expected fork anchor metadata update")
	}

	fork, ok := update.metadata["forked_from"].(map[string]any)
	if !ok {
		t.Fatalf("updated fork metadata missing: %#v", update.metadata)
	}
	if _, ok := fork["fork_message_id"]; ok {
		t.Fatalf("fork_message_id was not cleared: %#v", fork)
	}
}
