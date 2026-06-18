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
