package orchestration

import (
	"testing"
	"time"
)

func TestNewAttemptFence(t *testing.T) {
	leaseExpiresAt := time.Now().UTC().Add(30 * time.Second)
	attempt := TaskAttempt{
		ID:             "attempt-1",
		RunID:          "run-1",
		TaskID:         "task-1",
		WorkerID:       "worker-1",
		ExecutorID:     "executor-1",
		ClaimEpoch:     7,
		ClaimToken:     "claim-token",
		LeaseExpiresAt: &leaseExpiresAt,
	}

	fence := NewAttemptFence(attempt)
	if fence.AttemptID != attempt.ID || fence.RunID != attempt.RunID || fence.TaskID != attempt.TaskID {
		t.Fatalf("attempt fence identity = %#v, want attempt/run/task ids from source", fence)
	}
	if fence.WorkerID != attempt.WorkerID || fence.ExecutorID != attempt.ExecutorID {
		t.Fatalf("attempt fence worker binding = %#v, want source worker binding", fence)
	}
	if fence.ClaimEpoch != attempt.ClaimEpoch || fence.ClaimToken != attempt.ClaimToken {
		t.Fatalf("attempt fence claim = (%d, %q), want (%d, %q)", fence.ClaimEpoch, fence.ClaimToken, attempt.ClaimEpoch, attempt.ClaimToken)
	}
	if fence.LeaseExpiresAt != attempt.LeaseExpiresAt {
		t.Fatalf("attempt fence lease expiry pointer = %p, want %p", fence.LeaseExpiresAt, attempt.LeaseExpiresAt)
	}

	heartbeat := fence.Heartbeat(15)
	if heartbeat.AttemptID != attempt.ID || heartbeat.ClaimToken != attempt.ClaimToken || heartbeat.LeaseTTLSeconds != 15 {
		t.Fatalf("attempt heartbeat = %#v, want attempt id, claim token, and ttl from fence", heartbeat)
	}

	completion := fence.Completion(AttemptCompletion{
		AttemptID:  "stale-attempt",
		ClaimToken: "stale-token",
		Status:     TaskAttemptStatusCompleted,
		Summary:    "done",
	})
	if completion.AttemptID != attempt.ID || completion.ClaimToken != attempt.ClaimToken {
		t.Fatalf("attempt completion binding = (%q, %q), want (%q, %q)", completion.AttemptID, completion.ClaimToken, attempt.ID, attempt.ClaimToken)
	}
	if completion.Status != TaskAttemptStatusCompleted || completion.Summary != "done" {
		t.Fatalf("attempt completion result fields = (%q, %q), want preserved result fields", completion.Status, completion.Summary)
	}
}

func TestNewVerificationFence(t *testing.T) {
	leaseExpiresAt := time.Now().UTC().Add(30 * time.Second)
	verification := TaskVerification{
		ID:              "verification-1",
		RunID:           "run-1",
		TaskID:          "task-1",
		ResultID:        "result-1",
		WorkerID:        "worker-1",
		ExecutorID:      "executor-1",
		VerifierProfile: "llm.verifier",
		ClaimEpoch:      9,
		ClaimToken:      "claim-token",
		LeaseExpiresAt:  &leaseExpiresAt,
	}

	fence := NewVerificationFence(verification)
	if fence.VerificationID != verification.ID || fence.RunID != verification.RunID || fence.TaskID != verification.TaskID || fence.ResultID != verification.ResultID {
		t.Fatalf("verification fence identity = %#v, want verification/run/task/result ids from source", fence)
	}
	if fence.WorkerID != verification.WorkerID || fence.ExecutorID != verification.ExecutorID || fence.VerifierProfile != verification.VerifierProfile {
		t.Fatalf("verification fence worker binding = %#v, want source worker binding", fence)
	}
	if fence.ClaimEpoch != verification.ClaimEpoch || fence.ClaimToken != verification.ClaimToken {
		t.Fatalf("verification fence claim = (%d, %q), want (%d, %q)", fence.ClaimEpoch, fence.ClaimToken, verification.ClaimEpoch, verification.ClaimToken)
	}
	if fence.LeaseExpiresAt != verification.LeaseExpiresAt {
		t.Fatalf("verification fence lease expiry pointer = %p, want %p", fence.LeaseExpiresAt, verification.LeaseExpiresAt)
	}

	heartbeat := fence.Heartbeat(20)
	if heartbeat.VerificationID != verification.ID || heartbeat.ClaimToken != verification.ClaimToken || heartbeat.LeaseTTLSeconds != 20 {
		t.Fatalf("verification heartbeat = %#v, want verification id, claim token, and ttl from fence", heartbeat)
	}

	completion := fence.Completion(VerificationCompletion{
		VerificationID: "stale-verification",
		ClaimToken:     "stale-token",
		Status:         TaskVerificationStatusCompleted,
		Verdict:        VerificationVerdictAccepted,
		Summary:        "accepted",
	})
	if completion.VerificationID != verification.ID || completion.ClaimToken != verification.ClaimToken {
		t.Fatalf("verification completion binding = (%q, %q), want (%q, %q)", completion.VerificationID, completion.ClaimToken, verification.ID, verification.ClaimToken)
	}
	if completion.Status != TaskVerificationStatusCompleted || completion.Verdict != VerificationVerdictAccepted || completion.Summary != "accepted" {
		t.Fatalf("verification completion result fields = (%q, %q, %q), want preserved result fields", completion.Status, completion.Verdict, completion.Summary)
	}
}
