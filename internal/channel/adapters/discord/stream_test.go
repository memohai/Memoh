package discord

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/memohai/memoh/internal/channel"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestDiscordOutboundStream_PushErrorEventRedactsSecrets(t *testing.T) {
	channel.ResetIMErrorSecretsForTest()
	t.Cleanup(channel.ResetIMErrorSecretsForTest)

	const token = "discord-token-ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	channel.SetIMErrorSecrets("test", token)
	prefixHalf := token[:len(token)/2]

	var sentBody string
	session, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	session.Client = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, _ := io.ReadAll(req.Body)
			sentBody = string(body)
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"msg-1","channel_id":"ch-1"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}
			return resp, nil
		}),
	}

	stream := &discordOutboundStream{
		adapter: &DiscordAdapter{},
		target:  "ch-1",
		session: session,
	}

	err = stream.Push(context.Background(), channel.PreparedStreamEvent{
		Type:  channel.StreamEventError,
		Error: "request failed: " + prefixHalf,
	})
	if err != nil {
		t.Fatalf("push error event: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(sentBody), &payload); err != nil {
		t.Fatalf("decode sent body: %v (body=%q)", err, sentBody)
	}
	content, _ := payload["content"].(string)
	if strings.Contains(content, prefixHalf) {
		t.Fatalf("expected prefix half to be redacted, got %q", content)
	}
	if !strings.Contains(content, "Error: ") {
		t.Fatalf("expected error prefix, got %q", content)
	}
	if !strings.Contains(content, strings.Repeat("*", len(prefixHalf))) {
		t.Fatalf("expected redaction mask, got %q", content)
	}
}
