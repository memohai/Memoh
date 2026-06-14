package toolapproval

import (
	"testing"

	"github.com/memohai/memoh/internal/settings"
)

func TestNeedsApprovalFileBypass(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if needsApproval(cfg, "write", map[string]any{"path": "/data/tmp/output.txt"}) {
		t.Fatal("expected /data path to bypass write approval")
	}
	if needsApproval(cfg, "write", map[string]any{"path": "daily.md"}) {
		t.Fatal("expected relative /data path to bypass write approval")
	}
	if needsApproval(cfg, "edit", map[string]any{"path": "/tmp/output.txt"}) {
		t.Fatal("expected /tmp path to bypass edit approval")
	}
	if !needsApproval(cfg, "edit", map[string]any{"path": "/etc/passwd"}) {
		t.Fatal("expected non-bypassed edit path to require approval")
	}
}

func TestNeedsApprovalForceReviewOverridesBypass(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true
	cfg.Write.ForceReviewGlobs = []string{"/data/secret/**"}

	if !needsApproval(cfg, "write", map[string]any{"path": "/data/secret/token.txt"}) {
		t.Fatal("expected force-review path to require approval even under /data")
	}
}

func TestNeedsApprovalApplyPatchUsesFileApproval(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if !needsApproval(cfg, "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** End Patch"}) {
		t.Fatal("expected apply_patch to require approval when file approvals are enabled")
	}

	cfg.Write.RequireApproval = false
	cfg.Edit.RequireApproval = false
	if needsApproval(cfg, "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** End Patch"}) {
		t.Fatal("expected apply_patch to skip approval when file approvals are disabled")
	}

	cfg.Write.ForceReviewGlobs = []string{"/data/secret/**"}
	if !needsApproval(cfg, "apply_patch", map[string]any{"patch": "*** Begin Patch\n*** End Patch"}) {
		t.Fatal("expected apply_patch to require approval when force-review file globs exist")
	}
}

func TestNeedsApprovalExecDefaultsToAllowed(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true

	if needsApproval(cfg, "exec", map[string]any{"command": "npm test"}) {
		t.Fatal("expected exec to be allowed by default")
	}
	if needsApproval(cfg, "exec", map[string]any{"command": "npm test && rm -rf /data"}) {
		t.Fatal("expected compound exec to be allowed when approval is disabled")
	}
}

func TestNeedsApprovalExecForceReview(t *testing.T) {
	cfg := settings.DefaultToolApprovalConfig()
	cfg.Enabled = true
	cfg.Exec.ForceReviewCommands = []string{"rm"}

	if !needsApproval(cfg, "exec", map[string]any{"command": "rm file.txt"}) {
		t.Fatal("expected force-review command to require approval")
	}
}
