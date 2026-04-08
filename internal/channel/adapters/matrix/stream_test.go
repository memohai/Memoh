package matrix

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func mustPreparedMatrixEvent(t *testing.T, event channel.StreamEvent) channel.PreparedStreamEvent {
	t.Helper()
	prepared, err := channel.PrepareStreamEvent(context.Background(), nil, channel.ChannelConfig{
		BotID:       "bot-test",
		ChannelType: Type,
	}, event)
	if err != nil {
		t.Fatalf("prepare matrix stream event: %v", err)
	}
	return prepared
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMatrixStreamDoesNotSendDeltaBeforeTextPhaseEnds(t *testing.T) {
	requests := 0
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"event_id":"$evt1"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	stream := &matrixOutboundStream{
		adapter: adapter,
		cfg: Config{
			HomeserverURL: "https://matrix.example.com",
			AccessToken:   "tok",
		},
		target: "!room:example.com",
	}

	ctx := context.Background()
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "draft", Phase: channel.StreamPhaseText})); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no request before text phase ends, got %d", requests)
	}
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText})); err != nil {
		t.Fatalf("push phase end: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request after text phase end, got %d", requests)
	}
}

func TestMatrixStreamDropsBufferedTextWhenToolStarts(t *testing.T) {
	requests := 0
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"event_id":"$evt1"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	stream := &matrixOutboundStream{
		adapter: adapter,
		cfg: Config{
			HomeserverURL: "https://matrix.example.com",
			AccessToken:   "tok",
		},
		target: "!room:example.com",
	}

	ctx := context.Background()
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "I will inspect first", Phase: channel.StreamPhaseText})); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventToolCallStart})); err != nil {
		t.Fatalf("push tool call start: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no request for discarded pre-tool text, got %d", requests)
	}
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "Final answer", Phase: channel.StreamPhaseText})); err != nil {
		t.Fatalf("push final delta: %v", err)
	}
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventFinal, Final: &channel.StreamFinalizePayload{Message: channel.Message{Text: "Final answer"}}})); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected only final visible message to be sent, got %d", requests)
	}
}

func TestMatrixStreamFinalMarkdownUpdatesFormattedContent(t *testing.T) {
	bodies := make([]string, 0, 2)
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, string(payload))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"event_id":"$evt1"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	stream := &matrixOutboundStream{
		adapter: adapter,
		cfg: Config{
			HomeserverURL: "https://matrix.example.com",
			AccessToken:   "tok",
		},
		target: "!room:example.com",
	}

	ctx := context.Background()
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "**bold**", Phase: channel.StreamPhaseText})); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText})); err != nil {
		t.Fatalf("push phase end: %v", err)
	}
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventFinal, Final: &channel.StreamFinalizePayload{Message: channel.Message{Text: "**bold**", Format: channel.MessageFormatMarkdown}}})); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected two sends, got %d", len(bodies))
	}
	if strings.Contains(bodies[0], "formatted_body") {
		t.Fatalf("expected plain interim send, got %s", bodies[0])
	}
	if !strings.Contains(bodies[1], "formatted_body") || !strings.Contains(bodies[1], "org.matrix.custom.html") {
		t.Fatalf("expected markdown final edit, got %s", bodies[1])
	}
}

func TestMatrixStreamFinalSendsAttachments(t *testing.T) {
	bodies := make([]string, 0, 2)
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		bodies = append(bodies, string(payload))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"event_id":"$evt1"}`)),
			Header:     make(http.Header),
		}, nil
	})}

	stream := &matrixOutboundStream{
		adapter: adapter,
		cfg: Config{
			HomeserverURL: "https://matrix.example.com",
			AccessToken:   "tok",
		},
		target: "!room:example.com",
	}

	ctx := context.Background()
	if err := stream.Push(ctx, mustPreparedMatrixEvent(t, channel.StreamEvent{Type: channel.StreamEventFinal, Final: &channel.StreamFinalizePayload{Message: channel.Message{
		Text: "done",
		Attachments: []channel.Attachment{{
			Type:           channel.AttachmentImage,
			PlatformKey:    "mxc://matrix.example.com/media123",
			Name:           "image.png",
			SourcePlatform: Type.String(),
		}},
	}}})); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("expected text and attachment sends, got %d", len(bodies))
	}
	if !strings.Contains(bodies[0], `"msgtype":"m.notice"`) {
		t.Fatalf("expected first payload to be text, got %s", bodies[0])
	}
	if !strings.Contains(bodies[1], `"msgtype":"m.image"`) || !strings.Contains(bodies[1], `mxc://matrix.example.com/media123`) {
		t.Fatalf("expected second payload to be attachment, got %s", bodies[1])
	}
}
