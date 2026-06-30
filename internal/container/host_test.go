package container

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfSourceInvalidArgument(t *testing.T) {
	if _, err := ResolveConfSource(""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestResolveConfSourceUsesPreferredResolvedWhenAvailable(t *testing.T) {
	dataDir := t.TempDir()
	preferredPath := filepath.Join(dataDir, "preferred-resolv.conf")
	if err := os.WriteFile(preferredPath, []byte("nameserver 9.9.9.9\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := resolveConfSource(dataDir, preferredPath)
	if err != nil {
		t.Fatalf("resolveConfSource returned error: %v", err)
	}
	if path != preferredPath {
		t.Fatalf("expected preferred path, got %q", path)
	}
}

func TestResolveConfSourceFallbackCreatesReadableFile(t *testing.T) {
	dataDir := t.TempDir()
	path, err := resolveConfSource(dataDir, filepath.Join(dataDir, "missing-resolv.conf"))
	if err != nil {
		t.Fatalf("resolveConfSource returned error: %v", err)
	}
	if path != filepath.Join(dataDir, "resolv.conf") {
		t.Fatalf("expected fallback path, got %q", path)
	}
	content, err := os.ReadFile(path) // #nosec G304 -- test reads a temp file returned by resolveConfSource.
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != fallbackResolv {
		t.Fatalf("unexpected fallback resolv.conf contents: %q", string(content))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fallbackResolvPerm {
		t.Fatalf("expected mode %o, got %o", fallbackResolvPerm, info.Mode().Perm())
	}
}

func TestResolveConfSourceFallbackFixesExistingPermissions(t *testing.T) {
	dataDir := t.TempDir()
	fallbackPath := filepath.Join(dataDir, "resolv.conf")
	if err := os.WriteFile(fallbackPath, []byte(fallbackResolv), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err := resolveConfSource(dataDir, filepath.Join(dataDir, "missing-resolv.conf"))
	if err != nil {
		t.Fatalf("resolveConfSource returned error: %v", err)
	}
	if path != fallbackPath {
		t.Fatalf("expected fallback path, got %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != fallbackResolvPerm {
		t.Fatalf("expected mode %o, got %o", fallbackResolvPerm, info.Mode().Perm())
	}
}

func TestTimezoneSpecWithTZ(t *testing.T) {
	t.Setenv("TZ", "Asia/Shanghai")
	mounts, env := TimezoneSpec()
	if len(mounts) != 0 {
		t.Fatalf("expected no mounts, got %v", mounts)
	}
	if len(env) != 1 || env[0] != "TZ=Asia/Shanghai" {
		t.Fatalf("unexpected env: %v", env)
	}
}
