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

func TestDiscordOutboundStreamSendToolCallMessagePostsEmbed(t *testing.T) {
	t.Parallel()

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
	p := channel.ToolCallPresentation{
		Emoji:    "💻",
		ToolName: "exec",
		Status:   channel.ToolCallStatusCompleted,
		Header:   "$ pnpm test",
		Body: []channel.ToolCallBlock{
			{Type: channel.ToolCallBlockText, Text: "stdout: ok"},
			{Type: channel.ToolCallBlockLink, Title: "trace", URL: "https://example.test/run", Desc: "open it"},
			{Type: channel.ToolCallBlockCode, Title: "stderr", Text: "line 1\nline 2"},
		},
		Footer: "exit=0",
	}

	if err := stream.sendToolCallMessage(&channel.StreamToolCall{CallID: "call-1"}, p); err != nil {
		t.Fatalf("sendToolCallMessage: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(sentBody), &payload); err != nil {
		t.Fatalf("decode sent body: %v (body=%q)", err, sentBody)
	}
	if payload["content"] == "" {
		t.Fatalf("expected fallback content, got %#v", payload)
	}
	allowedMentions, ok := payload["allowed_mentions"].(map[string]any)
	if !ok {
		t.Fatalf("expected allowed_mentions to disable fallback pings, got %#v", payload["allowed_mentions"])
	}
	parse, ok := allowedMentions["parse"].([]any)
	if !ok || len(parse) != 0 {
		t.Fatalf("expected no allowed mention parse types, got %#v", allowedMentions["parse"])
	}
	embeds, ok := payload["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("expected one embed, got %#v", payload["embeds"])
	}
	embed, ok := embeds[0].(map[string]any)
	if !ok {
		t.Fatalf("expected embed object, got %#v", embeds[0])
	}
	if embed["title"] != "💻 exec · completed" {
		t.Fatalf("unexpected embed title: %#v", embed["title"])
	}
	if embed["description"] != "$ pnpm test" {
		t.Fatalf("unexpected embed description: %#v", embed["description"])
	}
	if embed["color"] == nil {
		t.Fatalf("expected status color, got %#v", embed)
	}
	fields, ok := embed["fields"].([]any)
	if !ok || len(fields) != 3 {
		t.Fatalf("expected body fields, got %#v", embed["fields"])
	}
	footer, ok := embed["footer"].(map[string]any)
	if !ok || footer["text"] != "exit=0" {
		t.Fatalf("expected footer text, got %#v", embed["footer"])
	}
}

func TestRenderDiscordToolCallMessageEscapesEmbedMarkdown(t *testing.T) {
	t.Parallel()

	payload := renderDiscordToolCallMessage(channel.ToolCallPresentation{
		Emoji:    "💻",
		ToolName: "exec <@123>",
		Status:   channel.ToolCallStatusCompleted,
		Header:   "run [evil](https://evil.test) <@123>",
		Body: []channel.ToolCallBlock{
			{Type: channel.ToolCallBlockText, Title: "stdout <@123>", Text: "ok [evil](https://evil.test) <@123>"},
			{Type: channel.ToolCallBlockLink, Title: "trace] bad", URL: "javascript:alert(1)", Desc: "open [evil](https://evil.test) <@123>"},
			{Type: channel.ToolCallBlockCode, Title: "stderr", Text: "line 1\n```\nline 2"},
		},
		Footer: "exit <@123>",
	})

	if payload.Embed == nil {
		t.Fatal("expected embed")
	}
	for _, text := range []string{
		payload.Embed.Title,
		payload.Embed.Description,
		payload.Embed.Footer.Text,
		payload.Embed.Fields[0].Name,
		payload.Embed.Fields[0].Value,
		payload.Embed.Fields[1].Name,
		payload.Embed.Fields[1].Value,
	} {
		for _, disallowed := range []string{"<@123>", "[evil](", "](javascript:"} {
			if strings.Contains(text, disallowed) {
				t.Fatalf("expected embed text to avoid %q, got %q", disallowed, text)
			}
		}
	}
	codeValue := payload.Embed.Fields[2].Value
	if !strings.HasPrefix(codeValue, "````\n") || !strings.HasSuffix(codeValue, "\n````") {
		t.Fatalf("expected code block to use a longer fence, got %q", codeValue)
	}
}
