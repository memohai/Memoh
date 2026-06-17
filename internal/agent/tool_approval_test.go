package agent

import (
	"testing"

	sdk "github.com/memohai/twilight-ai/sdk"
)

func TestMarkApprovalToolsCoversAllTools(t *testing.T) {
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
		if !tool.RequireApproval {
			t.Fatalf("%s RequireApproval = false, want true", tool.Name)
		}
	}
}
