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

func TestDiscordDescriptorAdvertisesRichText(t *testing.T) {
	t.Parallel()

	if !(&DiscordAdapter{}).Descriptor().Capabilities.RichText {
		t.Fatal("Discord descriptor must advertise rich text so Message.Parts reaches the Discord renderer")
	}
}

func TestDiscordDescriptorAdvertisesURLButtonsOnly(t *testing.T) {
	t.Parallel()

	caps := (&DiscordAdapter{}).Descriptor().Capabilities
	if caps.Buttons {
		t.Fatal("Discord descriptor must not advertise callback button support")
	}
	if !caps.URLButtons {
		t.Fatal("Discord descriptor must advertise URL button support")
	}
}

func TestMimeExtension(t *testing.T) {
	tests := []struct {
		mime string
		want string
	}{
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"video/mp4", ".mp4"},
		{"audio/mpeg", ".mp3"},
		{"application/pdf", ".pdf"},
		{"unknown/type", ""},
	}

	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			got := mimeExtension(tt.mime)
			if got != tt.want {
				t.Errorf("mimeExtension(%q) = %q, want %q", tt.mime, got, tt.want)
			}
		})
	}
}

func TestDiscordSendUsesPartsRenderer(t *testing.T) {
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
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"msg-1","channel_id":"ch-1"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	err = sendDiscordMessage(context.Background(), session, "ch-1", channel.PreparedOutboundMessage{
		Message: channel.PreparedMessage{
			Message: channel.Message{
				Format: channel.MessageFormatRich,
				Parts: []channel.MessagePart{
					{Type: channel.MessagePartText, Text: "Hello", Styles: []channel.MessageTextStyle{channel.MessageStyleBold}},
					{Type: channel.MessagePartLink, Text: "docs", URL: "https://example.test/a"},
					{Type: channel.MessagePartMention, Text: "@alice", ChannelIdentityID: "1234567890"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("sendDiscordMessage: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(sentBody), &payload); err != nil {
		t.Fatalf("decode sent body: %v (body=%q)", err, sentBody)
	}
	want := "**Hello**\n\n[docs](https://example.test/a)\n\n<@1234567890>"
	if payload["content"] != want {
		t.Fatalf("Discord rich content mismatch\n  got:  %q\n  want: %q", payload["content"], want)
	}
}

func TestDiscordSendRendersURLActionsAsComponents(t *testing.T) {
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
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"id":"msg-1","channel_id":"ch-1"}`)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}, nil
		}),
	}

	err = sendDiscordMessage(context.Background(), session, "ch-1", channel.PreparedOutboundMessage{
		Message: channel.PreparedMessage{
			Message: channel.Message{
				Text: "Read this",
				Actions: []channel.Action{
					{Label: "Open docs", URL: "https://example.test/docs"},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("sendDiscordMessage: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(sentBody), &payload); err != nil {
		t.Fatalf("decode sent body: %v (body=%q)", err, sentBody)
	}
	components, ok := payload["components"].([]any)
	if !ok || len(components) != 1 {
		t.Fatalf("expected one component row, got %#v", payload["components"])
	}
	row, ok := components[0].(map[string]any)
	if !ok {
		t.Fatalf("expected component row object, got %#v", components[0])
	}
	buttons, ok := row["components"].([]any)
	if !ok || len(buttons) != 1 {
		t.Fatalf("expected one button, got %#v", row["components"])
	}
	button, ok := buttons[0].(map[string]any)
	if !ok {
		t.Fatalf("expected button object, got %#v", buttons[0])
	}
	if button["label"] != "Open docs" || button["url"] != "https://example.test/docs" || button["style"] != float64(discordgo.LinkButton) {
		t.Fatalf("unexpected button payload: %#v", button)
	}
}

func TestDiscordURLActionComponentsRejectsMoreThanPlatformLimit(t *testing.T) {
	t.Parallel()

	actions := make([]channel.Action, 26)
	for i := range actions {
		actions[i] = channel.Action{
			Label: "Open docs",
			URL:   "https://example.test/docs",
		}
	}

	_, err := discordURLActionComponents(actions)
	if err == nil || !strings.Contains(err.Error(), "at most 25") {
		t.Fatalf("expected platform limit error, got %v", err)
	}
}

func TestDiscordPreparedAttachmentToFile(t *testing.T) {
	file, err := discordPreparedAttachmentToFile(context.Background(), channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentFile},
		Kind:    channel.PreparedAttachmentUpload,
		Name:    "hello.txt",
		Open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("Hello")), nil
		},
	})
	if err != nil {
		t.Fatalf("discordPreparedAttachmentToFile() error = %v", err)
	}
	data, err := io.ReadAll(file.Reader)
	if err != nil {
		t.Fatalf("read prepared file: %v", err)
	}
	if string(data) != "Hello" {
		t.Errorf("prepared attachment data = %q, want %q", string(data), "Hello")
	}
	_, err = discordPreparedAttachmentToFile(context.Background(), channel.PreparedAttachment{
		Logical: channel.Attachment{Type: channel.AttachmentFile},
		Kind:    channel.PreparedAttachmentPublicURL,
	})
	if err == nil {
		t.Error("discordPreparedAttachmentToFile() expected error for non-upload kind")
	}
}
