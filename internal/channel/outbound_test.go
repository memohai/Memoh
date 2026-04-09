package channel

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/memohai/memoh/internal/channel/channeltest"
)

type streamValidationAdapter struct {
	channelType    ChannelType
	outboundPolicy OutboundPolicy
}

func (a *streamValidationAdapter) Type() ChannelType {
	return a.channelType
}

func (a *streamValidationAdapter) Descriptor() Descriptor {
	return Descriptor{
		Type:        a.channelType,
		DisplayName: "stream-validation",
		Capabilities: ChannelCapabilities{
			Text:           true,
			Markdown:       true,
			Attachments:    true,
			Streaming:      true,
			BlockStreaming: true,
			Buttons:        true,
			Threads:        true,
		},
		OutboundPolicy: a.outboundPolicy,
	}
}

func newStreamValidationRegistry(t *testing.T) *Registry {
	t.Helper()
	registry := NewRegistry()
	if err := registry.Register(&streamValidationAdapter{channelType: ChannelType("test")}); err != nil {
		t.Fatalf("register adapter failed: %v", err)
	}
	return registry
}

func TestValidateStreamEventSupportedTypes(t *testing.T) {
	t.Parallel()

	registry := newStreamValidationRegistry(t)
	channelType := ChannelType("test")
	tests := []struct {
		name  string
		event StreamEvent
	}{
		{name: "status", event: StreamEvent{Type: StreamEventStatus, Status: StreamStatusStarted}},
		{name: "delta", event: StreamEvent{Type: StreamEventDelta, Delta: "hello"}},
		{name: "phase start", event: StreamEvent{Type: StreamEventPhaseStart, Phase: StreamPhaseText}},
		{name: "phase end", event: StreamEvent{Type: StreamEventPhaseEnd, Phase: StreamPhaseText}},
		{name: "tool start", event: StreamEvent{Type: StreamEventToolCallStart, ToolCall: &StreamToolCall{Name: "search"}}},
		{name: "tool end", event: StreamEvent{Type: StreamEventToolCallEnd, ToolCall: &StreamToolCall{Name: "search"}}},
		{name: "attachment", event: StreamEvent{Type: StreamEventAttachment, Attachments: []Attachment{{Type: AttachmentImage, URL: "https://example.com/img.png"}}}},
		{name: "agent start", event: StreamEvent{Type: StreamEventAgentStart}},
		{name: "agent end", event: StreamEvent{Type: StreamEventAgentEnd}},
		{name: "processing started", event: StreamEvent{Type: StreamEventProcessingStarted}},
		{name: "processing completed", event: StreamEvent{Type: StreamEventProcessingCompleted}},
		{name: "processing failed", event: StreamEvent{Type: StreamEventProcessingFailed, Error: "failed"}},
		{name: "final", event: StreamEvent{Type: StreamEventFinal, Final: &StreamFinalizePayload{Message: Message{Text: "done"}}}},
		{name: "error", event: StreamEvent{Type: StreamEventError, Error: "boom"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateStreamEvent(registry, channelType, tt.event); err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
		})
	}
}

func TestValidateStreamEventInvalidPayload(t *testing.T) {
	t.Parallel()

	registry := newStreamValidationRegistry(t)
	channelType := ChannelType("test")
	tests := []struct {
		name  string
		event StreamEvent
	}{
		{name: "missing status", event: StreamEvent{Type: StreamEventStatus}},
		{name: "missing tool call payload", event: StreamEvent{Type: StreamEventToolCallStart}},
		{name: "empty attachment payload", event: StreamEvent{Type: StreamEventAttachment}},
		{name: "processing failed missing error", event: StreamEvent{Type: StreamEventProcessingFailed}},
		{name: "missing final payload", event: StreamEvent{Type: StreamEventFinal}},
		{name: "missing error payload", event: StreamEvent{Type: StreamEventError}},
		{name: "unsupported type", event: StreamEvent{Type: StreamEventType("unknown")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := validateStreamEvent(registry, channelType, tt.event); err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}
}

// recordingStream captures events pushed to it.
type recordingStream struct {
	mu     sync.Mutex
	events []StreamEvent
}

func (r *recordingStream) Push(_ context.Context, event PreparedStreamEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event.LogicalEvent())
	return nil
}

func (*recordingStream) Close(_ context.Context) error { return nil }

func (r *recordingStream) Events() []StreamEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]StreamEvent, len(r.events))
	copy(out, r.events)
	return out
}

type failingFinalStream struct {
	recordingStream
}

func (f *failingFinalStream) Push(ctx context.Context, event PreparedStreamEvent) error {
	if event.Type == StreamEventFinal {
		return context.DeadlineExceeded
	}
	return f.recordingStream.Push(ctx, event)
}

func newChunkingTestStream(t *testing.T, chunkLimit int) (*managerOutboundStream, *recordingStream, *[]OutboundMessage) {
	t.Helper()
	channelType := ChannelType("telegram")
	registry := NewRegistry()
	adapter := &streamValidationAdapter{
		channelType:    channelType,
		outboundPolicy: OutboundPolicy{TextChunkLimit: chunkLimit},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter failed: %v", err)
	}
	manager := &Manager{registry: registry, attachmentStore: channeltest.NewMemoryAttachmentStore()}

	rec := &recordingStream{}
	var sent []OutboundMessage
	var mu sync.Mutex

	stream := &managerOutboundStream{
		manager:     manager,
		config:      ChannelConfig{BotID: "bot-1", ChannelType: channelType},
		stream:      rec,
		channelType: channelType,
		policy:      manager.resolveOutboundPolicy(channelType),
		send: func(_ context.Context, msg OutboundMessage) error {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, msg)
			return nil
		},
	}
	return stream, rec, &sent
}

func TestPushFinalWithChunking_ShortText(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 2000)

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{Text: "short text", Format: MessageFormatPlain},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 stream event, got %d", len(events))
	}
	if events[0].Final.Message.Text != "short text" {
		t.Fatalf("expected original text, got %q", events[0].Final.Message.Text)
	}
	if len(*sent) != 0 {
		t.Fatalf("expected no overflow sends, got %d", len(*sent))
	}
}

func TestPushFinalWithChunking_LongText(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 100)

	lines := make([]string, 0, 10)
	for i := range 10 {
		line := strings.Repeat("x", 20)
		if i > 0 {
			line = strings.Repeat("y", 20)
		}
		lines = append(lines, line)
	}
	longText := strings.Join(lines, "\n")

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{Text: longText, Format: MessageFormatPlain},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 stream event (first chunk), got %d", len(events))
	}
	firstChunk := events[0].Final.Message.Text
	if runeLen(firstChunk) > 100 {
		t.Fatalf("first chunk exceeds limit: %d runes", runeLen(firstChunk))
	}
	if len(*sent) == 0 {
		t.Fatal("expected overflow sends, got none")
	}
	for i, msg := range *sent {
		if runeLen(msg.Message.Text) > 100 {
			t.Fatalf("overflow chunk %d exceeds limit: %d runes", i, runeLen(msg.Message.Text))
		}
	}

	var reconstructed strings.Builder
	reconstructed.WriteString(firstChunk)
	for _, msg := range *sent {
		reconstructed.WriteString("\n")
		reconstructed.WriteString(msg.Message.Text)
	}
	if strings.TrimSpace(reconstructed.String()) != strings.TrimSpace(longText) {
		t.Fatal("reconstructed text does not match original")
	}
}

func TestPushFinalWithChunking_AttachmentsSeparated(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 50)

	longText := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	attachments := []Attachment{{Type: AttachmentImage, URL: "https://example.com/img.png"}}

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{
				Text:        longText,
				Format:      MessageFormatPlain,
				Attachments: attachments,
			},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 stream event, got %d", len(events))
	}
	if len(events[0].Final.Message.Attachments) != 0 {
		t.Fatal("first chunk should not contain attachments")
	}

	hasAttachment := false
	for _, msg := range *sent {
		if len(msg.Message.Attachments) > 0 {
			hasAttachment = true
			if msg.Message.Attachments[0].URL != "https://example.com/img.png" {
				t.Fatal("attachment URL mismatch")
			}
		}
	}
	if !hasAttachment {
		t.Fatal("expected attachments in overflow sends")
	}
}

func TestBuildOutboundMessages_InlineTextWithMediaMovesTextToCaption(t *testing.T) {
	t.Parallel()

	msgs, err := buildOutboundMessages(OutboundMessage{
		Target: "chat-1",
		Message: Message{
			Text: "test.jpg from QQ",
			Attachments: []Attachment{{
				Type: AttachmentImage,
				URL:  "https://example.com/test.jpg",
			}},
		},
	}, OutboundPolicy{
		TextChunkLimit:      100,
		MediaOrder:          OutboundOrderTextFirst,
		InlineTextWithMedia: true,
	})
	if err != nil {
		t.Fatalf("buildOutboundMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 outbound message, got %d", len(msgs))
	}
	if got := strings.TrimSpace(msgs[0].Message.Text); got != "" {
		t.Fatalf("expected inline caption to suppress standalone text, got %q", got)
	}
	if len(msgs[0].Message.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(msgs[0].Message.Attachments))
	}
	if got := msgs[0].Message.Attachments[0].Caption; got != "test.jpg from QQ" {
		t.Fatalf("unexpected attachment caption: %q", got)
	}
}

func TestPushFinalWithChunking_NonFinalPassthrough(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 100)

	event := StreamEvent{
		Type:   StreamEventStatus,
		Status: StreamStatusStarted,
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != StreamEventStatus {
		t.Fatalf("expected status event, got %s", events[0].Type)
	}
	if len(*sent) != 0 {
		t.Fatalf("expected no overflow sends, got %d", len(*sent))
	}
}

func TestPushFinalWithChunking_MarkdownFormat(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 100)

	para1 := strings.Repeat("a", 60)
	para2 := strings.Repeat("b", 60)
	longText := para1 + "\n\n" + para2

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{Text: longText, Format: MessageFormatMarkdown},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 stream event, got %d", len(events))
	}
	if events[0].Final.Message.Format != MessageFormatMarkdown {
		t.Fatal("first chunk should preserve markdown format")
	}
	if len(*sent) != 1 {
		t.Fatalf("expected 1 overflow send, got %d", len(*sent))
	}
	if (*sent)[0].Message.Format != MessageFormatMarkdown {
		t.Fatal("overflow chunk should preserve markdown format")
	}
}

func TestPushFinalWithChunking_ActionsOnLastChunk(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 50)

	longText := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	actions := []Action{{Type: "button", Label: "Click me", URL: "https://example.com"}}

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{
				Text:    longText,
				Format:  MessageFormatPlain,
				Actions: actions,
			},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 stream event, got %d", len(events))
	}
	if len(events[0].Final.Message.Actions) != 0 {
		t.Fatal("first chunk should NOT have actions when there are overflow chunks")
	}
	if len(*sent) == 0 {
		t.Fatal("expected overflow sends")
	}
	lastSent := (*sent)[len(*sent)-1]
	if len(lastSent.Message.Actions) != 1 || lastSent.Message.Actions[0].Label != "Click me" {
		t.Fatal("actions should be on the last overflow message")
	}
}

func TestPushFinalWithChunking_ActionsOnAttachmentMsg(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 50)

	longText := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	actions := []Action{{Type: "button", Label: "OK"}}
	attachments := []Attachment{{Type: AttachmentImage, URL: "https://example.com/img.png"}}

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{
				Text:        longText,
				Format:      MessageFormatPlain,
				Actions:     actions,
				Attachments: attachments,
			},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events[0].Final.Message.Actions) != 0 {
		t.Fatal("first chunk should NOT have actions")
	}

	var attachmentMsg *OutboundMessage
	for i := range *sent {
		if len((*sent)[i].Message.Attachments) > 0 {
			attachmentMsg = &(*sent)[i]
		}
	}
	if attachmentMsg == nil {
		t.Fatal("expected attachment message in overflow")
		return
	}
	if len(attachmentMsg.Message.Actions) != 1 || attachmentMsg.Message.Actions[0].Label != "OK" {
		t.Fatal("actions should be on the attachment (last) message")
	}

	for _, msg := range *sent {
		if len(msg.Message.Attachments) == 0 && len(msg.Message.Actions) > 0 {
			t.Fatal("text-only overflow chunks should NOT have actions when attachment message exists")
		}
	}
}

func TestPushFinalWithChunking_ThreadPropagated(t *testing.T) {
	t.Parallel()
	stream, rec, sent := newChunkingTestStream(t, 50)

	longText := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	thread := &ThreadRef{ID: "thread-123"}

	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{
				Text:   longText,
				Format: MessageFormatPlain,
				Thread: thread,
			},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	events := rec.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 stream event, got %d", len(events))
	}

	if len(*sent) == 0 {
		t.Fatal("expected overflow sends")
	}
	for i, msg := range *sent {
		if msg.Message.Thread == nil || msg.Message.Thread.ID != "thread-123" {
			t.Fatalf("overflow chunk %d should have thread propagated", i)
		}
	}
}

func TestPushFinalWithChunking_FirstChunkPushFailureFallback(t *testing.T) {
	t.Parallel()

	channelType := ChannelType("telegram")
	registry := NewRegistry()
	adapter := &streamValidationAdapter{
		channelType:    channelType,
		outboundPolicy: OutboundPolicy{TextChunkLimit: 100},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter failed: %v", err)
	}
	manager := &Manager{registry: registry, attachmentStore: channeltest.NewMemoryAttachmentStore()}

	rec := &failingFinalStream{}
	var sent []OutboundMessage
	stream := &managerOutboundStream{
		manager:     manager,
		config:      ChannelConfig{BotID: "bot-1", ChannelType: channelType},
		stream:      rec,
		channelType: channelType,
		policy:      manager.resolveOutboundPolicy(channelType),
		send: func(_ context.Context, msg OutboundMessage) error {
			sent = append(sent, msg)
			return nil
		},
	}

	longText := strings.Repeat("a", 80) + "\n" + strings.Repeat("b", 80) + "\n" + strings.Repeat("c", 80)
	event := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{Text: longText, Format: MessageFormatPlain},
		},
	}
	if err := stream.Push(context.Background(), event); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	if len(rec.Events()) != 0 {
		t.Fatalf("expected no stream events after first chunk push failure, got %d", len(rec.Events()))
	}
	if len(sent) < 2 {
		t.Fatalf("expected fallback sends for all chunks, got %d", len(sent))
	}

	var reconstructed strings.Builder
	for i, msg := range sent {
		if i > 0 {
			reconstructed.WriteString("\n")
		}
		reconstructed.WriteString(msg.Message.Text)
	}
	if strings.TrimSpace(reconstructed.String()) != strings.TrimSpace(longText) {
		t.Fatal("fallback reconstructed text does not match original")
	}
}

// reopenableStream wraps a list of recordingStreams; each reopen returns the next.
type reopenableStream struct {
	streams []*recordingStream
	idx     int
}

func (r *reopenableStream) current() *recordingStream {
	if r.idx >= len(r.streams) {
		return nil
	}
	return r.streams[r.idx]
}

func (r *reopenableStream) reopen(_ context.Context) (PreparedOutboundStream, error) {
	r.idx++
	if r.idx >= len(r.streams) {
		return nil, errors.New("no more streams")
	}
	return r.streams[r.idx], nil
}

func newDeltaSplitTestStream(t *testing.T, chunkLimit int, streamCount int) (*managerOutboundStream, *reopenableStream, *[]OutboundMessage) {
	t.Helper()
	channelType := ChannelType("telegram")
	registry := NewRegistry()
	adapter := &streamValidationAdapter{
		channelType:    channelType,
		outboundPolicy: OutboundPolicy{TextChunkLimit: chunkLimit},
	}
	if err := registry.Register(adapter); err != nil {
		t.Fatalf("register adapter failed: %v", err)
	}
	manager := &Manager{registry: registry, attachmentStore: channeltest.NewMemoryAttachmentStore()}

	streams := make([]*recordingStream, streamCount)
	for i := range streams {
		streams[i] = &recordingStream{}
	}
	reo := &reopenableStream{streams: streams}

	var sent []OutboundMessage
	var mu sync.Mutex

	stream := &managerOutboundStream{
		manager:     manager,
		config:      ChannelConfig{BotID: "bot-1", ChannelType: channelType},
		stream:      streams[0],
		channelType: channelType,
		policy:      manager.resolveOutboundPolicy(channelType),
		send: func(_ context.Context, msg OutboundMessage) error {
			mu.Lock()
			defer mu.Unlock()
			sent = append(sent, msg)
			return nil
		},
		reopen: reo.reopen,
	}
	return stream, reo, &sent
}

func TestPushDelta_NoSplitUnderLimit(t *testing.T) {
	t.Parallel()
	stream, reo, _ := newDeltaSplitTestStream(t, 100, 2)

	for i := range 5 {
		_ = i
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("a", 10),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	events := reo.streams[0].Events()
	if len(events) != 5 {
		t.Fatalf("expected 5 delta events on stream 0, got %d", len(events))
	}
	if stream.splitCount != 0 {
		t.Fatal("expected no splits")
	}
}

func TestPushDelta_SplitsAtLimit(t *testing.T) {
	t.Parallel()
	stream, reo, _ := newDeltaSplitTestStream(t, 100, 3)

	for range 12 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("x", 10),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	if stream.splitCount != 1 {
		t.Fatalf("expected 1 split, got %d", stream.splitCount)
	}

	s0 := reo.streams[0].Events()
	hasFinal := false
	deltaCount := 0
	for _, e := range s0 {
		if e.Type == StreamEventFinal {
			hasFinal = true
		}
		if e.Type == StreamEventDelta {
			deltaCount++
		}
	}
	if !hasFinal {
		t.Fatal("stream 0 should have received a Final event for finalization")
	}
	if deltaCount != 10 {
		t.Fatalf("stream 0 should have 10 deltas (100 runes), got %d", deltaCount)
	}

	s1 := reo.streams[1].Events()
	hasStarted := false
	deltaCount1 := 0
	for _, e := range s1 {
		if e.Type == StreamEventStatus && e.Status == StreamStatusStarted {
			hasStarted = true
		}
		if e.Type == StreamEventDelta {
			deltaCount1++
		}
	}
	if !hasStarted {
		t.Fatal("continuation stream should receive Status(Started) before deltas")
	}
	if deltaCount1 != 2 {
		t.Fatalf("stream 1 should have 2 deltas (overflow), got %d", deltaCount1)
	}
}

func TestPushDelta_MultipleSplits(t *testing.T) {
	t.Parallel()
	stream, reo, _ := newDeltaSplitTestStream(t, 50, 5)

	for range 15 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("y", 10),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	if stream.splitCount != 2 {
		t.Fatalf("expected 2 splits for 150 runes / 50 limit, got %d", stream.splitCount)
	}

	for i := 0; i <= 2; i++ {
		events := reo.streams[i].Events()
		if len(events) == 0 {
			t.Fatalf("stream %d has no events", i)
		}
	}
}

func TestPushDelta_FinalAfterSplitUsesBuffer(t *testing.T) {
	t.Parallel()
	stream, reo, sent := newDeltaSplitTestStream(t, 50, 3)

	for range 8 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("z", 10),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	if stream.splitCount == 0 {
		t.Fatal("expected at least 1 split")
	}

	finalEvent := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{
				Text:   strings.Repeat("z", 80),
				Format: MessageFormatPlain,
			},
		},
	}
	if err := stream.Push(context.Background(), finalEvent); err != nil {
		t.Fatalf("Final Push failed: %v", err)
	}

	lastStream := reo.current()
	events := lastStream.Events()
	hasFinal := false
	for _, e := range events {
		if e.Type == StreamEventFinal {
			hasFinal = true
			if !e.Final.Message.IsEmpty() {
				t.Fatal("after split, Final should have empty message (adapter uses buffer)")
			}
		}
	}
	if !hasFinal {
		t.Fatal("last stream should have received a Final event")
	}
	_ = sent
}

func TestPushDelta_FinalWithAttachmentsAfterSplit(t *testing.T) {
	t.Parallel()
	stream, _, sent := newDeltaSplitTestStream(t, 50, 3)

	for range 8 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("w", 10),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	attachments := []Attachment{{Type: AttachmentImage, URL: "https://example.com/img.png"}}
	actions := []Action{{Type: "button", Label: "OK"}}
	finalEvent := StreamEvent{
		Type: StreamEventFinal,
		Final: &StreamFinalizePayload{
			Message: Message{
				Text:        strings.Repeat("w", 80),
				Format:      MessageFormatPlain,
				Attachments: attachments,
				Actions:     actions,
			},
		},
	}
	if err := stream.Push(context.Background(), finalEvent); err != nil {
		t.Fatalf("Final Push failed: %v", err)
	}

	if len(*sent) != 1 {
		t.Fatalf("expected 1 attachment send, got %d", len(*sent))
	}
	if len((*sent)[0].Message.Attachments) != 1 {
		t.Fatal("expected attachments forwarded via send")
	}
	if len((*sent)[0].Message.Actions) != 1 || (*sent)[0].Message.Actions[0].Label != "OK" {
		t.Fatal("expected actions forwarded with attachments")
	}
}

func TestPushDelta_NoSplitWhenNoLimit(t *testing.T) {
	t.Parallel()
	stream, reo, _ := newDeltaSplitTestStream(t, 0, 2)

	for range 20 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("a", 100),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	if stream.splitCount != 0 {
		t.Fatal("expected no splits when TextChunkLimit is 0")
	}

	events := reo.streams[0].Events()
	if len(events) != 20 {
		t.Fatalf("expected all 20 deltas on stream 0, got %d", len(events))
	}
}

func TestPushDelta_ReasoningPhaseNotCounted(t *testing.T) {
	t.Parallel()
	stream, _, _ := newDeltaSplitTestStream(t, 50, 2)

	for range 10 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("r", 100),
			Phase: StreamPhaseReasoning,
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}

	if stream.splitCount != 0 {
		t.Fatal("reasoning deltas should not trigger splits")
	}
	if stream.deltaRunes != 0 {
		t.Fatal("reasoning deltas should not be counted")
	}
}

func TestPushDelta_SplitsAtNaturalBreak(t *testing.T) {
	t.Parallel()
	// limit=100, softLimit = 100 - 100/4 = 75
	stream, reo, _ := newDeltaSplitTestStream(t, 100, 3)

	// Push 70 runes — under soft limit.
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: strings.Repeat("a", 70),
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stream.splitCount != 0 {
		t.Fatal("should not split under soft limit")
	}

	// Push 10 runes ending with a sentence period → 80 runes total,
	// above soft (75) but under hard (100). Text ends with "." → natural break.
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: strings.Repeat("b", 9) + ".",
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stream.splitCount != 1 {
		t.Fatalf("expected split at natural break point, got %d splits", stream.splitCount)
	}
	if stream.deltaRunes != 0 {
		t.Fatalf("deltaRunes should reset after natural-break split, got %d", stream.deltaRunes)
	}

	// Verify stream 0 got both deltas and a Final.
	s0 := reo.streams[0].Events()
	deltaCount := 0
	hasFinal := false
	for _, e := range s0 {
		if e.Type == StreamEventDelta {
			deltaCount++
		}
		if e.Type == StreamEventFinal {
			hasFinal = true
		}
	}
	if deltaCount != 2 {
		t.Fatalf("stream 0: expected 2 deltas, got %d", deltaCount)
	}
	if !hasFinal {
		t.Fatal("stream 0 should have a Final event")
	}

	// Push more content to the continuation stream.
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: "continuation text",
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	s1 := reo.streams[1].Events()
	hasStarted := false
	for _, e := range s1 {
		if e.Type == StreamEventStatus && e.Status == StreamStatusStarted {
			hasStarted = true
		}
	}
	if !hasStarted {
		t.Fatal("continuation stream should receive Status(Started)")
	}
}

func TestPushDelta_NoEarlySplitWithoutBreakPoint(t *testing.T) {
	t.Parallel()
	// limit=100, softLimit=75
	stream, _, _ := newDeltaSplitTestStream(t, 100, 3)

	// Push 90 runes of plain text (no break point) — above soft, under hard.
	for range 9 {
		if err := stream.Push(context.Background(), StreamEvent{
			Type:  StreamEventDelta,
			Delta: strings.Repeat("x", 10),
		}); err != nil {
			t.Fatalf("Push failed: %v", err)
		}
	}
	if stream.splitCount != 0 {
		t.Fatalf("should NOT split in soft zone without a natural break, got %d", stream.splitCount)
	}
	if stream.deltaRunes != 90 {
		t.Fatalf("expected 90 accumulated runes, got %d", stream.deltaRunes)
	}

	// Push 20 more runes → 110 total → exceeds hard limit → force split.
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: strings.Repeat("x", 20),
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stream.splitCount != 1 {
		t.Fatalf("expected force-split at hard limit, got %d splits", stream.splitCount)
	}
}

func TestPushDelta_NewlineTriggersBreak(t *testing.T) {
	t.Parallel()
	stream, _, _ := newDeltaSplitTestStream(t, 100, 3)

	// 76 runes — just past soft limit (75).
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: strings.Repeat("w", 76),
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stream.splitCount != 0 {
		t.Fatal("no break yet")
	}

	// Newline → natural break.
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: "\n",
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stream.splitCount != 1 {
		t.Fatalf("expected split on newline in soft zone, got %d", stream.splitCount)
	}
}

func TestPushDelta_ChinesePunctuationBreak(t *testing.T) {
	t.Parallel()
	stream, _, _ := newDeltaSplitTestStream(t, 100, 3)

	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: strings.Repeat("字", 76),
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	// Chinese period → natural break.
	if err := stream.Push(context.Background(), StreamEvent{
		Type:  StreamEventDelta,
		Delta: "。",
	}); err != nil {
		t.Fatalf("Push failed: %v", err)
	}
	if stream.splitCount != 1 {
		t.Fatalf("expected split on Chinese period, got %d", stream.splitCount)
	}
}

func TestIsNaturalBreakPoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		text   string
		expect bool
	}{
		{"empty", "", false},
		{"plain word", "hello", false},
		{"period", "end.", true},
		{"period with trailing space", "end. ", true},
		{"question mark", "why?", true},
		{"exclamation", "wow!", true},
		{"Chinese period", "结束。", true},
		{"Chinese question", "什么？", true},
		{"Chinese exclamation", "好！", true},
		{"ellipsis", "wait…", true},
		{"newline", "line\n", true},
		{"double newline", "paragraph\n\n", true},
		{"semicolon", "clause;", true},
		{"Chinese semicolon", "分号；", true},
		{"comma", "hello,", false},
		{"mid-word", "incompl", false},
		{"version number", "v2.0", false},
		{"code dot", "obj.method", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isNaturalBreakPoint(tc.text)
			if got != tc.expect {
				t.Errorf("isNaturalBreakPoint(%q) = %v, want %v", tc.text, got, tc.expect)
			}
		})
	}
}

func TestNormalizeOutboundMessage_MarkdownDetected(t *testing.T) {
	t.Parallel()
	msg := normalizeOutboundMessage(Message{Text: "Hello **world**"})
	if msg.Format != MessageFormatMarkdown {
		t.Errorf("expected %q, got %q", MessageFormatMarkdown, msg.Format)
	}
}

func TestNormalizeOutboundMessage_PlainText(t *testing.T) {
	t.Parallel()
	msg := normalizeOutboundMessage(Message{Text: "Hello world"})
	if msg.Format != MessageFormatPlain {
		t.Errorf("expected %q, got %q", MessageFormatPlain, msg.Format)
	}
}

func TestNormalizeOutboundMessage_ExplicitFormatPreserved(t *testing.T) {
	t.Parallel()
	msg := normalizeOutboundMessage(Message{Text: "Hello **world**", Format: MessageFormatPlain})
	if msg.Format != MessageFormatPlain {
		t.Errorf("expected explicit format %q preserved, got %q", MessageFormatPlain, msg.Format)
	}
}

func TestNormalizeOutboundMessage_RichParts(t *testing.T) {
	t.Parallel()
	msg := normalizeOutboundMessage(Message{Parts: []MessagePart{{Type: "text", Text: "hello"}}})
	if msg.Format != MessageFormatRich {
		t.Errorf("expected %q, got %q", MessageFormatRich, msg.Format)
	}
}
