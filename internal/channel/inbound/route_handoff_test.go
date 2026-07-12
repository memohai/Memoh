package inbound

import (
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRouteHandoffNilStartPreservesWorkForRetry(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("first"))
	dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("second"))
	handoff := primary.Lease.Release()
	if err := handoff.Start(nil); !errors.Is(err, errRouteHandoffStartMissing) {
		t.Fatalf("nil Start() error = %v", err)
	}
	var started *routeLease
	if err := handoff.Start(func(lease *routeLease, work *deferredTurn) error {
		if work.id != "first" {
			t.Fatalf("retried work = %q", work.id)
		}
		started = lease
		return nil
	}); err != nil {
		t.Fatalf("retry Start() error = %v", err)
	}
	if started == nil {
		t.Fatal("retry did not claim handoff")
	}
	next := started.Release()
	if next == nil || next.work.id != "second" {
		t.Fatalf("next handoff = %#v", next)
	}
	next.Abort()
}

func TestRouteHandoffCannotStartReleasedLease(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("stale"))
	handoff := primary.Lease.Release()
	handoff.lease.Release()
	current := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("current"))
	var called atomic.Bool
	err := handoff.Start(func(*routeLease, *deferredTurn) error {
		called.Store(true)
		return nil
	})
	if !errors.Is(err, errRouteHandoffConsumed) || called.Load() {
		t.Fatalf("stale Start() error=%v called=%v", err, called.Load())
	}
	current.Lease.Release()
}

func TestRouteHandoffConcurrentStartAndAbortHasOneOwner(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
		dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("queued"))
		handoff := primary.Lease.Release()
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var callbackCount atomic.Int32
		var startErr error
		var started *routeLease
		var abortNext *routeHandoff
		go func() {
			defer wait.Done()
			<-start
			startErr = handoff.Start(func(lease *routeLease, _ *deferredTurn) error {
				callbackCount.Add(1)
				started = lease
				return nil
			})
		}()
		go func() {
			defer wait.Done()
			<-start
			abortNext = handoff.Abort()
		}()
		close(start)
		wait.Wait()

		switch callbackCount.Load() {
		case 0:
			if !errors.Is(startErr, errRouteHandoffConsumed) || abortNext != nil {
				t.Fatalf("iteration %d abort owner: err=%v next=%#v", iteration, startErr, abortNext)
			}
		case 1:
			if startErr != nil || started == nil || abortNext != nil {
				t.Fatalf("iteration %d start owner: err=%v lease=%#v next=%#v", iteration, startErr, started, abortNext)
			}
			started.Release()
		default:
			t.Fatalf("iteration %d callbacks = %d", iteration, callbackCount.Load())
		}
	}
}
