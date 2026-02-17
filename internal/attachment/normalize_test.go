package attachment

import (
	"io"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/media"
)

func TestMapMediaType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want media.MediaType
	}{
		{name: "image", in: "image", want: media.MediaTypeImage},
		{name: "gif", in: "gif", want: media.MediaTypeImage},
		{name: "audio", in: "audio", want: media.MediaTypeAudio},
		{name: "voice", in: "voice", want: media.MediaTypeAudio},
		{name: "video", in: "video", want: media.MediaTypeVideo},
		{name: "default", in: "file", want: media.MediaTypeFile},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MapMediaType(tc.in)
			if got != tc.want {
				t.Fatalf("MapMediaType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeBase64DataURL(t *testing.T) {
	got := NormalizeBase64DataURL("AAAA", "image/png")
	if got != "data:image/png;base64,AAAA" {
		t.Fatalf("unexpected normalized value: %q", got)
	}

	already := "data:image/jpeg;base64,BBBB"
	if NormalizeBase64DataURL(already, "image/png") != already {
		t.Fatalf("expected data url to pass through")
	}
}

func TestNormalizeMime(t *testing.T) {
	got := NormalizeMime("IMAGE/JPEG; charset=utf-8")
	if got != "image/jpeg" {
		t.Fatalf("NormalizeMime unexpected result: %q", got)
	}
}

func TestMimeFromDataURL(t *testing.T) {
	got := MimeFromDataURL("data:image/png;base64,AAAA")
	if got != "image/png" {
		t.Fatalf("MimeFromDataURL unexpected result: %q", got)
	}
	if MimeFromDataURL("https://example.com/demo.png") != "" {
		t.Fatalf("MimeFromDataURL should return empty for non-data-url")
	}
}

func TestResolveMime(t *testing.T) {
	if got := ResolveMime(media.MediaTypeImage, "application/octet-stream", "image/jpeg"); got != "image/jpeg" {
		t.Fatalf("ResolveMime image unexpected result: %q", got)
	}
	if got := ResolveMime(media.MediaTypeFile, "application/octet-stream", "application/pdf"); got != "application/pdf" {
		t.Fatalf("ResolveMime file unexpected result: %q", got)
	}
	if got := ResolveMime(media.MediaTypeImage, "", ""); got != "application/octet-stream" {
		t.Fatalf("ResolveMime empty unexpected result: %q", got)
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
