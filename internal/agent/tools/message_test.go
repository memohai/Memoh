package tools

import (
	"testing"

	"github.com/memohai/memoh/internal/channel"
)

func TestChannelAttachmentsToToolAttachments_NormalizesLocalPath(t *testing.T) {
	t.Parallel()

	atts := channelAttachmentsToToolAttachments([]channel.Attachment{
		{
			Type: channel.AttachmentImage,
			URL:  "/data/images/demo.png",
			Mime: "IMAGE/PNG; charset=utf-8",
		},
	})
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].Path != "/data/images/demo.png" {
		t.Fatalf("expected local path promoted to Path, got %q", atts[0].Path)
	}
	if atts[0].URL != "" {
		t.Fatalf("expected URL cleared for local path attachment, got %q", atts[0].URL)
	}
	if atts[0].Mime != "image/png" {
		t.Fatalf("expected normalized mime image/png, got %q", atts[0].Mime)
	}
}

func TestChannelAttachmentsToToolAttachments_PreservesRemoteURL(t *testing.T) {
	t.Parallel()

	atts := channelAttachmentsToToolAttachments([]channel.Attachment{
		{
			Type: channel.AttachmentFile,
			URL:  "https://example.com/demo.pdf",
			Name: "demo.pdf",
		},
	})
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].URL != "https://example.com/demo.pdf" {
		t.Fatalf("expected remote URL preserved, got %q", atts[0].URL)
	}
	if atts[0].Path != "" {
		t.Fatalf("expected empty path for remote URL, got %q", atts[0].Path)
	}
	if atts[0].Name != "demo.pdf" {
		t.Fatalf("expected name preserved, got %q", atts[0].Name)
	}
}

func TestChannelAttachmentsToToolAttachments_PreservesInlineBase64(t *testing.T) {
	t.Parallel()

	atts := channelAttachmentsToToolAttachments([]channel.Attachment{
		{
			Type:        channel.AttachmentImage,
			Base64:      "data:image/png;base64,AAAA",
			PlatformKey: "native-ref",
			Mime:        "image/png",
		},
	})
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	if atts[0].Base64 != "data:image/png;base64,AAAA" {
		t.Fatalf("expected inline base64 preserved, got %q", atts[0].Base64)
	}
	if atts[0].PlatformKey != "native-ref" {
		t.Fatalf("expected platform key preserved, got %q", atts[0].PlatformKey)
	}
	if atts[0].URL != "" || atts[0].Path != "" {
		t.Fatalf("expected no path/url for inline attachment, got path=%q url=%q", atts[0].Path, atts[0].URL)
	}
}
