package compaction

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRunCompactionFailureCooldownSkipsImmediateRetry(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	cfg := machineryConfig(&stubModel{}, 450)
	fail := &failingModel{}
	cfg.HTTPClient = &http.Client{Transport: fail}

	if err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
		t.Fatal("first attempt must run and fail")
	}
	if fail.calls != 1 {
		t.Fatalf("calls = %d, want 1", fail.calls)
	}

	if err := svc.RunCompactionSync(context.Background(), cfg); err != nil {
		t.Fatalf("cooldown skip must not surface an error: %v", err)
	}
	if fail.calls != 1 {
		t.Fatalf("immediate retry within cooldown must be skipped, calls=%d", fail.calls)
	}

	now = now.Add(compactionFailureCooldown + time.Second)
	if err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
		t.Fatal("attempt after cooldown must run and fail again")
	}
	if fail.calls != 2 {
		t.Fatalf("attempt after cooldown should run, calls=%d", fail.calls)
	}
}

func TestRunCompactionManualRequestBypassesFailureCooldown(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	autoCfg := machineryConfig(&stubModel{}, 450)
	autoCfg.HTTPClient = &http.Client{Transport: &failingModel{}}
	if err := svc.RunCompactionSync(context.Background(), autoCfg); err == nil {
		t.Fatal("first automatic attempt must run and fail, arming the cooldown")
	}

	// The user fixes the model and presses compact within the cooldown window.
	// A manual request must actually run (not be skipped and reported as done):
	// it compacts and reports a real result instead of a false success.
	manualStub := &stubModel{summary: "recovered by manual run"}
	manualCfg := autoCfg
	manualCfg.Manual = true
	manualCfg.HTTPClient = &http.Client{Transport: manualStub}
	if err := svc.RunCompactionSync(context.Background(), manualCfg); err != nil {
		t.Fatalf("manual compaction must run despite cooldown: %v", err)
	}
	if manualStub.calls != 1 {
		t.Fatalf("manual request must call the model, not skip on cooldown (calls=%d)", manualStub.calls)
	}
	if !q.created || len(q.markedIDs) == 0 || q.finalizeCalls != 1 {
		t.Fatalf("manual run must do real work: created=%v finalized=%d calls=%d", q.created, len(q.markedIDs), q.finalizeCalls)
	}

	// An automatic request in the same window still respects the cooldown: the
	// manual success above cleared it, so this one runs — proving cooldown is a
	// shared per-session state that manual participates in, not a bypass leak.
	autoRetry := &failingModel{}
	autoRetryCfg := autoCfg
	autoRetryCfg.HTTPClient = &http.Client{Transport: autoRetry}
	if err := svc.RunCompactionSync(context.Background(), autoRetryCfg); err == nil {
		t.Fatal("automatic retry after a successful manual run should proceed and fail")
	}
	if autoRetry.calls != 1 {
		t.Fatalf("manual success must clear the shared cooldown for automatic runs too (calls=%d)", autoRetry.calls)
	}
}

func TestRunCompactionManualFailureStillSurfacesError(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	autoCfg := machineryConfig(&stubModel{}, 450)
	autoCfg.HTTPClient = &http.Client{Transport: &failingModel{}}
	if err := svc.RunCompactionSync(context.Background(), autoCfg); err == nil {
		t.Fatal("automatic attempt must fail to arm the cooldown")
	}

	// A manual request that also fails must surface the real error, never a
	// silent nil that callers render as "done".
	manualFail := &failingModel{}
	manualCfg := autoCfg
	manualCfg.Manual = true
	manualCfg.HTTPClient = &http.Client{Transport: manualFail}
	if err := svc.RunCompactionSync(context.Background(), manualCfg); err == nil {
		t.Fatal("a failing manual compaction must return an error, not a false success")
	}
	if manualFail.calls != 1 {
		t.Fatalf("manual request must attempt the model despite cooldown (calls=%d)", manualFail.calls)
	}
}

func TestRunCompactionFailureCooldownClearsOnSuccess(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	sessionCfg := machineryConfig(&stubModel{}, 450)

	failCfg := sessionCfg
	failCfg.HTTPClient = &http.Client{Transport: &failingModel{}}
	if err := svc.RunCompactionSync(context.Background(), failCfg); err == nil {
		t.Fatal("expected initial failure")
	}

	now = now.Add(compactionFailureCooldown + time.Second)
	successStub := &stubModel{summary: "recovered"}
	successCfg := sessionCfg
	successCfg.HTTPClient = &http.Client{Transport: successStub}
	if err := svc.RunCompactionSync(context.Background(), successCfg); err != nil {
		t.Fatalf("attempt after cooldown should succeed: %v", err)
	}
	if successStub.calls != 1 {
		t.Fatalf("success attempt should have called the model once, got %d", successStub.calls)
	}

	retryFail := &failingModel{}
	retryCfg := sessionCfg
	retryCfg.HTTPClient = &http.Client{Transport: retryFail}
	if err := svc.RunCompactionSync(context.Background(), retryCfg); err == nil {
		t.Fatal("expected failure from immediate retry model")
	}
	if retryFail.calls != 1 {
		t.Fatalf("success must have cleared the cooldown, allowing an immediate retry; calls=%d", retryFail.calls)
	}
}

func TestRunCompactionAttemptReportsSessionOwner(t *testing.T) {
	rows := machineryCorpus(t)
	q := &fakeQueries{uncompacted: rows}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)

	cfg := machineryConfig(stub, 450)
	if !svc.beginSessionCompaction(cfg.SessionID) {
		t.Fatal("first acquisition must succeed")
	}
	defer svc.endSessionCompaction(cfg.SessionID)

	result, err := svc.runCompaction(context.Background(), cfg)
	if err != nil {
		t.Fatalf("in-flight skip must not error: %v", err)
	}
	if result.Status != StatusNoop || result.inflightDone == nil {
		t.Fatalf("in-flight result = %#v, want noop with owner completion signal", result)
	}
	if stub.calls != 0 || q.created || len(q.markedIDs) != 0 {
		t.Fatalf("in-flight session must skip entirely (calls=%d created=%v marked=%d)", stub.calls, q.created, len(q.markedIDs))
	}
}

func TestRunCompactionSyncWaitsForSessionOwnerUntilContextCanceled(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)
	cfg := machineryConfig(stub, 450)
	if !svc.beginSessionCompaction(cfg.SessionID) {
		t.Fatal("first acquisition must succeed")
	}
	defer svc.endSessionCompaction(cfg.SessionID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := svc.RunCompactionSync(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunCompactionSync() error = %v, want context canceled", err)
	}
	if _, err := svc.RunCompactionSyncResult(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunCompactionSyncResult() error = %v, want context canceled", err)
	}
	if stub.calls != 0 || q.created || len(q.markedIDs) != 0 {
		t.Fatalf("canceled waiter ran compaction (calls=%d created=%v marked=%d)", stub.calls, q.created, len(q.markedIDs))
	}
}
