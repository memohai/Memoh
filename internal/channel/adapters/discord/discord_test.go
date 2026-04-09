package discord

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

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
