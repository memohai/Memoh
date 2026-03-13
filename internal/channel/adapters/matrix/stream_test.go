package matrix

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestMatrixStreamDoesNotSendDeltaBeforeTextPhaseEnds(t *testing.T) {
	requests := 0
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "draft", Phase: channel.StreamPhaseText}); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no request before text phase ends, got %d", requests)
	}
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventPhaseEnd, Phase: channel.StreamPhaseText}); err != nil {
		t.Fatalf("push phase end: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request after text phase end, got %d", requests)
	}
}

func TestMatrixStreamDropsBufferedTextWhenToolStarts(t *testing.T) {
	requests := 0
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
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
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "I will inspect first", Phase: channel.StreamPhaseText}); err != nil {
		t.Fatalf("push delta: %v", err)
	}
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventToolCallStart}); err != nil {
		t.Fatalf("push tool call start: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no request for discarded pre-tool text, got %d", requests)
	}
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventDelta, Delta: "Final answer", Phase: channel.StreamPhaseText}); err != nil {
		t.Fatalf("push final delta: %v", err)
	}
	if err := stream.Push(ctx, channel.StreamEvent{Type: channel.StreamEventFinal, Final: &channel.StreamFinalizePayload{Message: channel.Message{Text: "Final answer"}}}); err != nil {
		t.Fatalf("push final: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected only final visible message to be sent, got %d", requests)
	}
}
