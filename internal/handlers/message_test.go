package handlers

import (
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

func TestDecodeAttachmentBase64(t *testing.T) {
	t.Parallel()

	data := []byte("hello")
	encoded := base64.StdEncoding.EncodeToString(data)
	decoded, err := decodeAttachmentBase64(encoded, 16)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := io.ReadAll(decoded)
	if err != nil {
		t.Fatalf("read decoded failed: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("unexpected decoded value: %q", string(got))
	}
}

func TestDecodeAttachmentBase64DataURL(t *testing.T) {
	t.Parallel()

	encoded := "data:text/plain;base64," + base64.StdEncoding.EncodeToString([]byte("payload"))
	decoded, err := decodeAttachmentBase64(encoded, 32)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, err := io.ReadAll(decoded)
	if err != nil {
		t.Fatalf("read decoded failed: %v", err)
	}
	if string(got) != "payload" {
		t.Fatalf("unexpected decoded value: %q", string(got))
	}
}

func TestNormalizeBase64DataURL(t *testing.T) {
	t.Parallel()

	raw := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 4)))
	got := normalizeBase64DataURL(raw, "image/png")
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("expected data url prefix, got %q", got)
	}
	existing := "data:text/plain;base64,AAA="
	if normalizeBase64DataURL(existing, "image/png") != existing {
		t.Fatalf("expected existing data url unchanged")
	}
}
