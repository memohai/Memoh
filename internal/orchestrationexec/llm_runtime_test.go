package orchestrationexec

import (
	"testing"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/orchestration"
)

func TestDecodeJSONObjectTextStripsCodeFence(t *testing.T) {
	payload, err := decodeJSONObjectText("```json\n{\"status\":\"completed\",\"summary\":\"ok\"}\n```")
	if err != nil {
		t.Fatalf("decodeJSONObjectText error = %v", err)
	}
	if got := payload["status"]; got != "completed" {
		t.Fatalf("status = %v, want completed", got)
	}
}

func TestDecodeAttemptCompletionPayloadPromotesTopLevelChildTasks(t *testing.T) {
	completion, err := decodeAttemptCompletionPayload(
		orchestration.TaskAttempt{ID: "attempt-1", ClaimToken: "claim-1"},
		sqlc.OrchestrationTask{Goal: "compute fib"},
		map[string]any{
			"status":         "completed",
			"summary":        "needs decomposition",
			"request_replan": true,
			"child_tasks": []any{
				map[string]any{"goal": "step a"},
			},
		},
	)
	if err != nil {
		t.Fatalf("decodeAttemptCompletionPayload error = %v", err)
	}
	if !completion.RequestReplan {
		t.Fatal("RequestReplan = false, want true")
	}
	childTasks, ok := completion.StructuredOutput["child_tasks"].([]any)
	if !ok || len(childTasks) != 1 {
		t.Fatalf("structured_output.child_tasks = %#v, want 1 item", completion.StructuredOutput["child_tasks"])
	}
}

func TestDecodeVerificationCompletionPayloadDefaultsRejectReason(t *testing.T) {
	completion, err := decodeVerificationCompletionPayload(
		orchestration.TaskVerification{ID: "verification-1", ClaimToken: "claim-1"},
		sqlc.OrchestrationTask{Goal: "verify fib"},
		sqlc.OrchestrationTaskResult{Summary: "worker said fib=1346269"},
		map[string]any{
			"status":  "failed",
			"verdict": "rejected",
			"summary": "result is inconsistent",
		},
	)
	if err != nil {
		t.Fatalf("decodeVerificationCompletionPayload error = %v", err)
	}
	if completion.TerminalReason != "result is inconsistent" {
		t.Fatalf("TerminalReason = %q, want summary fallback", completion.TerminalReason)
	}
}
