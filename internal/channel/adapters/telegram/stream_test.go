package telegram

import (
	"context"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/memohai/memoh/internal/channel"
)

func TestTelegramOutboundStream_CloseNil(t *testing.T) {
	t.Parallel()

	var s *telegramOutboundStream
	ctx := context.Background()
	if err := s.Close(ctx); err != nil {
		t.Fatalf("Close on nil stream should return nil: %v", err)
	}
}

func TestTelegramOutboundStream_PushClosed(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter}
	s.closed.Store(true)

	ctx := context.Background()
	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "x"})
	if err == nil {
		t.Fatal("Push on closed stream should return error")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Fatalf("expected closed error: %v", err)
	}
}

func TestTelegramOutboundStream_PushStatusNoOp(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter}

	ctx := context.Background()
	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventStatus})
	if err != nil {
		t.Fatalf("StreamEventStatus should be no-op: %v", err)
	}
}

func TestTelegramOutboundStream_PushNilAdapter(t *testing.T) {
	t.Parallel()

	s := &telegramOutboundStream{adapter: nil}
	ctx := context.Background()
	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "x"})
	if err == nil {
		t.Fatal("Push with nil adapter should return error")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("expected not configured error: %v", err)
	}
}

func TestTelegramOutboundStream_PushUnknownEventTypeSkipped(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter}
	ctx := context.Background()

	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventType("unknown")})
	if err != nil {
		t.Fatalf("Push with unknown event type should be silently skipped: %v", err)
	}
}

func TestTelegramOutboundStream_PushEmptyDeltaNoOp(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter}
	ctx := context.Background()

	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: ""})
	if err != nil {
		t.Fatalf("empty delta should be no-op: %v", err)
	}
}

func TestTelegramOutboundStream_PushErrorEventEmptyNoOp(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter}
	ctx := context.Background()

	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventError, Error: ""})
	if err != nil {
		t.Fatalf("empty error event should be no-op: %v", err)
	}
}

func TestTelegramOutboundStream_CloseContextCanceled(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Close(ctx)
	if err != context.Canceled {
		t.Fatalf("Close with canceled context should return context.Canceled: %v", err)
	}
}

// Test editStreamMessage dedup: no API call when content equals lastEdited (avoids Telegram "message is not modified" error).
func TestEditStreamMessage_NoEditWhenSameContent(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:      adapter,
		streamChatID: 1,
		streamMsgID:  1,
		lastEdited:   "hello",
		lastEditedAt: time.Now().Add(-time.Minute),
	}
	ctx := context.Background()

	tests := []struct {
		name string
		text string
	}{
		{"exact same", "hello"},
		{"trimmed same", "  hello  "},
		{"leading space", " hello"},
		{"trailing space", "hello "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.editStreamMessage(ctx, tt.text)
			if err != nil {
				t.Fatalf("editStreamMessage(same content) should return nil to avoid duplicate edit API call: %v", err)
			}
		})
	}
}

func TestEditStreamMessage_NoEditWhenMessageNotSent(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter, streamMsgID: 0}
	ctx := context.Background()

	err := s.editStreamMessage(ctx, "any")
	if err != nil {
		t.Fatalf("editStreamMessage when streamMsgID==0 should return nil: %v", err)
	}
}

func TestEditStreamMessage_NoEditWhenThrottled(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:      adapter,
		streamChatID: 1,
		streamMsgID:  1,
		lastEdited:   "a",
		lastEditedAt: time.Now(), // just now, within throttle window
	}
	ctx := context.Background()

	err := s.editStreamMessage(ctx, "ab")
	if err != nil {
		t.Fatalf("editStreamMessage within throttle window should skip edit and return nil: %v", err)
	}
}

func TestEditStreamMessage_429SetsBackoffAndReturnsNil(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	before := time.Now().Add(-time.Minute)
	s := &telegramOutboundStream{
		adapter:      adapter,
		cfg:          channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		streamChatID: 1,
		streamMsgID:  1,
		lastEdited:   "a",
		lastEditedAt: before,
	}
	ctx := context.Background()

	origGetBot := getOrCreateBotForTest
	origEdit := testEditFunc
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	testEditFunc = func(*tgbotapi.BotAPI, int64, int, string, string) error {
		return tgbotapi.Error{
			Code:               429,
			Message:            "Too Many Requests",
			ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 2},
		}
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		testEditFunc = origEdit
	}()

	err := s.editStreamMessage(ctx, "b")
	if err != nil {
		t.Fatalf("editStreamMessage on 429 should return nil (backoff): %v", err)
	}
	s.mu.Lock()
	lastEdited := s.lastEdited
	lastEditedAt := s.lastEditedAt
	s.mu.Unlock()
	if lastEdited != "a" {
		t.Fatalf("on 429 lastEdited should remain unchanged: got %q", lastEdited)
	}
	if !lastEditedAt.After(before) {
		t.Fatalf("on 429 lastEditedAt should be pushed forward for backoff: got %v", lastEditedAt)
	}
}

func TestEditStreamMessageFinal_Success(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:      adapter,
		cfg:          channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		streamChatID: 1,
		streamMsgID:  1,
		lastEdited:   "a",
		lastEditedAt: time.Now().Add(-time.Minute),
	}
	ctx := context.Background()

	origGetBot := getOrCreateBotForTest
	origEdit := testEditFunc
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	testEditFunc = func(*tgbotapi.BotAPI, int64, int, string, string) error {
		return nil
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		testEditFunc = origEdit
	}()

	err := s.editStreamMessageFinal(ctx, "final text")
	if err != nil {
		t.Fatalf("editStreamMessageFinal should succeed: %v", err)
	}
	s.mu.Lock()
	lastEdited := s.lastEdited
	s.mu.Unlock()
	if lastEdited != "final text" {
		t.Fatalf("expected lastEdited to be updated: got %q", lastEdited)
	}
}

func TestEditStreamMessageFinal_SameContentNoOp(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:      adapter,
		streamChatID: 1,
		streamMsgID:  1,
		lastEdited:   "same",
		lastEditedAt: time.Now(),
	}
	ctx := context.Background()

	err := s.editStreamMessageFinal(ctx, "same")
	if err != nil {
		t.Fatalf("editStreamMessageFinal with same content should return nil: %v", err)
	}
}

func TestEditStreamMessageFinal_NoMessageNoOp(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{adapter: adapter, streamMsgID: 0}
	ctx := context.Background()

	err := s.editStreamMessageFinal(ctx, "any")
	if err != nil {
		t.Fatalf("editStreamMessageFinal when streamMsgID==0 should return nil: %v", err)
	}
}
