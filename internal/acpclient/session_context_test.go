package acpclient

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSessionContextRejectsUnknownBackend(t *testing.T) {
	_, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeAPIKey,
		BotID:         "bot-1",
		Backend:       "remote",
		WorkspaceRoot: "/data",
		LocalDataRoot: "/tmp/memoh",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported workspace backend") {
		t.Fatalf("ResolveSessionContext() error = %v, want unsupported backend", err)
	}
}

func TestResolveSessionContextHermesManagedHome(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "project"), 0o750); err != nil {
		t.Fatal(err)
	}
	localDataRoot := t.TempDir()

	local, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeAPIKey,
		BotID:         "bot-1",
		Backend:       "local",
		WorkspaceRoot: root,
		ProjectPath:   "/data/project",
		LocalDataRoot: localDataRoot,
	})
	if err != nil {
		t.Fatalf("ResolveSessionContext(local Hermes) error = %v", err)
	}
	if want := filepath.Join(localDataRoot, "acp", "hermes", "bot-1"); local.HermesHome != want {
		t.Fatalf("local HermesHome = %q, want %q", local.HermesHome, want)
	}
	rootEval, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if local.CWD != filepath.Join(rootEval, "project") {
		t.Fatalf("local CWD = %q, want project path under root", local.CWD)
	}

	container, err := ResolveSessionContext(SessionContextInput{
		AgentID:     "hermes",
		SetupMode:   SetupModeAPIKey,
		BotID:       "bot-1",
		Backend:     "container",
		ProjectPath: "/data/project",
	})
	if err != nil {
		t.Fatalf("ResolveSessionContext(container Hermes) error = %v", err)
	}
	if container.HermesHome != HermesContainerHome {
		t.Fatalf("container HermesHome = %q, want %q", container.HermesHome, HermesContainerHome)
	}
}

func TestResolveSessionContextHermesSelfDoesNotSetManagedHome(t *testing.T) {
	root := t.TempDir()
	resolved, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeSelf,
		BotID:         "bot-1",
		Backend:       "local",
		WorkspaceRoot: root,
		ProjectPath:   "/data",
		LocalDataRoot: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("ResolveSessionContext(self Hermes) error = %v", err)
	}
	if resolved.HermesHome != "" {
		t.Fatalf("self HermesHome = %q, want empty", resolved.HermesHome)
	}
}

func TestResolveSessionContextLocalHermesManagedRequiresIsolationInputs(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeAPIKey,
		BotID:         "bot-1",
		Backend:       "local",
		WorkspaceRoot: root,
		ProjectPath:   "/data",
	}); err == nil || !strings.Contains(err.Error(), "local data root") {
		t.Fatalf("ResolveSessionContext() error = %v, want local data root error", err)
	}
	if _, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeAPIKey,
		Backend:       "local",
		WorkspaceRoot: root,
		ProjectPath:   "/data",
		LocalDataRoot: t.TempDir(),
	}); err == nil || !strings.Contains(err.Error(), "bot id") {
		t.Fatalf("ResolveSessionContext() error = %v, want bot id error", err)
	}
}

func TestResolveSessionContextLocalHermesBotIDIsPathSafe(t *testing.T) {
	root := t.TempDir()
	localDataRoot := t.TempDir()
	resolved, err := ResolveSessionContext(SessionContextInput{
		AgentID:       "hermes",
		SetupMode:     SetupModeAPIKey,
		BotID:         "../x:y",
		Backend:       "local",
		WorkspaceRoot: root,
		ProjectPath:   "/data",
		LocalDataRoot: localDataRoot,
	})
	if err != nil {
		t.Fatalf("ResolveSessionContext() error = %v", err)
	}
	wantPrefix := filepath.Join(localDataRoot, "acp", "hermes")
	rel, err := filepath.Rel(wantPrefix, resolved.HermesHome)
	if err != nil {
		t.Fatalf("relative HermesHome: %v", err)
	}
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) || strings.Contains(rel, string(filepath.Separator)) {
		t.Fatalf("HermesHome = %q escapes bot-scoped directory %q", resolved.HermesHome, wantPrefix)
	}
}
