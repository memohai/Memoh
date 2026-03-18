package containerd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfSource_InvalidArgument(t *testing.T) {
	if _, err := ResolveConfSource(""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestResolveConfSource_UsesPreferredResolvedWhenAvailable(t *testing.T) {
	dataDir := t.TempDir()
	preferredPath := filepath.Join(dataDir, "preferred-resolv.conf")
	if err := os.WriteFile(preferredPath, []byte("nameserver 9.9.9.9\n"), 0o600); err != nil {
		t.Fatalf("failed to seed preferred resolv.conf: %v", err)
	}

	path, err := resolveConfSource(dataDir, preferredPath)
	if err != nil {
		t.Fatalf("resolveConfSource returned error: %v", err)
	}
	if path != preferredPath {
		t.Fatalf("expected preferred path, got %q", path)
	}
}

func TestResolveConfSource_UsesSystemdResolvedWhenAvailable(t *testing.T) {
	if _, err := os.Stat(systemdResolvConf); errors.Is(err, os.ErrNotExist) {
		t.Skip("systemd-resolved config not available on this host")
	} else if err != nil {
		t.Fatalf("failed to stat %s: %v", systemdResolvConf, err)
	}

	path, err := ResolveConfSource(t.TempDir())
	if err != nil {
		t.Fatalf("ResolveConfSource returned error: %v", err)
	}
	if path != systemdResolvConf {
		t.Fatalf("expected systemd-resolved path, got %q", path)
	}
}

func TestResolveConfSource_FallbackCreatesReadableFile(t *testing.T) {
	dataDir := t.TempDir()
	preferredPath := filepath.Join(dataDir, "missing-preferred-resolv.conf")

	path, err := resolveConfSource(dataDir, preferredPath)
	if err != nil {
		t.Fatalf("resolveConfSource returned error: %v", err)
	}

	if path != filepath.Join(dataDir, "resolv.conf") {
		t.Fatalf("expected fallback path, got %q", path)
	}

	//nolint:gosec // test reads a file it just created in a temp directory
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

	path, err := resolveConfSource(dataDir, filepath.Join(dataDir, "missing-preferred-resolv.conf"))
	if err != nil {
		t.Fatalf("resolveConfSource returned error: %v", err)
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
