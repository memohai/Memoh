package sessionruntime

import "testing"

func TestResolvedDecisionFromCommand(t *testing.T) {
	t.Parallel()

	userInput := ResolvedDecisionFromCommand("user_input", "input-1", CommandUserInputResponse, []byte(`{"Canceled":true}`))
	if userInput == nil || userInput.Kind != "user_input" || userInput.ID != "input-1" || userInput.Status != "canceled" {
		t.Fatalf("user input decision = %#v", userInput)
	}

	approval := ResolvedDecisionFromCommand("tool_approval", "approval-1", CommandToolApprovalResponse, []byte(`{"decision":"approve"}`))
	if approval == nil || approval.Kind != "tool_approval" || approval.ID != "approval-1" || approval.Status != "approved" {
		t.Fatalf("approval decision = %#v", approval)
	}

	if got := ResolvedDecisionFromCommand("tool_approval", "approval-1", CommandToolApprovalResponse, []byte(`{"decision":"maybe"}`)); got != nil {
		t.Fatalf("invalid approval decision = %#v", got)
	}
}

func TestNormalizeResolvedDecisionRejectsMismatchedStatus(t *testing.T) {
	t.Parallel()

	if _, err := normalizeResolvedDecision(&ResolvedDecisionView{Kind: "user_input", ID: "input-1", Status: "approved"}); err == nil {
		t.Fatal("expected mismatched user_input status to fail")
	}
	if _, err := normalizeResolvedDecision(&ResolvedDecisionView{Kind: "tool_approval", ID: "approval-1", Status: "submitted"}); err == nil {
		t.Fatal("expected mismatched tool_approval status to fail")
	}
}
