package avatar

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/storage/providers/localfs"
)

var testPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
}

func TestServiceStoresAndOpensInlineAvatar(t *testing.T) {
	t.Parallel()

	service := NewService(media.NewService(nil, localfs.New(t.TempDir())))
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(testPNG)
	stored, err := service.StoreDataURL(context.Background(), dataURL)
	if err != nil {
		t.Fatalf("StoreDataURL() error = %v", err)
	}
	if !media.IsContentHash(stored.ContentHash) || stored.Mime != "image/png" {
		t.Fatalf("unexpected stored avatar: %#v", stored)
	}
	if stored.URL != "/avatars/"+stored.ContentHash {
		t.Fatalf("stored URL = %q", stored.URL)
	}

	reader, asset, err := service.Open(context.Background(), stored.ContentHash)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = reader.Close() }()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read avatar: %v", err)
	}
	if string(got) != string(testPNG) || asset.Namespace != StorageNamespace {
		t.Fatalf("unexpected avatar round trip: asset=%#v data=%x", asset, got)
	}
}

func TestServicePassesExternalURLThrough(t *testing.T) {
	t.Parallel()

	service := NewService(nil)
	got, handled, err := service.StoreIfInline(context.Background(), " https://example.test/avatar.png ")
	if err != nil || handled || got != "https://example.test/avatar.png" {
		t.Fatalf("StoreIfInline() = %q, %v, %v", got, handled, err)
	}
}

func TestServiceRejectsInvalidAndOversizedAvatar(t *testing.T) {
	t.Parallel()

	service := NewService(media.NewService(nil, localfs.New(t.TempDir())))
	if _, err := service.StoreDataURL(context.Background(), "data:text/plain;base64,aGVsbG8="); !errors.Is(err, ErrInvalidImage) {
		t.Fatalf("invalid image error = %v", err)
	}

	tooLarge := strings.Repeat("a", int(MaxBytes)+1)
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte(tooLarge))
	if _, err := service.StoreDataURL(context.Background(), dataURL); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized image error = %v", err)
	}
}
