package orchestration

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/sqlc"
)

func TestNormalizeVerificationPolicyRejectsInvalidMode(t *testing.T) {
	t.Parallel()

	if _, err := normalizeVerificationPolicy(map[string]any{"mode": "strict"}); err == nil {
		t.Fatal("normalizeVerificationPolicy(invalid mode) error = nil, want non-nil")
	}
}

func TestEvaluateBuiltinVerificationFailsClosedWithoutReplacementPlan(t *testing.T) {
	t.Parallel()

	completion := evaluateBuiltinVerification(TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, map[string]any{
		"on_reject":                 VerificationRejectActionReplan,
		"require_structured_output": true,
	}, map[string]any{}, nil)

	if completion.Status != TaskVerificationStatusFailed {
		t.Fatalf("completion status = %q, want %q", completion.Status, TaskVerificationStatusFailed)
	}
	if completion.RequestReplan {
		t.Fatal("completion request_replan = true, want false")
	}
}

func TestEvaluateBuiltinVerificationPassesWithRequiredArtifactsAndOutput(t *testing.T) {
	t.Parallel()

	completion := evaluateBuiltinVerification(TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, map[string]any{
		"require_structured_output": true,
		"required_artifact_kinds":   []any{"report"},
	}, map[string]any{
		"summary": "done",
	}, []sqlc.OrchestrationArtifact{
		{Kind: "report"},
	})

	if completion.Status != TaskVerificationStatusCompleted {
		t.Fatalf("completion status = %q, want %q", completion.Status, TaskVerificationStatusCompleted)
	}
	if completion.Verdict != VerificationVerdictAccepted {
		t.Fatalf("completion verdict = %q, want %q", completion.Verdict, VerificationVerdictAccepted)
	}
}

func TestEvaluateBuiltinVerificationRequestsReplanWhenPolicyAllowsIt(t *testing.T) {
	t.Parallel()

	completion := evaluateBuiltinVerification(TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, map[string]any{
		"require_structured_output": true,
		"required_artifact_kinds":   []any{"report"},
		"on_reject":                 VerificationRejectActionReplan,
	}, map[string]any{
		"child_tasks": []any{
			map[string]any{
				"goal":           "replacement child",
				"worker_profile": DefaultRootWorkerProfile,
			},
		},
	}, nil)

	if completion.Status != TaskVerificationStatusCompleted {
		t.Fatalf("completion status = %q, want %q", completion.Status, TaskVerificationStatusCompleted)
	}
	if completion.Verdict != VerificationVerdictRejected {
		t.Fatalf("completion verdict = %q, want %q", completion.Verdict, VerificationVerdictRejected)
	}
	if completion.FailureClass != "verification_requested_replan" {
		t.Fatalf("completion failure_class = %q, want %q", completion.FailureClass, "verification_requested_replan")
	}
	if !completion.RequestReplan {
		t.Fatal("completion request_replan = false, want true")
	}
	if completion.Summary != "artifact kind \"report\" is required" {
		t.Fatalf("completion summary = %q, want %q", completion.Summary, "artifact kind \"report\" is required")
	}
	if completion.TerminalReason != "artifact kind \"report\" is required" {
		t.Fatalf("completion terminal_reason = %q, want %q", completion.TerminalReason, "artifact kind \"report\" is required")
	}
}

func TestArtifactsForResultAttemptFiltersPreviousAttempts(t *testing.T) {
	t.Parallel()

	currentAttempt := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	previousAttempt := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	filtered := artifactsForResultAttempt([]sqlc.OrchestrationArtifact{
		{Kind: "report", AttemptID: previousAttempt},
		{Kind: "report", AttemptID: currentAttempt},
	}, currentAttempt)

	if len(filtered) != 1 {
		t.Fatalf("filtered artifact count = %d, want 1", len(filtered))
	}
	if filtered[0].AttemptID != currentAttempt {
		t.Fatalf("filtered artifact attempt_id = %#v, want current attempt", filtered[0].AttemptID)
	}
}
