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

// --- Draft mode (sendMessageDraft) tests ---

func TestSendDraft_ThrottleSkip(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
		lastEditedAt:  time.Now(), // just now, within draft throttle window
	}
	ctx := context.Background()

	err := s.sendDraft(ctx, "hello")
	if err != nil {
		t.Fatalf("sendDraft within throttle window should skip and return nil: %v", err)
	}
}

func TestSendDraft_EmptyTextSkip(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
		lastEditedAt:  time.Now().Add(-time.Minute),
	}
	ctx := context.Background()

	err := s.sendDraft(ctx, "   ")
	if err != nil {
		t.Fatalf("sendDraft with whitespace-only text should skip and return nil: %v", err)
	}
}

func TestSendDraft_Success(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
		lastEditedAt:  time.Now().Add(-time.Minute),
	}
	ctx := context.Background()

	origGetBot := getOrCreateBotForTest
	origDraft := sendDraftForTest
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	var capturedChatID int64
	var capturedDraftID int
	var capturedText string
	sendDraftForTest = func(_ *tgbotapi.BotAPI, chatID int64, draftID int, text string, _ string) error {
		capturedChatID = chatID
		capturedDraftID = draftID
		capturedText = text
		return nil
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		sendDraftForTest = origDraft
	}()

	err := s.sendDraft(ctx, "streaming text")
	if err != nil {
		t.Fatalf("sendDraft should succeed: %v", err)
	}
	if capturedChatID != 123 {
		t.Fatalf("expected chatID 123, got %d", capturedChatID)
	}
	if capturedDraftID != 1 {
		t.Fatalf("expected draftID 1, got %d", capturedDraftID)
	}
	if capturedText != "streaming text" {
		t.Fatalf("expected text 'streaming text', got %q", capturedText)
	}
}

func TestSendDraft_429Backoff(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	before := time.Now().Add(-time.Minute)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
		lastEditedAt:  before,
	}
	ctx := context.Background()

	origGetBot := getOrCreateBotForTest
	origDraft := sendDraftForTest
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	sendDraftForTest = func(*tgbotapi.BotAPI, int64, int, string, string) error {
		return tgbotapi.Error{
			Code:               429,
			Message:            "Too Many Requests",
			ResponseParameters: tgbotapi.ResponseParameters{RetryAfter: 2},
		}
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		sendDraftForTest = origDraft
	}()

	err := s.sendDraft(ctx, "hello")
	if err != nil {
		t.Fatalf("sendDraft on 429 should return nil (backoff): %v", err)
	}
	s.mu.Lock()
	lastEditedAt := s.lastEditedAt
	s.mu.Unlock()
	if !lastEditedAt.After(before) {
		t.Fatalf("on 429 lastEditedAt should be pushed forward for backoff")
	}
}

func TestDraftMode_DeltaUsesSendDraft(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
	}
	ctx := context.Background()

	origGetBot := getOrCreateBotForTest
	origDraft := sendDraftForTest
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	draftCalls := 0
	sendDraftForTest = func(*tgbotapi.BotAPI, int64, int, string, string) error {
		draftCalls++
		return nil
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		sendDraftForTest = origDraft
	}()

	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "Hello "})
	if err != nil {
		t.Fatalf("Push delta should succeed: %v", err)
	}
	if draftCalls != 1 {
		t.Fatalf("expected 1 sendDraft call, got %d", draftCalls)
	}
	s.mu.Lock()
	buf := s.buf.String()
	s.mu.Unlock()
	if buf != "Hello " {
		t.Fatalf("expected buffer to be 'Hello ', got %q", buf)
	}
}

func TestDraftMode_PhaseEndTextIsNoOp(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
	}
	s.buf.WriteString("some content")
	ctx := context.Background()

	err := s.Push(ctx, channel.StreamEvent{
		Type:  channel.StreamEventPhaseEnd,
		Phase: channel.StreamPhaseText,
	})
	if err != nil {
		t.Fatalf("PhaseEnd in draft mode should be no-op: %v", err)
	}
}

func TestDraftMode_ToolCallStartSendsPermanentMessage(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		target:        "123",
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
	}
	s.buf.WriteString("partial text")
	ctx := context.Background()

	origGetBot := getOrCreateBotForTest
	origSendEdit := sendEditForTest
	origSendText := sendTextForTest
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	var sentText string
	sendTextForTest = func(_ *tgbotapi.BotAPI, _ string, text string, _ int, _ string) (int64, int, error) {
		sentText = text
		return 123, 1, nil
	}
	sendEditForTest = func(_ *tgbotapi.BotAPI, edit tgbotapi.EditMessageTextConfig) error {
		t.Error("editMessage should not be called in draft mode")
		return nil
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		sendEditForTest = origSendEdit
		sendTextForTest = origSendText
	}()

	err := s.Push(ctx, channel.StreamEvent{Type: channel.StreamEventToolCallStart})
	if err != nil {
		t.Fatalf("Push ToolCallStart should succeed: %v", err)
	}
	if sentText != "partial text" {
		t.Fatalf("expected sendPermanentMessage with 'partial text', got %q", sentText)
	}

	s.mu.Lock()
	bufAfter := s.buf.String()
	chatID := s.streamChatID
	s.mu.Unlock()
	if bufAfter != "" {
		t.Fatalf("buffer should be reset after ToolCallStart: got %q", bufAfter)
	}
	// streamChatID should be preserved in draft mode
	if chatID != 123 {
		t.Fatalf("streamChatID should be preserved in draft mode: got %d", chatID)
	}
}

func TestDraftMode_FinalEmptyBufferSkipsDuplicate(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		target:        "123",
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
	}
	ctx := context.Background()

	// Simulate: buffer was already committed during ToolCallStart, so it's empty.
	// StreamEventFinal should NOT re-send the message via PlainText() fallback.
	origGetBot := getOrCreateBotForTest
	origSendText := sendTextForTest
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	sendTextForTest = func(_ *tgbotapi.BotAPI, _ string, _ string, _ int, _ string) (int64, int, error) {
		t.Error("sendTelegramText should not be called when buffer is empty in draft mode")
		return 0, 0, nil
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		sendTextForTest = origSendText
	}()

	err := s.Push(ctx, channel.StreamEvent{
		Type: channel.StreamEventFinal,
		Final: &channel.StreamFinalizePayload{
			Message: channel.Message{Text: "already sent text"},
		},
	})
	if err != nil {
		t.Fatalf("StreamEventFinal with empty buffer in draft mode should succeed: %v", err)
	}
}

// TestDraftMode_MultipleFinalEventsOnlyOneSend verifies that when multiple
// StreamEventFinal events fire (one per assistant output in multi-tool-call
// responses), only the first one sends the buffer text as a permanent message.
// Subsequent finals find the buffer empty and skip sending.
func TestDraftMode_MultipleFinalEventsOnlyOneSend(t *testing.T) {
	t.Parallel()

	adapter := NewTelegramAdapter(nil)
	s := &telegramOutboundStream{
		adapter:       adapter,
		cfg:           channel.ChannelConfig{ID: "test", Credentials: map[string]any{"bot_token": "fake"}},
		target:        "123",
		isPrivateChat: true,
		draftID:       1,
		streamChatID:  123,
	}
	ctx := context.Background()

	// Simulate buffer containing the final summary text
	s.buf.WriteString("final summary")

	origGetBot := getOrCreateBotForTest
	origSendText := sendTextForTest
	getOrCreateBotForTest = func(_ *TelegramAdapter, _, _ string) (*tgbotapi.BotAPI, error) {
		return &tgbotapi.BotAPI{Token: "fake"}, nil
	}
	sendCount := 0
	sendTextForTest = func(_ *tgbotapi.BotAPI, _ string, _ string, _ int, _ string) (int64, int, error) {
		sendCount++
		return 123, 1, nil
	}
	defer func() {
		getOrCreateBotForTest = origGetBot
		sendTextForTest = origSendText
	}()

	// Push 3 StreamEventFinal events (simulating 3 assistant outputs).
	// Only the first should actually send a message.
	for i, text := range []string{"intermediate 1", "intermediate 2", "final summary"} {
		err := s.Push(ctx, channel.StreamEvent{
			Type: channel.StreamEventFinal,
			Final: &channel.StreamFinalizePayload{
				Message: channel.Message{Text: text},
			},
		})
		if err != nil {
			t.Fatalf("StreamEventFinal #%d should succeed: %v", i+1, err)
		}
	}

	if sendCount != 1 {
		t.Fatalf("expected exactly 1 sendTelegramText call, got %d", sendCount)
	}
}
