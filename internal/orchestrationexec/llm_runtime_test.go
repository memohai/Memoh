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

func TestDecodeStartRunPlannerPayloadRequiresChildTasks(t *testing.T) {
	_, err := decodeStartRunPlannerPayload(map[string]any{
		"summary": "single task",
	})
	if err == nil {
		t.Fatal("decodeStartRunPlannerPayload() error = nil, want missing child_tasks error")
	}
}

func TestDecodeStartRunPlannerPayloadRejectsUnknownFields(t *testing.T) {
	_, err := decodeStartRunPlannerPayload(map[string]any{
		"summary":     "split",
		"child_tasks": []any{map[string]any{"goal": "step a", "depends_on_aliases": []any{}}},
	})
	if err == nil {
		t.Fatal("decodeStartRunPlannerPayload() error = nil, want unknown field error")
	}
}

func TestDecodeStartRunPlannerPayloadValidatesChildTasks(t *testing.T) {
	plan, err := decodeStartRunPlannerPayload(map[string]any{
		"summary": "split",
		"child_tasks": []any{
			map[string]any{
				"alias":               "a",
				"kind":                "step",
				"goal":                "step a",
				"inputs":              map[string]any{"x": "1"},
				"depends_on":          []any{},
				"worker_profile":      "llm.default",
				"priority":            float64(2),
				"retry_policy":        map[string]any{},
				"verification_policy": map[string]any{"mode": "builtin.basic"},
				"blackboard_scope":    "run.a",
			},
		},
	})
	if err != nil {
		t.Fatalf("decodeStartRunPlannerPayload() error = %v", err)
	}
	if plan.Summary != "split" || len(plan.ChildTasks) != 1 {
		t.Fatalf("plan = %#v, want summary and one child", plan)
	}
	child := plan.ChildTasks[0]
	if child.Alias != "a" || child.Goal != "step a" || child.Priority != 2 {
		t.Fatalf("child = %#v, want normalized task fields", child)
	}
	if child.VerificationPolicy["mode"] != "builtin.basic" {
		t.Fatalf("verification policy = %#v, want preserved policy", child.VerificationPolicy)
	}
}

func TestDecodeStartRunPlannerPayloadRejectsFractionalPriority(t *testing.T) {
	_, err := decodeStartRunPlannerPayload(map[string]any{
		"summary": "split",
		"child_tasks": []any{
			map[string]any{
				"goal":     "step a",
				"priority": float64(1.5),
			},
		},
	})
	if err == nil {
		t.Fatal("decodeStartRunPlannerPayload() error = nil, want fractional priority error")
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
