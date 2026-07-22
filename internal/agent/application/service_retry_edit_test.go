package application

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	acpclient "github.com/memohai/memoh/internal/agent/runtime/acp/client"
	messageevent "github.com/memohai/memoh/internal/chat/event"
	messagepkg "github.com/memohai/memoh/internal/chat/message"
	session "github.com/memohai/memoh/internal/chat/thread"
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
	resolver := &Service{
		messageService: messages,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(context.Context, string) (session.Thread, error) {
				return session.Thread{
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
			updateMetadataFn: func(_ context.Context, _ string, metadata map[string]any) (session.Thread, error) {
				updated = metadata
				return session.Thread{Metadata: metadata}, nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	if err := resolver.replacePersistedTurn(
		context.Background(),
		ChatRequest{ThreadID: "fork-session", HistoryCutoffBeforeMessageID: "assistant-old"},
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

type acpReplacementMessageService struct {
	recordingMessageService
	messages    map[string]messagepkg.Message
	turn        messagepkg.HistoryTurn
	replacement []string
}

func (s *acpReplacementMessageService) Persist(_ context.Context, input messagepkg.PersistInput) (messagepkg.Message, error) {
	s.persisted = append(s.persisted, input)
	id := input.Role + "-new"
	return messagepkg.Message{
		ID:             id,
		BotID:          input.BotID,
		SessionID:      input.SessionID,
		Role:           input.Role,
		Content:        input.Content,
		Metadata:       input.Metadata,
		DisplayContent: input.DisplayText,
	}, nil
}

func (s *acpReplacementMessageService) GetByIDBySession(_ context.Context, _, messageID string) (messagepkg.Message, error) {
	return s.messages[messageID], nil
}

func (s *acpReplacementMessageService) GetVisibleTurnByMessage(context.Context, string, string) (messagepkg.HistoryTurn, error) {
	return s.turn, nil
}

func (s *acpReplacementMessageService) GetLatestVisibleTurnBySession(context.Context, string) (messagepkg.HistoryTurn, error) {
	return s.turn, nil
}

func (s *acpReplacementMessageService) ReplaceTurn(_ context.Context, sessionID, oldTurnID, requestMessageID, assistantMessageID, reason string) (messagepkg.HistoryTurn, error) {
	s.replacement = []string{sessionID, oldTurnID, requestMessageID, assistantMessageID, reason}
	return messagepkg.HistoryTurn{}, nil
}

func newACPReplacementResolver(messages *acpReplacementMessageService) *Service {
	return &Service{
		messageService: messages,
		acpPool: &recordingACPPrompter{result: withTranscriptOutput(acpclient.PromptResult{
			Text:       "new answer",
			StopReason: "end_turn",
		})},
		botPermissions: allowWorkspaceExecFor("user-1"),
		sessionService: acpRuntimeSessionServiceForTest("user-1"),
		logger:         slog.New(slog.DiscardHandler),
	}
}

func TestACPRetryReusesUserAndReplacesLatestTurn(t *testing.T) {
	t.Parallel()

	messages := &acpReplacementMessageService{
		messages: map[string]messagepkg.Message{
			"user-old":      {ID: "user-old", Role: "user", Content: newTextContent("original")},
			"assistant-old": {ID: "assistant-old", Role: "assistant"},
		},
		turn: messagepkg.HistoryTurn{
			ID:                 "turn-old",
			RequestMessageID:   "user-old",
			AssistantMessageID: "assistant-old",
		},
	}
	resolver := newACPReplacementResolver(messages)

	err := resolver.RetryLatestMessageWS(context.Background(), RetryLatestMessageInput{
		BotID:                  "bot-1",
		SessionID:              "session-1",
		StreamID:               "stream-1",
		MessageID:              "assistant-old",
		ActorChannelIdentityID: "identity-1",
		ActorUserID:            "user-1",
	}, make(chan WSStreamEvent, 16), make(chan struct{}))
	if err != nil {
		t.Fatalf("RetryLatestMessageWS() error = %v", err)
	}
	if len(messages.persisted) != 1 || messages.persisted[0].Role != "assistant" {
		t.Fatalf("persisted inputs = %#v, want replacement assistant only", messages.persisted)
	}
	if !messages.persisted[0].SkipHistoryTurn {
		t.Fatal("retry replacement became visible before replacing the old turn")
	}
	wantReplacement := []string{"session-1", "turn-old", "user-old", "assistant-new", "retry"}
	if !equalStrings(messages.replacement, wantReplacement) {
		t.Fatalf("replacement = %#v, want %#v", messages.replacement, wantReplacement)
	}
	pool := resolver.acpPool.(*recordingACPPrompter)
	if !strings.Contains(pool.input.ContextMarkdown, "## Turn Replacement") ||
		!strings.Contains(pool.input.ContextMarkdown, "fresh answer") {
		t.Fatalf("retry context = %q, want turn replacement notice", pool.input.ContextMarkdown)
	}
	if !strings.Contains(pool.input.Prompt, "original") || strings.Contains(pool.input.Prompt, "retracted") {
		t.Fatalf("retry prompt = %q, want unmodified user text", pool.input.Prompt)
	}
}

func TestACPEditPersistsHiddenUserAndReplacesLatestTurn(t *testing.T) {
	t.Parallel()

	messages := &acpReplacementMessageService{
		messages: map[string]messagepkg.Message{
			"user-old": {ID: "user-old", Role: "user", Content: newTextContent("original")},
		},
		turn: messagepkg.HistoryTurn{
			ID:                 "turn-old",
			RequestMessageID:   "user-old",
			AssistantMessageID: "assistant-old",
		},
	}
	resolver := newACPReplacementResolver(messages)

	err := resolver.EditLatestMessageWS(context.Background(), EditLatestMessageInput{
		BotID:                  "bot-1",
		SessionID:              "session-1",
		StreamID:               "stream-1",
		MessageID:              "user-old",
		Text:                   "edited",
		ActorChannelIdentityID: "identity-1",
		ActorUserID:            "user-1",
	}, make(chan WSStreamEvent, 16), make(chan struct{}))
	if err != nil {
		t.Fatalf("EditLatestMessageWS() error = %v", err)
	}
	if len(messages.persisted) != 2 || messages.persisted[0].Role != "user" || messages.persisted[1].Role != "assistant" {
		t.Fatalf("persisted inputs = %#v, want hidden user + assistant", messages.persisted)
	}
	for _, input := range messages.persisted {
		if !input.SkipHistoryTurn {
			t.Fatalf("%s replacement became visible before replacing the old turn", input.Role)
		}
	}
	wantReplacement := []string{"session-1", "turn-old", "user-new", "assistant-new", "edit"}
	if !equalStrings(messages.replacement, wantReplacement) {
		t.Fatalf("replacement = %#v, want %#v", messages.replacement, wantReplacement)
	}
	pool := resolver.acpPool.(*recordingACPPrompter)
	if !strings.Contains(pool.input.ContextMarkdown, "## Turn Replacement") ||
		!strings.Contains(pool.input.ContextMarkdown, "revised") {
		t.Fatalf("edit context = %q, want turn replacement notice", pool.input.ContextMarkdown)
	}
	if pool.input.Prompt != "edited" {
		t.Fatalf("edit prompt = %q, want unmodified user text", pool.input.Prompt)
	}
}

func (p *recordingEventPublisher) Publish(event messageevent.Event) {
	p.events = append(p.events, event)
}

func TestReplacePersistedTurnPublishesReplacementMessageEvent(t *testing.T) {
	t.Parallel()

	messages := &recordingMessageService{}
	events := &recordingEventPublisher{}
	resolver := &Service{
		messageService: messages,
		eventPublisher: events,
		logger:         slog.Default(),
	}

	err := resolver.replacePersistedTurn(context.Background(), ChatRequest{
		BotID:    "bot-1",
		ThreadID: "session-1",
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
	resolver := &Service{
		messageService: messages,
		sessionService: &fakeBackgroundSessionService{
			getFn: func(context.Context, string) (session.Thread, error) {
				return session.Thread{
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
			updateMetadataFn: func(_ context.Context, _ string, metadata map[string]any) (session.Thread, error) {
				updated = metadata
				return session.Thread{Metadata: metadata}, nil
			},
		},
		logger: slog.New(slog.DiscardHandler),
	}

	if err := resolver.replacePersistedTurn(
		context.Background(),
		ChatRequest{ThreadID: "fork-session", HistoryCutoffBeforeMessageID: "assistant-old"},
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
