package agent

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestMarkApprovalToolsCoversConfiguredOperations(t *testing.T) {
	t.Parallel()

	tools := markApprovalTools([]sdk.Tool{
		{Name: "read"},
		{Name: "list"},
		{Name: "write"},
		{Name: "edit"},
		{Name: "apply_patch"},
		{Name: "exec"},
		{Name: "web_search"},
	})

	for _, tool := range tools {
		switch tool.Name {
		case "read", "list", "write", "edit", "apply_patch", "exec":
			if !tool.RequireApproval {
				t.Fatalf("%s RequireApproval = false, want true", tool.Name)
			}
		default:
			if tool.RequireApproval {
				t.Fatalf("%s RequireApproval = true, want false", tool.Name)
			}
		}
	}
}
