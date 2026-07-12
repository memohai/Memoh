package inbound

import (
	"log/slog"
	"sync"
	"testing"
)

func TestRouteDispatcherAdmitDeferredReservesIdleRoute(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	turn := testDeferredTurn("queued")
	if admission := dispatcher.admitDeferred("route-1", turn); admission != routeAdmissionStartPrimary {
		t.Fatalf("idle admission = %v", admission)
	}
	if !turn.routeOwnerTransferred || turn.transferredInjectCh == nil || !dispatcher.IsActive("route-1") {
		t.Fatalf("idle admission did not reserve ownership: turn=%#v active=%v", turn, dispatcher.IsActive("route-1"))
	}
	if result := dispatcher.MarkDone("route-1"); len(result.DeferredTurns) != 0 || dispatcher.IsActive("route-1") {
		t.Fatalf("release = %#v active=%v", result, dispatcher.IsActive("route-1"))
	}
}

func TestRouteDispatcherDeferredQueueHandsOffOneAtATime(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	first := testDeferredTurn("first")
	second := testDeferredTurn("second")
	dispatcher.MarkActive("route-1")
	dispatcher.admitDeferred("route-1", first)
	dispatcher.admitDeferred("route-1", second)
	result := dispatcher.MarkDone("route-1")
	if len(result.DeferredTurns) != 1 || result.DeferredTurns[0] != first {
		t.Fatalf("first handoff = %#v", result.DeferredTurns)
	}
	result = dispatcher.MarkDone("route-1")
	if len(result.DeferredTurns) != 1 || result.DeferredTurns[0] != second {
		t.Fatalf("second handoff = %#v", result.DeferredTurns)
	}
	if result = dispatcher.MarkDone("route-1"); len(result.DeferredTurns) != 0 || dispatcher.IsActive("route-1") {
		t.Fatalf("final deferred release = %#v active=%v", result, dispatcher.IsActive("route-1"))
	}
}

func TestRouteDispatcherDeferredAdmissionReleaseLinearizes(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		dispatcher.MarkActive("route-1")
		turn := testDeferredTurn("queued")
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var admission routeAdmissionKind
		var result MarkDoneResult
		go func() {
			defer wait.Done()
			<-start
			admission = dispatcher.admitDeferred("route-1", turn)
		}()
		go func() {
			defer wait.Done()
			<-start
			result = dispatcher.MarkDone("route-1")
		}()
		close(start)
		wait.Wait()
		if admission == routeAdmissionQueued {
			if len(result.DeferredTurns) != 1 || result.DeferredTurns[0] != turn {
				t.Fatalf("iteration %d accepted turn stranded: %#v", iteration, result.DeferredTurns)
			}
		} else if admission != routeAdmissionStartPrimary || len(result.DeferredTurns) != 0 {
			t.Fatalf("iteration %d admission=%v release=%#v", iteration, admission, result.DeferredTurns)
		}
		if !dispatcher.IsActive("route-1") {
			t.Fatalf("iteration %d admitted turn lost ownership", iteration)
		}
		dispatcher.MarkDone("route-1")
	}
}

func TestRouteDispatcherConcurrentIdleDeferredAdmissionStartsOne(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		turns := []*deferredTurn{testDeferredTurn("first"), testDeferredTurn("second")}
		start := make(chan struct{})
		results := make(chan routeAdmissionKind, len(turns))
		var wait sync.WaitGroup
		for _, turn := range turns {
			wait.Add(1)
			go func() {
				defer wait.Done()
				<-start
				results <- dispatcher.admitDeferred("route-1", turn)
			}()
		}
		close(start)
		wait.Wait()
		close(results)

		counts := map[routeAdmissionKind]int{}
		for result := range results {
			counts[result]++
		}
		if counts[routeAdmissionStartPrimary] != 1 || counts[routeAdmissionQueued] != 1 {
			t.Fatalf("iteration %d admissions = %#v", iteration, counts)
		}
		if result := dispatcher.MarkDone("route-1"); len(result.DeferredTurns) != 1 || !dispatcher.IsActive("route-1") {
			t.Fatalf("iteration %d handoff = %#v active=%v", iteration, result, dispatcher.IsActive("route-1"))
		}
		dispatcher.MarkDone("route-1")
	}
}
