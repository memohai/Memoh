package toolapproval

import (
	"testing"

	"github.com/memohai/memoh/internal/settings"
)

func TestNeedsApprovalFileBypass(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if needsApproval(cfg, "write", map[string]any{"path": "/data/tmp/output.txt"}) {
		t.Fatal("expected tmp path to bypass write approval")
	}
	if !needsApproval(cfg, "edit", map[string]any{"path": "/data/src/main.go"}) {
		t.Fatal("expected non-bypassed edit path to require approval")
	}
}

func TestNeedsApprovalExecBypass(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if needsApproval(cfg, "exec", map[string]any{"command": "npm test"}) {
		t.Fatal("expected single npm command to bypass exec approval")
	}
	if !needsApproval(cfg, "exec", map[string]any{"command": "npm test && rm -rf /data"}) {
		t.Fatal("expected compound command to require approval")
	}
}
