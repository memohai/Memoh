package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
	messagepkg "github.com/memohai/memoh/internal/message"
	"github.com/memohai/memoh/internal/session"
)

type fakeHistorySessionLister struct {
	sessions []session.Session
}

func (f fakeHistorySessionLister) ListByBot(_ context.Context, _ string) ([]session.Session, error) {
	return f.sessions, nil
}

type fakeHistoryMessageReader struct {
	latestSessionID     string
	beforeSessionID     string
	latestHeadSessionID string
	beforeHeadSessionID string
	latestHeadTurnID    string
	beforeHeadTurnID    string
	before              time.Time
	latestMessages      []messagepkg.Message
	beforeMessages      []messagepkg.Message
}

func (f *fakeHistoryMessageReader) ListLatest(_ context.Context, _ string, _ int32) ([]messagepkg.Message, error) {
	return f.latestMessages, nil
}

func (f *fakeHistoryMessageReader) ListBefore(_ context.Context, _ string, before time.Time, _ string, _ int32) ([]messagepkg.Message, error) {
	f.before = before
	return f.beforeMessages, nil
}

func (f *fakeHistoryMessageReader) ListLatestBySession(_ context.Context, sessionID string, _ int32) ([]messagepkg.Message, error) {
	f.latestSessionID = sessionID
	return f.latestMessages, nil
}

func (f *fakeHistoryMessageReader) ListBeforeBySession(_ context.Context, sessionID string, before time.Time, _ string, _ int32) ([]messagepkg.Message, error) {
	f.beforeSessionID = sessionID
	f.before = before
	return f.beforeMessages, nil
}

func (f *fakeHistoryMessageReader) ListLatestBySessionHead(_ context.Context, sessionID string, headTurnID string, _ int32) ([]messagepkg.Message, error) {
	f.latestHeadSessionID = sessionID
	f.latestHeadTurnID = headTurnID
	return f.latestMessages, nil
}

func (f *fakeHistoryMessageReader) ListBeforeBySessionHead(_ context.Context, sessionID string, headTurnID string, before time.Time, _ string, _ int32) ([]messagepkg.Message, error) {
	f.beforeHeadSessionID = sessionID
	f.beforeHeadTurnID = headTurnID
	f.before = before
	return f.beforeMessages, nil
}

func TestHistoryProviderGetMessagesDefaultsToCurrentSession(t *testing.T) {
	t.Parallel()

	older := historyTestMessage(t, "msg-1", "session-current", "user", "hello", time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC))
	newer := historyTestMessage(t, "msg-2", "session-current", "assistant", "hi there", time.Date(2026, 6, 14, 9, 1, 0, 0, time.UTC))
	reader := &fakeHistoryMessageReader{latestMessages: []messagepkg.Message{newer, older}}
	provider := NewHistoryProvider(nil, nil, reader, nil)

	got, err := provider.execGetMessages(context.Background(), SessionContext{
		BotID:     "bot-1",
		SessionID: "session-current",
	}, map[string]any{"limit": 2})
	if err != nil {
		t.Fatalf("execGetMessages() error = %v", err)
	}
	if reader.latestSessionID != "session-current" {
		t.Fatalf("latest session id = %q, want session-current", reader.latestSessionID)
	}

	out := got.(map[string]any)
	messages := out["messages"].([]map[string]any)
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0]["id"] != "msg-1" || messages[0]["text"] != "hello" {
		t.Fatalf("first message = %#v, want oldest message text", messages[0])
	}
	if messages[1]["id"] != "msg-2" || messages[1]["text"] != "hi there" {
		t.Fatalf("second message = %#v, want newest message text", messages[1])
	}
}

func TestHistoryProviderGetMessagesUsesSelectedHeadForCurrentSession(t *testing.T) {
	t.Parallel()

	reader := &fakeHistoryMessageReader{latestMessages: []messagepkg.Message{
		historyTestMessage(t, "msg-selected", "session-current", "assistant", "selected head", time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)),
	}}
	provider := NewHistoryProvider(nil, nil, reader, nil)

	got, err := provider.execGetMessages(context.Background(), SessionContext{
		BotID:          "bot-1",
		SessionID:      "session-current",
		ViewHeadTurnID: "turn-selected",
	}, map[string]any{"limit": 1})
	if err != nil {
		t.Fatalf("execGetMessages() error = %v", err)
	}
	if reader.latestHeadSessionID != "session-current" || reader.latestHeadTurnID != "turn-selected" {
		t.Fatalf("selected head read = %q/%q, want session-current/turn-selected", reader.latestHeadSessionID, reader.latestHeadTurnID)
	}
	if reader.latestSessionID != "" {
		t.Fatalf("default-head read was used for session %q", reader.latestSessionID)
	}
	messages := got.(map[string]any)["messages"].([]map[string]any)
	if messages[0]["text"] != "selected head" {
		t.Fatalf("message text = %v, want selected head", messages[0]["text"])
	}
}

func TestHistoryProviderGetMessagesUsesDefaultHeadForOtherSession(t *testing.T) {
	t.Parallel()

	reader := &fakeHistoryMessageReader{latestMessages: []messagepkg.Message{
		historyTestMessage(t, "msg-other", "session-other", "assistant", "other default", time.Date(2026, 6, 14, 9, 0, 0, 0, time.UTC)),
	}}
	provider := NewHistoryProvider(nil, fakeHistorySessionLister{
		sessions: []session.Session{{ID: "session-other", BotID: "bot-1"}},
	}, reader, nil)

	_, err := provider.execGetMessages(context.Background(), SessionContext{
		BotID:          "bot-1",
		SessionID:      "session-current",
		ViewHeadTurnID: "turn-selected",
	}, map[string]any{
		"session_id": "session-other",
	})
	if err != nil {
		t.Fatalf("execGetMessages() error = %v", err)
	}
	if reader.latestSessionID != "session-other" {
		t.Fatalf("latest session id = %q, want session-other", reader.latestSessionID)
	}
	if reader.latestHeadTurnID != "" {
		t.Fatalf("selected head turn id = %q, want empty for other session", reader.latestHeadTurnID)
	}
}

func TestHistoryProviderGetMessagesBeforeUsesRequestedSession(t *testing.T) {
	t.Parallel()

	reader := &fakeHistoryMessageReader{
		beforeMessages: []messagepkg.Message{
			historyTestMessage(t, "msg-1", "session-other", "user", "before cursor", time.Date(2026, 6, 14, 8, 0, 0, 0, time.UTC)),
		},
	}
	provider := NewHistoryProvider(nil, fakeHistorySessionLister{
		sessions: []session.Session{{ID: "session-other", BotID: "bot-1"}},
	}, reader, nil)

	got, err := provider.execGetMessages(context.Background(), SessionContext{
		BotID:     "bot-1",
		SessionID: "session-current",
	}, map[string]any{
		"session_id": "session-other",
		"before":     "2026-06-14T09:00:00Z",
	})
	if err != nil {
		t.Fatalf("execGetMessages() error = %v", err)
	}
	if reader.beforeSessionID != "session-other" {
		t.Fatalf("before session id = %q, want session-other", reader.beforeSessionID)
	}
	if reader.before.IsZero() {
		t.Fatal("before cursor was not parsed")
	}

	out := got.(map[string]any)
	if out["session_id"] != "session-other" {
		t.Fatalf("session_id = %v, want session-other", out["session_id"])
	}
	messages := out["messages"].([]map[string]any)
	if messages[0]["text"] != "before cursor" {
		t.Fatalf("message text = %v, want before cursor", messages[0]["text"])
	}
}

func TestHistoryProviderGetMessagesRejectsSessionOutsideBot(t *testing.T) {
	t.Parallel()

	reader := &fakeHistoryMessageReader{}
	provider := NewHistoryProvider(nil, fakeHistorySessionLister{
		sessions: []session.Session{{ID: "session-current", BotID: "bot-1"}},
	}, reader, nil)

	_, err := provider.execGetMessages(context.Background(), SessionContext{
		BotID:     "bot-1",
		SessionID: "session-current",
	}, map[string]any{
		"session_id": "session-other",
	})
	if err == nil {
		t.Fatal("execGetMessages() error = nil, want session ownership error")
	}
}

func TestExtractTextContentSummarizesAssistantToolCalls(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{
		{"type": "reasoning", "text": "thinking"},
		{"type": "tool-call", "toolName": "read", "toolCallId": "call-1", "input": map[string]any{"path": "/tmp/a.txt"}},
		{"type": "tool-call", "toolName": "edit", "toolCallId": "call-2", "input": map[string]any{"path": "/tmp/a.txt"}},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	raw, err := json.Marshal(conversation.ModelMessage{
		Role:    "assistant",
		Content: content,
	})
	if err != nil {
		t.Fatalf("marshal model message: %v", err)
	}

	got := extractTextContent(raw)
	want := "[tool_call: read, edit]"
	if got != want {
		t.Fatalf("extractTextContent() = %q, want %q", got, want)
	}
}

func historyTestMessage(t *testing.T, id, sessionID, role, text string, createdAt time.Time) messagepkg.Message {
	t.Helper()

	rawText, err := json.Marshal(text)
	if err != nil {
		t.Fatalf("marshal text: %v", err)
	}
	content, err := json.Marshal(conversation.ModelMessage{
		Role:    role,
		Content: rawText,
	})
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}
	return messagepkg.Message{
		ID:        id,
		BotID:     "bot-1",
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: createdAt,
	}
}

func TestExtractTextContentSummarizesToolResults(t *testing.T) {
	t.Parallel()

	content, err := json.Marshal([]map[string]any{
		{"type": "tool-result", "toolName": "search_messages", "toolCallId": "call-1", "result": map[string]any{"count": 3}},
	})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	raw, err := json.Marshal(conversation.ModelMessage{
		Role:    "tool",
		Content: content,
	})
	if err != nil {
		t.Fatalf("marshal model message: %v", err)
	}

	got := extractTextContent(raw)
	want := "[tool_result: search_messages]"
	if got != want {
		t.Fatalf("extractTextContent() = %q, want %q", got, want)
	}
}
