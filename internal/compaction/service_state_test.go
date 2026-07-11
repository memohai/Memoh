package compaction

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestRunCompactionSyncReportsScopedNoop(t *testing.T) {
	// An already-compact/fresh session compacts nothing. The result must be a
	// scoped no-op, so a caller (the HTTP endpoint) never has to fall back to an
	// unscoped bot-wide log that could belong to another session.
	q := &fakeQueries{}
	stub := &stubModel{summary: "unused"}
	svc := newMachineryService(q)

	res, err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 100))
	if err != nil {
		t.Fatalf("no-op must not error: %v", err)
	}
	if res.Status != StatusNoop || res.MessageCount != 0 || res.Summary != "" {
		t.Fatalf("no-op result = %+v, want noop/0/empty", res)
	}
	if stub.calls != 0 || q.created {
		t.Fatalf("no-op must not call the model or create a log (calls=%d created=%v)", stub.calls, q.created)
	}
}

func TestRunCompactionSyncReportsScopedSummaryOnSuccess(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	stub := &stubModel{summary: "SUMMARY-OK"}
	svc := newMachineryService(q)

	res, err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 450))
	if err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if res.Status != StatusOK || res.Summary != "SUMMARY-OK" || res.MessageCount != len(q.markedIDs) {
		t.Fatalf("success result = %+v, want ok/SUMMARY-OK/%d", res, len(q.markedIDs))
	}
}

func TestRunCompactionSyncFailureSurfacesError(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	cfg := machineryConfig(&stubModel{}, 450)
	cfg.HTTPClient = &http.Client{Transport: &failingModel{}}

	res, err := svc.RunCompactionSync(context.Background(), cfg)
	if err == nil {
		t.Fatal("a summarizer failure must surface an error, not a stale result")
	}
	if res.Status == StatusOK {
		t.Fatalf("failed result must not report ok: %+v", res)
	}
}

func TestRunCompactionFailureCooldownSkipsImmediateRetry(t *testing.T) {
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	now := time.Now()
	svc.nowFn = func() time.Time { return now }

	cfg := machineryConfig(&stubModel{}, 450)
	fail := &failingModel{}
	cfg.HTTPClient = &http.Client{Transport: fail}

	if _, err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
		t.Fatal("first attempt must run and fail")
	}
	if fail.calls != 1 {
		t.Fatalf("calls = %d, want 1", fail.calls)
	}

	if _, err := svc.RunCompactionSync(context.Background(), cfg); err != nil {
		t.Fatalf("cooldown skip must not surface an error: %v", err)
	}
	if fail.calls != 1 {
		t.Fatalf("immediate retry within cooldown must be skipped, calls=%d", fail.calls)
	}

	now = now.Add(compactionFailureCooldown + time.Second)
	if _, err := svc.RunCompactionSync(context.Background(), cfg); err == nil {
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
	if _, err := svc.RunCompactionSync(context.Background(), autoCfg); err == nil {
		t.Fatal("first automatic attempt must run and fail, arming the cooldown")
	}

	// The user fixes the model and presses compact within the cooldown window.
	// A manual request must actually run (not be skipped and reported as done):
	// it compacts and reports a real result instead of a false success.
	manualStub := &stubModel{summary: "recovered by manual run"}
	manualCfg := autoCfg
	manualCfg.Manual = true
	manualCfg.HTTPClient = &http.Client{Transport: manualStub}
	if _, err := svc.RunCompactionSync(context.Background(), manualCfg); err != nil {
		t.Fatalf("manual compaction must run despite cooldown: %v", err)
	}
	if manualStub.calls != 1 {
		t.Fatalf("manual request must call the model, not skip on cooldown (calls=%d)", manualStub.calls)
	}
	if !q.created || len(q.markedIDs) == 0 || q.completed.Status != "ok" {
		t.Fatalf("manual run must do real work: created=%v marked=%d status=%q", q.created, len(q.markedIDs), q.completed.Status)
	}

	// An automatic request in the same window still respects the cooldown: the
	// manual success above cleared it, so this one runs — proving cooldown is a
	// shared per-session state that manual participates in, not a bypass leak.
	autoRetry := &failingModel{}
	autoRetryCfg := autoCfg
	autoRetryCfg.HTTPClient = &http.Client{Transport: autoRetry}
	if _, err := svc.RunCompactionSync(context.Background(), autoRetryCfg); err == nil {
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
	if _, err := svc.RunCompactionSync(context.Background(), autoCfg); err == nil {
		t.Fatal("automatic attempt must fail to arm the cooldown")
	}

	// A manual request that also fails must surface the real error, never a
	// silent nil that callers render as "done".
	manualFail := &failingModel{}
	manualCfg := autoCfg
	manualCfg.Manual = true
	manualCfg.HTTPClient = &http.Client{Transport: manualFail}
	if _, err := svc.RunCompactionSync(context.Background(), manualCfg); err == nil {
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
	if _, err := svc.RunCompactionSync(context.Background(), failCfg); err == nil {
		t.Fatal("expected initial failure")
	}

	now = now.Add(compactionFailureCooldown + time.Second)
	successStub := &stubModel{summary: "recovered"}
	successCfg := sessionCfg
	successCfg.HTTPClient = &http.Client{Transport: successStub}
	if _, err := svc.RunCompactionSync(context.Background(), successCfg); err != nil {
		t.Fatalf("attempt after cooldown should succeed: %v", err)
	}
	if successStub.calls != 1 {
		t.Fatalf("success attempt should have called the model once, got %d", successStub.calls)
	}

	retryFail := &failingModel{}
	retryCfg := sessionCfg
	retryCfg.HTTPClient = &http.Client{Transport: retryFail}
	if _, err := svc.RunCompactionSync(context.Background(), retryCfg); err == nil {
		t.Fatal("expected failure from immediate retry model")
	}
	if retryFail.calls != 1 {
		t.Fatalf("success must have cleared the cooldown, allowing an immediate retry; calls=%d", retryFail.calls)
	}
}

type errTransport struct{ err error }

func (t errTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, t.err }

func TestRunCompactionSyncInterruptedRunDoesNotArmCooldown(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		ctx  func(t *testing.T) context.Context
	}{
		{"caller canceled", func(t *testing.T) context.Context {
			t.Helper()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}},
		{"caller deadline exceeded", func(t *testing.T) context.Context {
			t.Helper()
			ctx, cancel := context.WithDeadline(context.Background(), time.Unix(0, 0))
			t.Cleanup(cancel)
			return ctx
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &fakeQueries{uncompacted: machineryCorpus(t)}
			svc := newMachineryService(q)
			stub := &stubModel{summary: "recovered summary"}
			cfg := machineryConfig(stub, 200)

			interruptedCtx := tc.ctx(t)
			interrupted := cfg
			interrupted.HTTPClient = &http.Client{Transport: errTransport{err: interruptedCtx.Err()}}
			if _, err := svc.RunCompactionSync(interruptedCtx, interrupted); err == nil {
				t.Fatal("interrupted run must surface an error")
			}

			res, err := svc.RunCompactionSync(context.Background(), cfg)
			if err != nil {
				t.Fatalf("run after interruption: %v", err)
			}
			if res.Status != StatusOK {
				t.Fatalf("status after interruption = %q, want %q (cooldown must not be armed)", res.Status, StatusOK)
			}
		})
	}
}

func TestRunCompactionSyncClientTimeoutStillArmsCooldown(t *testing.T) {
	t.Parallel()

	// The HTTP client's own timeout fires with the caller's context still
	// live: the model is genuinely too slow, so the cooldown must arm —
	// otherwise the sync backstop re-attempts the same slow call every turn.
	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	stub := &stubModel{summary: "healthy summary"}
	cfg := machineryConfig(stub, 200)

	slow := cfg
	slow.HTTPClient = &http.Client{Transport: errTransport{err: context.DeadlineExceeded}}
	if _, err := svc.RunCompactionSync(context.Background(), slow); err == nil {
		t.Fatal("timed-out run must surface an error")
	}

	res, err := svc.RunCompactionSync(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run during cooldown: %v", err)
	}
	if res.Status != StatusNoop {
		t.Fatalf("status during cooldown = %q, want %q", res.Status, StatusNoop)
	}
	if stub.calls != 0 {
		t.Fatalf("model called %d times during cooldown, want 0", stub.calls)
	}
}

func TestRunCompactionSyncModelFailureStillArmsCooldown(t *testing.T) {
	t.Parallel()

	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	svc := newMachineryService(q)
	stub := &stubModel{summary: "healthy summary"}
	cfg := machineryConfig(stub, 200)

	failing := cfg
	failing.HTTPClient = &http.Client{Transport: errTransport{err: errors.New("upstream 500")}}
	if _, err := svc.RunCompactionSync(context.Background(), failing); err == nil {
		t.Fatal("failing run must surface an error")
	}

	res, err := svc.RunCompactionSync(context.Background(), cfg)
	if err != nil {
		t.Fatalf("run during cooldown: %v", err)
	}
	if res.Status != StatusNoop {
		t.Fatalf("status during cooldown = %q, want %q", res.Status, StatusNoop)
	}
	if stub.calls != 0 {
		t.Fatalf("model called %d times during cooldown, want 0", stub.calls)
	}
}

func TestRunCompactionSyncSurfacesCompletionPersistenceFailure(t *testing.T) {
	t.Parallel()

	q := &fakeQueries{uncompacted: machineryCorpus(t), completeErr: errors.New("db down")}
	svc := newMachineryService(q)
	stub := &stubModel{summary: "durable summary"}

	res, err := svc.RunCompactionSync(context.Background(), machineryConfig(stub, 200))
	if err == nil {
		t.Fatal("completion persistence failure must surface an error")
	}
	if res.Status == StatusOK {
		t.Fatal("result must not claim ok when the summary was never persisted")
	}
}

func TestDoCompactionSharesPromptBudgetWithPriorContext(t *testing.T) {
	t.Parallel()

	rows := make([]sqlc.ListUncompactedMessagesBySessionRow, 0, 13)
	for i := 0; i < 12; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		rows = append(rows, mkRow(t, role, `"turn `+strconv.Itoa(i)+`"`, 100))
	}
	rows = append(rows, mkRow(t, "user", `"current"`, 40))

	run := func(t *testing.T, priorLogs []sqlc.BotHistoryMessageCompact) int {
		t.Helper()
		q := &fakeQueries{uncompacted: rows, priorLogs: priorLogs}
		stub := &stubModel{summary: "condensed"}
		svc := newMachineryService(q)
		cfg := machineryConfig(stub, 100)
		cfg.MaxCompactTokens = 1000
		res, err := svc.RunCompactionSync(context.Background(), cfg)
		if err != nil {
			t.Fatalf("RunCompactionSync: %v", err)
		}
		if res.Status != StatusOK {
			t.Fatalf("status = %q, want %q", res.Status, StatusOK)
		}
		return len(q.markedIDs)
	}

	without := run(t, nil)
	with := run(t, []sqlc.BotHistoryMessageCompact{{Status: "ok", Summary: strings.Repeat("p", 1200)}})
	if with >= without {
		t.Fatalf("prior context must carve out of the entries budget: marked %d with prior, %d without", with, without)
	}
}

func TestDoCompactionSacrificesPriorContextForOversizedEntries(t *testing.T) {
	t.Parallel()

	rows := []sqlc.ListUncompactedMessagesBySessionRow{
		mkRow(t, "user", `"the enormous old turn"`, 900),
		mkRow(t, "user", `"small follow-up"`, 100),
		mkRow(t, "user", `"current"`, 40),
	}
	prior := strings.Repeat("history so far ", 70) // ~262 tokens, within the 1/4 allowance
	q := &fakeQueries{
		uncompacted: rows,
		priorLogs:   []sqlc.BotHistoryMessageCompact{{Status: "ok", Summary: prior}},
	}
	stub := &stubModel{summary: "condensed"}
	svc := newMachineryService(q)

	cfg := machineryConfig(stub, 100)
	cfg.MaxCompactTokens = 1000
	res, err := svc.RunCompactionSync(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunCompactionSync: %v", err)
	}
	if res.Status != StatusOK {
		t.Fatalf("status = %q, want %q", res.Status, StatusOK)
	}
	if len(q.markedIDs) != 1 || q.markedIDs[0] != rows[0].ID {
		t.Fatalf("marked = %v, want the oversized first markable turn", q.markedIDs)
	}
	// The oversized kept entries exceed their budget share, so the reference
	// prior context must shrink (truncate) to keep the combined prompt within
	// MaxCompactTokens instead of stacking on top of it.
	if strings.Contains(stub.prompt, prior) {
		t.Fatal("full prior context must not ride along with oversized entries")
	}
	if got := estimateBytesAsTokens(stub.prompt); got > 1000+150 {
		t.Fatalf("combined prompt ~%d tokens, want within MaxCompactTokens plus the fixed wrapper", got)
	}
}
