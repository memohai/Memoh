package containerd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfSource_InvalidArgument(t *testing.T) {
	if _, err := ResolveConfSource(""); err != ErrInvalidArgument {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestResolveConfSource_FallbackCreatesReadableFile(t *testing.T) {
	dataDir := t.TempDir()

	path, err := ResolveConfSource(dataDir)
	if err != nil {
		t.Fatalf("ResolveConfSource returned error: %v", err)
	}

	if path != filepath.Join(dataDir, "resolv.conf") {
		t.Fatalf("expected fallback path, got %q", path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read fallback resolv.conf: %v", err)
	}
	if string(content) != fallbackResolv {
		t.Fatalf("unexpected fallback resolv.conf contents: %q", string(content))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat fallback resolv.conf: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fallbackResolvPerm {
		t.Fatalf("expected permissions %o, got %o", fallbackResolvPerm, perm)
	}
}

func TestResolveConfSource_FallbackFixesExistingPermissions(t *testing.T) {
	dataDir := t.TempDir()
	fallbackPath := filepath.Join(dataDir, "resolv.conf")
	if err := os.WriteFile(fallbackPath, []byte(fallbackResolv), 0o600); err != nil {
		t.Fatalf("failed to seed fallback resolv.conf: %v", err)
	}

	path, err := ResolveConfSource(dataDir)
	if err != nil {
		t.Fatalf("ResolveConfSource returned error: %v", err)
	}
	if path != fallbackPath {
		t.Fatalf("expected existing fallback path, got %q", path)
	}

	info, err := os.Stat(fallbackPath)
	if err != nil {
		t.Fatalf("failed to stat fallback resolv.conf: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fallbackResolvPerm {
		t.Fatalf("expected permissions %o, got %o", fallbackResolvPerm, perm)
	}
}
