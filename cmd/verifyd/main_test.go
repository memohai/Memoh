package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/orchestration"
)

type fakeVerificationRuntime struct {
	heartbeatErrs  []error
	completions    []orchestration.VerificationCompletion
	completeErrs   []error
	heartbeatCalls int
}

func (f *fakeVerificationRuntime) HeartbeatVerification(_ context.Context, input orchestration.VerificationHeartbeat) (*orchestration.TaskVerification, error) {
	f.heartbeatCalls++
	if len(f.heartbeatErrs) > 0 {
		err := f.heartbeatErrs[0]
		f.heartbeatErrs = f.heartbeatErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &orchestration.TaskVerification{ID: input.VerificationID, ClaimToken: input.ClaimToken}, nil
}

func (f *fakeVerificationRuntime) CompleteVerification(_ context.Context, input orchestration.VerificationCompletion) (*orchestration.TaskVerification, error) {
	f.completions = append(f.completions, input)
	if len(f.completeErrs) > 0 {
		err := f.completeErrs[0]
		f.completeErrs = f.completeErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &orchestration.TaskVerification{ID: input.VerificationID, ClaimToken: input.ClaimToken, Status: input.Status}, nil
}

func TestRunVerificationRewritesSuccessfulCompletionOnShutdown(t *testing.T) {
	runtime := &fakeVerificationRuntime{}
	log := slog.New(slog.DiscardHandler)
	parentCtx, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	leaseLost := runVerificationWithInterval(parentCtx, runtime, log, orchestration.TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, 30, time.Hour, []string{orchestration.DefaultVerifierProfile}, func(_ context.Context, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
		return orchestration.VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         orchestration.TaskVerificationStatusCompleted,
			Verdict:        orchestration.VerificationVerdictAccepted,
			Summary:        "passed",
		}
	})
	if leaseLost {
		t.Fatal("runVerificationWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(runtime.completions))
	}
	completion := runtime.completions[0]
	if completion.Status != orchestration.TaskVerificationStatusFailed {
		t.Fatalf("completion status = %q, want %q", completion.Status, orchestration.TaskVerificationStatusFailed)
	}
	if completion.Verdict != orchestration.VerificationVerdictRejected {
		t.Fatalf("completion verdict = %q, want %q", completion.Verdict, orchestration.VerificationVerdictRejected)
	}
	if completion.FailureClass != "worker_shutdown" {
		t.Fatalf("completion failure_class = %q, want %q", completion.FailureClass, "worker_shutdown")
	}
	if completion.Summary != "worker shutdown interrupted verification" {
		t.Fatalf("completion summary = %q, want %q", completion.Summary, "worker shutdown interrupted verification")
	}
	if completion.TerminalReason != "worker shutdown interrupted verification" {
		t.Fatalf("completion terminal_reason = %q, want %q", completion.TerminalReason, "worker shutdown interrupted verification")
	}
}

func TestRunVerificationDropsStaleCompletionAfterLeaseConflict(t *testing.T) {
	runtime := &fakeVerificationRuntime{
		heartbeatErrs: []error{orchestration.ErrVerificationLeaseConflict},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runVerificationWithInterval(context.Background(), runtime, log, orchestration.TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, []string{orchestration.DefaultVerifierProfile}, func(ctx context.Context, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
		<-ctx.Done()
		return orchestration.VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         orchestration.TaskVerificationStatusCompleted,
			Verdict:        orchestration.VerificationVerdictAccepted,
			Summary:        "passed",
		}
	})
	if !leaseLost {
		t.Fatal("runVerificationWithInterval() leaseLost = false, want true")
	}
	if len(runtime.completions) != 0 {
		t.Fatalf("completion count = %d, want 0", len(runtime.completions))
	}
}

func TestRunVerificationRetriesTransientCompletionFailures(t *testing.T) {
	runtime := &fakeVerificationRuntime{
		completeErrs: []error{errors.New("transient"), nil},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runVerificationWithInterval(context.Background(), runtime, log, orchestration.TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, 30, time.Hour, []string{orchestration.DefaultVerifierProfile}, func(_ context.Context, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
		return orchestration.VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         orchestration.TaskVerificationStatusCompleted,
			Verdict:        orchestration.VerificationVerdictAccepted,
			Summary:        "passed",
		}
	})
	if leaseLost {
		t.Fatal("runVerificationWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 2 {
		t.Fatalf("completion count = %d, want 2", len(runtime.completions))
	}
}

func TestRunVerificationTreatsCompletionLeaseConflictAsLeaseLoss(t *testing.T) {
	runtime := &fakeVerificationRuntime{
		completeErrs: []error{orchestration.ErrVerificationLeaseConflict},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runVerificationWithInterval(context.Background(), runtime, log, orchestration.TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, 30, time.Hour, []string{orchestration.DefaultVerifierProfile}, func(_ context.Context, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
		return orchestration.VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         orchestration.TaskVerificationStatusCompleted,
			Verdict:        orchestration.VerificationVerdictAccepted,
			Summary:        "passed",
		}
	})
	if !leaseLost {
		t.Fatal("runVerificationWithInterval() leaseLost = false, want true")
	}
	if len(runtime.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(runtime.completions))
	}
}

func TestRunVerificationTreatsCompletionImmutableAsNonLeaseLoss(t *testing.T) {
	runtime := &fakeVerificationRuntime{
		completeErrs: []error{orchestration.ErrVerificationImmutable},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runVerificationWithInterval(context.Background(), runtime, log, orchestration.TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, 30, time.Hour, []string{orchestration.DefaultVerifierProfile}, func(_ context.Context, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
		return orchestration.VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         orchestration.TaskVerificationStatusCompleted,
			Verdict:        orchestration.VerificationVerdictAccepted,
			Summary:        "passed",
		}
	})
	if leaseLost {
		t.Fatal("runVerificationWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(runtime.completions))
	}
}

func TestRunVerificationCancelsImmediatelyOnImmutableHeartbeat(t *testing.T) {
	runtime := &fakeVerificationRuntime{
		heartbeatErrs: []error{orchestration.ErrVerificationImmutable},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runVerificationWithInterval(context.Background(), runtime, log, orchestration.TaskVerification{
		ID:         "verification-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, []string{orchestration.DefaultVerifierProfile}, func(ctx context.Context, verification orchestration.TaskVerification, _ []string) orchestration.VerificationCompletion {
		<-ctx.Done()
		return orchestration.VerificationCompletion{
			VerificationID: verification.ID,
			ClaimToken:     verification.ClaimToken,
			Status:         orchestration.TaskVerificationStatusCompleted,
			Verdict:        orchestration.VerificationVerdictAccepted,
			Summary:        "passed",
		}
	})
	if leaseLost {
		t.Fatal("runVerificationWithInterval() leaseLost = true, want false")
	}
	if runtime.heartbeatCalls != 1 {
		t.Fatalf("heartbeat call count = %d, want 1", runtime.heartbeatCalls)
	}
	if len(runtime.completions) != 0 {
		t.Fatalf("completion count = %d, want 0", len(runtime.completions))
	}
}
