package acpclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

func TestCodexACPLiveLocal(t *testing.T) {
	if os.Getenv("MEMOH_LIVE_CODEX_ACP") != "1" {
		t.Skip("set MEMOH_LIVE_CODEX_ACP=1 to run the live Codex ACP local smoke test")
	}

	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Memoh ACP live smoke\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(nil, testWorkspace{
		client: newTestBridgeClient(t, root),
		info: bridge.WorkspaceInfo{
			Backend:        bridge.WorkspaceBackendLocal,
			DefaultWorkDir: root,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := runner.Run(ctx, RunRequest{
		BotID:       "bot-live",
		Task:        "Reply with exactly this text and do not modify files: memoh-acp-live-ok",
		ProjectPath: "/data/project",
		Command:     "npx",
		Args: []string{
			"-y",
			"@agentclientprotocol/codex-acp@1.1.4",
		},
		Timeout: 90 * time.Second,
	})
	if err != nil {
		skipIfExternalCodexLimit(t, err)
		t.Fatalf("live Codex ACP run failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(result.Text), "memoh-acp-live-ok") {
		t.Fatalf("live Codex ACP text = %q, want marker memoh-acp-live-ok", result.Text)
	}
}

func skipIfExternalCodexLimit(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "usage_limit_exceeded") ||
		strings.Contains(message, "you've hit your usage limit") ||
		strings.Contains(message, "insufficient_quota") {
		t.Skipf("Codex live smoke skipped because the external Codex account/API quota is unavailable: %v", err)
	}
}
