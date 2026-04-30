package main

import (
	"context"
	"errors"
	"log/slog"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/orchestration"
)

type fakeAttemptRuntime struct {
	mu                sync.Mutex
	heartbeatErrs     []error
	heartbeatCalls    int
	completions       []orchestration.AttemptCompletion
	completeErrs      []error
	completeCallback  func(orchestration.AttemptCompletion, int) error
	heartbeatCallback func()
}

func (f *fakeAttemptRuntime) HeartbeatAttempt(_ context.Context, input orchestration.AttemptHeartbeat) (*orchestration.TaskAttempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.heartbeatCalls++
	if f.heartbeatCallback != nil {
		f.heartbeatCallback()
	}
	if len(f.heartbeatErrs) == 0 {
		return &orchestration.TaskAttempt{ID: input.AttemptID, ClaimToken: input.ClaimToken}, nil
	}
	err := f.heartbeatErrs[0]
	f.heartbeatErrs = f.heartbeatErrs[1:]
	if err != nil {
		return nil, err
	}
	return &orchestration.TaskAttempt{ID: input.AttemptID, ClaimToken: input.ClaimToken}, nil
}

func (f *fakeAttemptRuntime) CompleteAttempt(_ context.Context, input orchestration.AttemptCompletion) (*orchestration.TaskAttempt, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.completions = append(f.completions, input)
	if f.completeCallback != nil {
		if err := f.completeCallback(input, len(f.completions)); err != nil {
			return nil, err
		}
	}
	if len(f.completeErrs) > 0 {
		err := f.completeErrs[0]
		f.completeErrs = f.completeErrs[1:]
		if err != nil {
			return nil, err
		}
	}
	return &orchestration.TaskAttempt{ID: input.AttemptID, ClaimToken: input.ClaimToken, Status: input.Status}, nil
}

func TestRunAttemptHeartbeatLoopCancelsAfterRepeatedFailures(t *testing.T) {
	runtime := &fakeAttemptRuntime{
		heartbeatErrs: []error{
			errors.New("transient"),
			errors.New("transient"),
			errors.New("transient"),
		},
	}
	log := slog.New(slog.DiscardHandler)
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	execCtx, cancelExec := context.WithCancel(context.Background())
	defer cancelExec()
	done := make(chan bool, 1)

	go runAttemptHeartbeatLoopWithInterval(ctx, cancelExec, runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, done)

	select {
	case leaseLost := <-done:
		if !leaseLost {
			t.Fatal("runAttemptHeartbeatLoopWithInterval() leaseLost = false, want true")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runAttemptHeartbeatLoopWithInterval() timed out")
	}

	select {
	case <-execCtx.Done():
	default:
		t.Fatal("execution context was not cancelled after repeated heartbeat failures")
	}
}

func TestRunAttemptRewritesSuccessfulCompletionOnShutdown(t *testing.T) {
	runtime := &fakeAttemptRuntime{}
	log := slog.New(slog.DiscardHandler)
	parentCtx, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	leaseLost := runAttemptWithInterval(parentCtx, runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Hour, []string{"root"}, func(_ context.Context, attempt orchestration.TaskAttempt, _ []string) orchestration.AttemptCompletion {
		return orchestration.AttemptCompletion{
			AttemptID:    attempt.ID,
			ClaimToken:   attempt.ClaimToken,
			Status:       orchestration.TaskAttemptStatusCompleted,
			Summary:      "done",
			FailureClass: "",
		}
	})
	if leaseLost {
		t.Fatal("runAttemptWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(runtime.completions))
	}
	completion := runtime.completions[0]
	if completion.Status != orchestration.TaskAttemptStatusFailed {
		t.Fatalf("completion status = %q, want %q", completion.Status, orchestration.TaskAttemptStatusFailed)
	}
	if completion.FailureClass != "worker_shutdown" {
		t.Fatalf("completion failure_class = %q, want %q", completion.FailureClass, "worker_shutdown")
	}
}

func TestRunAttemptRewritesCancelledExecutionFailureOnShutdown(t *testing.T) {
	runtime := &fakeAttemptRuntime{}
	log := slog.New(slog.DiscardHandler)
	parentCtx, cancelParent := context.WithCancel(context.Background())
	cancelParent()

	leaseLost := runAttemptWithInterval(parentCtx, runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Hour, []string{"root"}, func(execCtx context.Context, attempt orchestration.TaskAttempt, _ []string) orchestration.AttemptCompletion {
		<-execCtx.Done()
		return orchestration.AttemptCompletion{
			AttemptID:      attempt.ID,
			ClaimToken:     attempt.ClaimToken,
			Status:         orchestration.TaskAttemptStatusFailed,
			Summary:        "task lookup failed",
			FailureClass:   "task_lookup_failed",
			TerminalReason: execCtx.Err().Error(),
		}
	})
	if leaseLost {
		t.Fatal("runAttemptWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 1 {
		t.Fatalf("completion count = %d, want 1", len(runtime.completions))
	}
	completion := runtime.completions[0]
	if completion.Status != orchestration.TaskAttemptStatusFailed {
		t.Fatalf("completion status = %q, want %q", completion.Status, orchestration.TaskAttemptStatusFailed)
	}
	if completion.FailureClass != "worker_shutdown" {
		t.Fatalf("completion failure_class = %q, want %q", completion.FailureClass, "worker_shutdown")
	}
	if completion.TerminalReason != "worker shutdown interrupted attempt" {
		t.Fatalf("completion terminal_reason = %q, want worker shutdown interrupted attempt", completion.TerminalReason)
	}
}

func TestRunAttemptDropsStaleCompletionAfterLeaseConflict(t *testing.T) {
	runtime := &fakeAttemptRuntime{
		heartbeatErrs: []error{orchestration.ErrAttemptLeaseConflict},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runAttemptWithInterval(context.Background(), runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, []string{"root"}, func(ctx context.Context, attempt orchestration.TaskAttempt, _ []string) orchestration.AttemptCompletion {
		<-ctx.Done()
		return orchestration.AttemptCompletion{
			AttemptID:  attempt.ID,
			ClaimToken: attempt.ClaimToken,
			Status:     orchestration.TaskAttemptStatusCompleted,
			Summary:    "done",
		}
	})
	if !leaseLost {
		t.Fatal("runAttemptWithInterval() leaseLost = false, want true")
	}
	if len(runtime.completions) != 0 {
		t.Fatalf("completion count = %d, want 0", len(runtime.completions))
	}
}

func TestRunAttemptDropsCancelledExecutionFailureAfterLeaseConflict(t *testing.T) {
	runtime := &fakeAttemptRuntime{
		heartbeatErrs: []error{orchestration.ErrAttemptLeaseConflict},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runAttemptWithInterval(context.Background(), runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, []string{"root"}, func(execCtx context.Context, attempt orchestration.TaskAttempt, _ []string) orchestration.AttemptCompletion {
		<-execCtx.Done()
		return orchestration.AttemptCompletion{
			AttemptID:      attempt.ID,
			ClaimToken:     attempt.ClaimToken,
			Status:         orchestration.TaskAttemptStatusFailed,
			Summary:        "task lookup failed",
			FailureClass:   "task_lookup_failed",
			TerminalReason: execCtx.Err().Error(),
		}
	})
	if !leaseLost {
		t.Fatal("runAttemptWithInterval() leaseLost = false, want true")
	}
	if len(runtime.completions) != 0 {
		t.Fatalf("completion count = %d, want 0", len(runtime.completions))
	}
}

func TestRunAttemptRetriesTransientCompletionFailuresWhileHeartbeatContinues(t *testing.T) {
	runtime := &fakeAttemptRuntime{
		completeErrs: []error{
			errors.New("transient complete failure"),
			nil,
		},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runAttemptWithInterval(context.Background(), runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, []string{"root"}, func(_ context.Context, attempt orchestration.TaskAttempt, _ []string) orchestration.AttemptCompletion {
		return orchestration.AttemptCompletion{
			AttemptID:  attempt.ID,
			ClaimToken: attempt.ClaimToken,
			Status:     orchestration.TaskAttemptStatusCompleted,
			Summary:    "done",
		}
	})
	if leaseLost {
		t.Fatal("runAttemptWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 2 {
		t.Fatalf("completion count = %d, want 2", len(runtime.completions))
	}
	if runtime.heartbeatCalls == 0 {
		t.Fatal("heartbeatCalls = 0, want heartbeat loop to keep renewing lease during completion retry")
	}
}

func TestRunAttemptRetriesAfterCompletionAckLossAndConverges(t *testing.T) {
	var committed bool
	runtime := &fakeAttemptRuntime{
		completeCallback: func(_ orchestration.AttemptCompletion, call int) error {
			if call == 1 {
				committed = true
				return errors.New("completion ack lost after commit")
			}
			if !committed {
				t.Fatal("second completion arrived before simulated commit")
			}
			return nil
		},
	}
	log := slog.New(slog.DiscardHandler)

	leaseLost := runAttemptWithInterval(context.Background(), runtime, log, orchestration.TaskAttempt{
		ID:         "attempt-1",
		ClaimToken: "claim-1",
	}, 30, time.Millisecond, []string{"root"}, func(_ context.Context, attempt orchestration.TaskAttempt, _ []string) orchestration.AttemptCompletion {
		return orchestration.AttemptCompletion{
			AttemptID:  attempt.ID,
			ClaimToken: attempt.ClaimToken,
			Status:     orchestration.TaskAttemptStatusCompleted,
			Summary:    "done",
		}
	})
	if leaseLost {
		t.Fatal("runAttemptWithInterval() leaseLost = true, want false")
	}
	if len(runtime.completions) != 2 {
		t.Fatalf("completion count = %d, want 2", len(runtime.completions))
	}
	if !reflect.DeepEqual(runtime.completions[0], runtime.completions[1]) {
		t.Fatalf("replayed completion mismatch: first=%+v second=%+v", runtime.completions[0], runtime.completions[1])
	}
}
