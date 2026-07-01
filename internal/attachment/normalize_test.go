package attachment

import (
	"io"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/media"
)

func TestResolveMime(t *testing.T) {
	cases := []struct {
		name      string
		mediaType media.MediaType
		source    string
		sniffed   string
		want      string
	}{
		{
			name:      "image: sniffed preferred over wrong platform mime",
			mediaType: media.MediaTypeImage,
			source:    "image/png",
			sniffed:   "image/jpeg",
			want:      "image/jpeg",
		},
		{
			name:      "image: fallback to source when sniff fails",
			mediaType: media.MediaTypeImage,
			source:    "image/png",
			sniffed:   "",
			want:      "image/png",
		},
		{
			name:      "image: both empty returns octet-stream",
			mediaType: media.MediaTypeImage,
			source:    "",
			sniffed:   "",
			want:      "application/octet-stream",
		},
		{
			name:      "file: source preferred over sniffed",
			mediaType: media.MediaTypeFile,
			source:    "application/pdf",
			sniffed:   "application/octet-stream",
			want:      "application/pdf",
		},
		{
			name:      "file: sniffed used when source is generic",
			mediaType: media.MediaTypeFile,
			source:    "application/octet-stream",
			sniffed:   "application/pdf",
			want:      "application/pdf",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveMime(tc.mediaType, tc.source, tc.sniffed)
			if got != tc.want {
				t.Fatalf("ResolveMime(%v, %q, %q) = %q, want %q",
					tc.mediaType, tc.source, tc.sniffed, got, tc.want)
			}
		})
	}
}

func TestPrepareReaderAndMime(t *testing.T) {
	reader, mime, err := PrepareReaderAndMime(strings.NewReader("\x89PNG\r\n\x1a\npayload"), media.MediaTypeImage, "")
	if err != nil {
		t.Fatalf("PrepareReaderAndMime returned error: %v", err)
	}
	if mime != "image/png" {
		t.Fatalf("PrepareReaderAndMime mime = %q, want image/png", mime)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read prepared reader failed: %v", err)
	}
	if !strings.HasPrefix(string(raw), "\x89PNG\r\n\x1a\n") {
		t.Fatalf("prepared reader lost prefix bytes")
	}
}

func TestDecodeBase64(t *testing.T) {
	reader, err := DecodeBase64("aGVsbG8=", 1024)
	if err != nil {
		t.Fatalf("DecodeBase64 returned error: %v", err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decoded bytes failed: %v", err)
	}
	if string(raw) != "hello" {
		t.Fatalf("decoded content = %q, want hello", string(raw))
	}

	reader, err = DecodeBase64("data:text/plain;base64,aGVsbG8=", 1024)
	if err != nil {
		t.Fatalf("DecodeBase64 with data URL returned error: %v", err)
	}
	raw, err = io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decoded data URL bytes failed: %v", err)
	}
	if string(raw) != "hello" {
		t.Fatalf("decoded data URL content = %q, want hello", string(raw))
	}

	_, err = DecodeBase64("", 1024)
	if err == nil {
		t.Fatalf("expected empty base64 to return error")
	}
}
