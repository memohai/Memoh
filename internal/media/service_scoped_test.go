package media

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/memohai/memoh/internal/storage/providers/localfs"
)

func TestServiceScopedAssetRoundTrip(t *testing.T) {
	t.Parallel()

	service := NewService(nil, localfs.New(t.TempDir()))
	payload := []byte("scoped-avatar-bytes")
	asset, err := service.IngestScoped(context.Background(), "avatars", ScopedIngestInput{
		Mime:     "image/png",
		Reader:   bytes.NewReader(payload),
		MaxBytes: int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("IngestScoped() error = %v", err)
	}
	if asset.Namespace != "avatars" || asset.BotID != "" || !IsContentHash(asset.ContentHash) {
		t.Fatalf("unexpected scoped asset: %#v", asset)
	}

	reader, opened, err := service.OpenScoped(context.Background(), "avatars", asset.ContentHash)
	if err != nil {
		t.Fatalf("OpenScoped() error = %v", err)
	}
	defer func() { _ = reader.Close() }()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read scoped asset: %v", err)
	}
	if !bytes.Equal(got, payload) || opened.Namespace != "avatars" {
		t.Fatalf("unexpected scoped round trip: asset=%#v payload=%q", opened, got)
	}
}

func TestServiceScopedAssetRejectsTraversal(t *testing.T) {
	t.Parallel()

	service := NewService(nil, localfs.New(t.TempDir()))
	_, err := service.IngestScoped(context.Background(), "../avatars", ScopedIngestInput{
		Reader: bytes.NewReader([]byte("x")),
	})
	if !errors.Is(err, ErrPathTraversal) {
		t.Fatalf("IngestScoped() error = %v, want ErrPathTraversal", err)
	}
}
