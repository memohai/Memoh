package edge

import (
	"bytes"
	"context"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/tts"
)

func TestEdgeAdapter_TypeAndMeta(t *testing.T) {
	t.Parallel()
	adapter := NewEdgeAdapter(slog.Default())
	if adapter.Type() != TtsTypeEdge {
		t.Errorf("Type() = %q, want %q", adapter.Type(), TtsTypeEdge)
	}
	meta := adapter.Meta()
	if meta.Provider != "Microsoft Edge" {
		t.Errorf("Meta().Provider = %q, want %q", meta.Provider, "Microsoft Edge")
	}
	if meta.Description != "Microsoft Edge TTS" {
		t.Errorf("Meta().Description = %q, want %q", meta.Description, "Microsoft Edge TTS")
	}
}

func TestEdgeAdapter_Synthesize_WithMockServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(mockEdgeTTSHandler(t))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/edge/v1"
	client := NewEdgeWsClient()
	client.BaseURL = wsURL
	adapter := NewEdgeAdapterWithClient(slog.Default(), client)

	ctx := context.Background()
	config := tts.AudioConfig{Voice: "en-US-JennyNeural"}
	audio, err := adapter.Synthesize(ctx, "Hello", config)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if len(audio) == 0 {
		t.Fatal("expected non-empty audio")
	}
	if !bytes.Equal(audio, []byte("fake-webm-audio-data")) {
		t.Errorf("audio = %q", string(audio))
	}
}

func TestEdgeAdapter_Stream_WithMockServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(mockEdgeTTSHandler(t))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/edge/v1"
	client := NewEdgeWsClient()
	client.BaseURL = wsURL
	adapter := NewEdgeAdapterWithClient(slog.Default(), client)

	ctx := context.Background()
	config := tts.AudioConfig{Voice: "en-US-JennyNeural"}
	ch, errCh := adapter.Stream(ctx, "Hi", config)
	var chunks [][]byte
	for b := range ch {
		chunks = append(chunks, b)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Stream err: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !bytes.Equal(chunks[0], []byte("fake-webm-audio-data")) {
		t.Errorf("chunk = %q", chunks[0])
	}
}

func TestEdgeAdapter_Synthesize_NotConnected(t *testing.T) {
	t.Parallel()
	// use client without BaseURL and not Connect, Synthesize will try to connect real Edge address, here use invalid URL to trigger quick failure
	client := NewEdgeWsClient()
	client.BaseURL = "ws://127.0.0.1:0/edge/v1" // no service
	adapter := NewEdgeAdapterWithClient(slog.Default(), client)

	ctx := context.Background()
	_, err := adapter.Synthesize(ctx, "x", tts.AudioConfig{})
	if err == nil {
		t.Fatal("expected error when connection fails")
	}
}
