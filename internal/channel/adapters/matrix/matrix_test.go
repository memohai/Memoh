package matrix

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestIsMatrixBotMentionedByMentionsMetadata(t *testing.T) {
	content := map[string]any{
		"body": "hi bot",
		"m.mentions": map[string]any{
			"user_ids": []any{"@memoh:example.com"},
		},
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected mention metadata to be detected")
	}
}

func TestIsMatrixBotMentionedByFormattedBody(t *testing.T) {
	content := map[string]any{
		"body":           "hello Memoh",
		"formatted_body": `<a href="https://matrix.to/#/@memoh:example.com">Memoh</a> hello`,
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected formatted body mention to be detected")
	}
}

func TestIsMatrixBotMentionedByBodyFallback(t *testing.T) {
	content := map[string]any{
		"body": "@memoh:example.com ping",
	}
	if !isMatrixBotMentioned("@memoh:example.com", content) {
		t.Fatal("expected body fallback mention to be detected")
	}
}

func TestMatrixSinceTokenFromRouting(t *testing.T) {
	routing := map[string]any{
		matrixRoutingStateKey: map[string]any{"since_token": "s123"},
	}
	if got := matrixSinceTokenFromRouting(routing); got != "s123" {
		t.Fatalf("unexpected since token: %q", got)
	}
}

func TestPersistSinceTokenUsesConfiguredSaver(t *testing.T) {
	var gotConfigID string
	var gotSince string
	adapter := NewMatrixAdapter(nil)
	adapter.SetSyncStateSaver(func(_ context.Context, configID string, since string) error {
		gotConfigID = configID
		gotSince = since
		return nil
	})
	if err := adapter.persistSinceToken(context.Background(), "cfg-1", "token-1"); err != nil {
		t.Fatalf("persistSinceToken returned error: %v", err)
	}
	if gotConfigID != "cfg-1" || gotSince != "token-1" {
		t.Fatalf("unexpected saver args: %q %q", gotConfigID, gotSince)
	}
}

func TestBootstrapSinceTokenPersistsLatestCursor(t *testing.T) {
	adapter := NewMatrixAdapter(nil)
	adapter.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"next_batch":"s123","rooms":{"join":{"!room:example.com":{"timeline":{"events":[{"event_id":"$evt1"}]}}}}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	var gotConfigID string
	var gotSince string
	adapter.SetSyncStateSaver(func(_ context.Context, configID string, since string) error {
		gotConfigID = configID
		gotSince = since
		return nil
	})

	since, err := adapter.bootstrapSinceToken(context.Background(), channel.ChannelConfig{ID: "cfg-1"}, Config{
		HomeserverURL: "https://matrix.example.com",
		AccessToken:   "tok",
	})
	if err != nil {
		t.Fatalf("bootstrapSinceToken returned error: %v", err)
	}
	if since != "s123" {
		t.Fatalf("unexpected since token: %q", since)
	}
	if gotConfigID != "cfg-1" || gotSince != "s123" {
		t.Fatalf("unexpected persisted cursor: %q %q", gotConfigID, gotSince)
	}
	if !adapter.seenEvent("cfg-1", "$evt1") {
		t.Fatal("expected bootstrap event to be remembered as seen")
	}
}
