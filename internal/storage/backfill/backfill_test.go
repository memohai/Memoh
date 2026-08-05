package backfill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/memohai/memoh/internal/storage/providers/localfs"
)

func TestRunnerCopiesVerifiesAndResumesFilesystemBackfill(t *testing.T) {
	t.Parallel()

	payload := []byte("legacy image")
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	key := filepath.Join("bot-1", hash[:2], hash+".png")
	sourceRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(sourceRoot, key)), 0o750); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, key), payload, 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "ignored.txt"), []byte("not content addressed"), 0o600); err != nil {
		t.Fatalf("write ignored source: %v", err)
	}

	target := localfs.New(t.TempDir())
	runner, err := New(target, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	source := NewFilesystemSource("host", sourceRoot)

	result, err := runner.Run(context.Background(), source)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Copied != 1 || result.Ignored != 1 || result.Failed != 0 {
		t.Fatalf("first result = %#v", result)
	}

	result, err = runner.Run(context.Background(), source)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if result.Existing != 1 || result.Copied != 0 || result.Ignored != 1 {
		t.Fatalf("second result = %#v", result)
	}
}
