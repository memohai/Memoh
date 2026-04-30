package orchestration

import "context"

func NewAttemptFence(attempt TaskAttempt) AttemptFence {
	return AttemptFence{
		AttemptID:      attempt.ID,
		RunID:          attempt.RunID,
		TaskID:         attempt.TaskID,
		WorkerID:       attempt.WorkerID,
		ExecutorID:     attempt.ExecutorID,
		ClaimEpoch:     attempt.ClaimEpoch,
		ClaimToken:     attempt.ClaimToken,
		LeaseExpiresAt: attempt.LeaseExpiresAt,
	}
}

func (f AttemptFence) Heartbeat(leaseTTLSeconds int) AttemptHeartbeat {
	return AttemptHeartbeat{
		AttemptID:       f.AttemptID,
		ClaimToken:      f.ClaimToken,
		LeaseTTLSeconds: leaseTTLSeconds,
	}
}

func (f AttemptFence) Completion(completion AttemptCompletion) AttemptCompletion {
	completion.AttemptID = f.AttemptID
	completion.ClaimToken = f.ClaimToken
	return completion
}

func NewVerificationFence(verification TaskVerification) VerificationFence {
	return VerificationFence{
		VerificationID:  verification.ID,
		RunID:           verification.RunID,
		TaskID:          verification.TaskID,
		ResultID:        verification.ResultID,
		WorkerID:        verification.WorkerID,
		ExecutorID:      verification.ExecutorID,
		VerifierProfile: verification.VerifierProfile,
		ClaimEpoch:      verification.ClaimEpoch,
		ClaimToken:      verification.ClaimToken,
		LeaseExpiresAt:  verification.LeaseExpiresAt,
	}
}

func (f VerificationFence) Heartbeat(leaseTTLSeconds int) VerificationHeartbeat {
	return VerificationHeartbeat{
		VerificationID:  f.VerificationID,
		ClaimToken:      f.ClaimToken,
		LeaseTTLSeconds: leaseTTLSeconds,
	}
}

func (f VerificationFence) Completion(completion VerificationCompletion) VerificationCompletion {
	completion.VerificationID = f.VerificationID
	completion.ClaimToken = f.ClaimToken
	return completion
}

// WorkerLeaseRuntime owns worker process registration and liveness fencing.
// Executors must keep this lease alive before claiming or advancing work.
type WorkerLeaseRuntime interface {
	RegisterWorker(context.Context, WorkerRegistration) (*WorkerLease, error)
	HeartbeatWorker(context.Context, string, string, int) (*WorkerLease, error)
}

// ClaimedAttemptRuntime is the narrow contract a worker uses after an attempt
// has been claimed. All calls must carry the current claim token.
type ClaimedAttemptRuntime interface {
	HeartbeatAttempt(context.Context, AttemptHeartbeat) (*TaskAttempt, error)
	CompleteAttempt(context.Context, AttemptCompletion) (*TaskAttempt, error)
}

// AttemptExecutor is the control contract for task attempt execution. The
// service implementation currently backs the builtin workerd executor, but
// callers should depend on this boundary rather than the full Service.
type AttemptExecutor interface {
	ClaimNextAttempt(context.Context, AttemptClaim) (*TaskAttempt, error)
	StartClaimedAttempt(context.Context, AttemptFence) (*TaskAttempt, error)
	ClaimedAttemptRuntime
}

// ClaimedVerificationRuntime is the verifier equivalent of
// ClaimedAttemptRuntime.
type ClaimedVerificationRuntime interface {
	HeartbeatVerification(context.Context, VerificationHeartbeat) (*TaskVerification, error)
	CompleteVerification(context.Context, VerificationCompletion) (*TaskVerification, error)
}

// TaskDispatcher advances ready tasks into created attempts. Callers that know
// worker capacity can pass normalized worker profiles to avoid dispatching work
// that cannot currently be claimed.
type TaskDispatcher interface {
	DispatchNextReadyTaskForWorkerProfiles(context.Context, []string) (bool, error)
	DispatchNextReadyTaskForActiveWorkers(context.Context) (bool, error)
}

// VerificationWorkExecutor is the control contract for verifier execution.
type VerificationWorkExecutor interface {
	ClaimNextVerification(context.Context, VerificationClaim) (*TaskVerification, error)
	StartClaimedVerification(context.Context, VerificationFence) (*TaskVerification, error)
	ClaimedVerificationRuntime
}

var (
	_ WorkerLeaseRuntime         = (*Service)(nil)
	_ AttemptExecutor            = (*Service)(nil)
	_ ClaimedAttemptRuntime      = (*Service)(nil)
	_ TaskDispatcher             = (*Service)(nil)
	_ VerificationWorkExecutor   = (*Service)(nil)
	_ ClaimedVerificationRuntime = (*Service)(nil)
)
