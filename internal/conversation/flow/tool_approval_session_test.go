package flow

import (
	"testing"

	"github.com/memohai/memoh/internal/toolapproval"
)

func TestIsInteractiveApprovalSession(t *testing.T) {
	t.Parallel()

	for _, sessionType := range []string{"", "chat", "CHAT"} {
		if !isInteractiveApprovalSession(sessionType) {
			t.Fatalf("expected %q to allow interactive approvals", sessionType)
		}
	}

	for _, sessionType := range []string{"discuss", "schedule", "heartbeat", "subagent"} {
		if isInteractiveApprovalSession(sessionType) {
			t.Fatalf("expected %q to reject interactive approvals", sessionType)
		}
	}
}

func TestApprovalResultMetadata(t *testing.T) {
	t.Parallel()

	got := approvalResultMetadata(toolapproval.Request{
		ShortID:    7,
		Status:     toolapproval.StatusRejected,
		ToolName:   "exec",
		ToolCallID: "call-1",
	})

	if got["short_id"] != 7 ||
		got["status"] != toolapproval.StatusRejected ||
		got["tool_name"] != "exec" ||
		got["tool_call_id"] != "call-1" {
		t.Fatalf("unexpected metadata: %#v", got)
	}
}
