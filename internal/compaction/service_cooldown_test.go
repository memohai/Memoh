package compaction

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type compactionErrorTransport struct {
	err error
}

func TestRunCompactionInterruptedModelCallDoesNotArmFailureCooldown(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name       string
		newContext func() (context.Context, context.CancelFunc)
	}{
		{
			name: "canceled",
			newContext: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx, cancel
			},
		},
		{
			name: "deadline exceeded",
			newContext: func() (context.Context, context.CancelFunc) {
				return context.WithDeadline(context.Background(), time.Unix(0, 0))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			q := &fakeQueries{uncompacted: machineryCorpus(t)}
			service := newMachineryService(q)
			cfg := machineryConfig(&stubModel{}, 450)
			ctx, cancel := test.newContext()
			defer cancel()
			cfg.HTTPClient = &http.Client{Transport: compactionErrorTransport{err: ctx.Err()}}

			if _, err := service.RunCompactionSync(ctx, cfg); !errors.Is(err, ctx.Err()) {
				t.Fatalf("interrupted compaction error = %v, want %v", err, ctx.Err())
			}

			recovered := &stubModel{summary: "recovered summary"}
			cfg.HTTPClient = &http.Client{Transport: recovered}
			result, err := service.RunCompactionSync(context.Background(), cfg)
			if err != nil {
				t.Fatalf("healthy retry after interruption: %v", err)
			}
			if result.Status != StatusOK || recovered.calls != 1 {
				t.Fatalf("healthy retry = result:%#v calls:%d, want successful model call", result, recovered.calls)
			}
		})
	}
}

func TestProviderTimeoutStillArmsFailureCooldown(t *testing.T) {
	t.Parallel()

	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	service := newMachineryService(q)
	cfg := machineryConfig(&stubModel{}, 450)
	timedOut := cfg
	timedOut.HTTPClient = &http.Client{Transport: compactionErrorTransport{err: context.DeadlineExceeded}}
	if _, err := service.RunCompactionSync(context.Background(), timedOut); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("provider timeout error = %v, want deadline exceeded", err)
	}

	recovered := &stubModel{summary: "must stay cooled down"}
	cfg.HTTPClient = &http.Client{Transport: recovered}
	result, err := service.RunCompactionSync(context.Background(), cfg)
	if err != nil {
		t.Fatalf("automatic retry during provider cooldown: %v", err)
	}
	if result.Status != StatusNoop || recovered.calls != 0 {
		t.Fatalf("automatic retry = result:%#v calls:%d, want cooldown noop", result, recovered.calls)
	}
}

func TestInterruptedManualBypassPreservesExistingFailureCooldown(t *testing.T) {
	t.Parallel()

	q := &fakeQueries{uncompacted: machineryCorpus(t)}
	service := newMachineryService(q)
	cfg := machineryConfig(&stubModel{}, 450)
	now := time.Now()
	service.nowFn = func() time.Time { return now }
	service.recordCompactionFailure(cfg.SessionID)
	now = now.Add(compactionFailureCooldown - time.Second)

	manual := cfg
	manual.Manual = true
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manual.HTTPClient = &http.Client{Transport: compactionErrorTransport{err: ctx.Err()}}
	if _, err := service.RunCompactionSync(ctx, manual); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted manual compaction error = %v, want context canceled", err)
	}
	now = now.Add(2 * time.Second)

	recovered := &stubModel{summary: "old cooldown expired"}
	cfg.HTTPClient = &http.Client{Transport: recovered}
	result, err := service.RunCompactionSync(context.Background(), cfg)
	if err != nil {
		t.Fatalf("automatic retry after original cooldown: %v", err)
	}
	if result.Status != StatusOK || recovered.calls != 1 {
		t.Fatalf("automatic retry = result:%#v calls:%d, want expired original cooldown", result, recovered.calls)
	}
}

func TestCallerInterruptionOnlyRejectsConcurrentFailures(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	persistenceErr := errors.New("persistence failed")

	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "direct cause", err: context.Canceled, want: true},
		{name: "wrapped cause", err: fmt.Errorf("model call: %w", context.Canceled), want: true},
		{name: "joined failure", err: errors.Join(context.Canceled, persistenceErr), want: false},
		{name: "unrelated failure", err: persistenceErr, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := callerInterruptionOnly(ctx, test.err); got != test.want {
				t.Fatalf("callerInterruptionOnly(%v) = %v, want %v", test.err, got, test.want)
			}
		})
	}
}

func (t compactionErrorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}
