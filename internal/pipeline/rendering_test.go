package pipeline

import (
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestRenderMessage_ImageRefsPopulated(t *testing.T) {
	msg := &ICMessage{
		MessageID:    "msg-1",
		ReceivedAtMs: 100,
		TimestampSec: 100,
		Content:      []ContentNode{{Type: "text", Text: "photo"}},
		Attachments: []Attachment{
			{Type: "image", ContentHash: "hash-1", MimeType: "image/jpeg", FilePath: "/data/media/bot/ab/hash-1.jpg"},
			{Type: "file", ContentHash: "hash-2", MimeType: "application/pdf", FilePath: "/data/media/bot/cd/hash-2.pdf"},
			{Type: "image", MimeType: "image/png"},
		},
		Conversation: ConversationMeta{Channel: "telegram", ConversationType: "private"},
	}

	seg := renderMessage(msg, RenderParams{})

	if len(seg.ImageRefs) != 1 {
		t.Fatalf("expected 1 image ref (only images with ContentHash), got %d", len(seg.ImageRefs))
	}
	if seg.ImageRefs[0].ContentHash != "hash-1" {
		t.Fatalf("expected hash-1, got %q", seg.ImageRefs[0].ContentHash)
	}
	if seg.ImageRefs[0].Mime != "image/jpeg" {
		t.Fatalf("expected image/jpeg, got %q", seg.ImageRefs[0].Mime)
	}
}

func TestRenderMessage_NoImageRefs(t *testing.T) {
	msg := &ICMessage{
		MessageID:    "msg-2",
		ReceivedAtMs: 200,
		TimestampSec: 200,
		Content:      []ContentNode{{Type: "text", Text: "text only"}},
		Conversation: ConversationMeta{Channel: "telegram", ConversationType: "private"},
	}

	seg := renderMessage(msg, RenderParams{})

	if len(seg.ImageRefs) != 0 {
		t.Fatalf("expected 0 image refs, got %d", len(seg.ImageRefs))
	}
}

func TestAdaptAttachments_ContentHash(t *testing.T) {
	atts := []channel.Attachment{
		{Type: channel.AttachmentImage, ContentHash: "abc123", URL: "/data/media/bot/ab/abc123.jpg", Mime: "image/jpeg"},
		{Type: channel.AttachmentFile, URL: "https://example.com/doc.pdf", Mime: "application/pdf"},
	}
	got := adaptAttachments(atts)
	if len(got) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(got))
	}
	if got[0].ContentHash != "abc123" || got[0].MimeType != "image/jpeg" {
		t.Fatalf("unexpected first attachment: %+v", got[0])
	}
	if got[0].FilePath != "/data/media/bot/ab/abc123.jpg" {
		t.Fatalf("expected FilePath from URL, got %q", got[0].FilePath)
	}
	if got[1].Type != "file" || got[1].MimeType != "application/pdf" {
		t.Fatalf("unexpected second attachment: %+v", got[1])
	}
	if got[1].FilePath != "https://example.com/doc.pdf" {
		t.Fatalf("expected FilePath from URL, got %q", got[1].FilePath)
	}
}
