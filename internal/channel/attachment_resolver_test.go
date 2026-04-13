package channel

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/memohai/memoh/internal/media"
)

func TestReadAttachmentBodyStreamsPayload(t *testing.T) {
	t.Parallel()

	src := bytes.Repeat([]byte("a"), 2048)
	head, body, sniffed, err := readAttachmentBody(io.NopCloser(bytes.NewReader(src)), media.MaxAssetBytes)
	if err != nil {
		t.Fatalf("readAttachmentBody: %v", err)
	}
	defer func() { _ = body.Close() }()

	if len(head) != attachmentSniffBytes {
		t.Fatalf("expected %d sniff bytes, got %d", attachmentSniffBytes, len(head))
	}
	if sniffed != http.DetectContentType(head) {
		t.Fatalf("unexpected sniffed mime: %q", sniffed)
	}

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("streamed payload did not match source")
	}
}

func TestReadAttachmentBodyReturnsTooLargeDuringStream(t *testing.T) {
	t.Parallel()

	src := bytes.Repeat([]byte("b"), 600)
	_, body, _, err := readAttachmentBody(io.NopCloser(bytes.NewReader(src)), 550)
	if err != nil {
		t.Fatalf("readAttachmentBody: %v", err)
	}
	defer func() { _ = body.Close() }()

	got, err := io.ReadAll(body)
	if !errors.Is(err, media.ErrAssetTooLarge) {
		t.Fatalf("expected ErrAssetTooLarge, got %v", err)
	}
	if len(got) != 550 {
		t.Fatalf("expected 550 bytes before limit error, got %d", len(got))
	}
}
