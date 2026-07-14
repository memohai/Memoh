package inbound

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/conversation"
)

func TestRouteLifecycleSessionChangePurgesOldGeneration(t *testing.T) {
	t.Parallel()
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	oldPrimaryWork := testDeferredTurn("old-primary")
	oldPrimaryWork.sessionID = "session-old"
	oldPrimary := dispatcher.Admit("route-1", routeIntentContinue, oldPrimaryWork)
	oldQueued := testDeferredTurn("old-queued")
	oldQueued.sessionID = "session-old"
	dispatcher.Admit("route-1", routeIntentQueue, oldQueued)
	oldInjected := testDeferredTurn("old-injected")
	oldInjected.sessionID = "session-old"
	ticket := dispatcher.Admit("route-1", routeIntentContinue, oldInjected)
	if ticket.Ticket.Deliver(conversation.InjectMessage{
		Text: "old-injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-old"},
	}) != injectionDeliveryAccepted {
		t.Fatal("old injection was not delivered")
	}
	oldParallelWork := testDeferredTurn("old-parallel")
	oldParallelWork.sessionID = "session-old"
	oldParallel := dispatcher.Admit("route-1", routeIntentParallel, oldParallelWork)
	var primaryCanceled atomic.Int32
	oldPrimary.Lease.BindCancel(func() { primaryCanceled.Add(1) })
	newWork := testDeferredTurn("new-primary")
	newWork.sessionID = "session-new"
	if got := dispatcher.AdvanceScope("route-1", "session-new"); got != 2 {
		t.Fatalf("old generation canceled leases = %d, want 2", got)
	}
	newPrimary := dispatcher.Admit("route-1", routeIntentContinue, newWork)
	if newPrimary.Kind != routeAdmissionStartPrimary || newPrimary.Lease == nil {
		t.Fatalf("new generation admission = %#v", newPrimary)
	}
	if primaryCanceled.Load() != 1 {
		t.Fatalf("old primary cancel count = %d", primaryCanceled.Load())
	}
	var parallelCanceled atomic.Int32
	oldParallel.Lease.BindCancel(func() { parallelCanceled.Add(1) })
	if parallelCanceled.Load() != 1 {
		t.Fatalf("late old parallel cancel count = %d", parallelCanceled.Load())
	}
	select {
	case _, open := <-oldPrimary.Lease.Inbox():
		if open {
			t.Fatal("old primary inbox remained open")
		}
	default:
		t.Fatal("old primary inbox was not closed")
	}
	if oldPrimary.Lease.CommitPersisted("receipt-old") {
		t.Fatal("stale generation committed an old receipt")
	}
	if handoff := oldPrimary.Lease.Release(); handoff != nil {
		t.Fatalf("stale primary released old backlog: %#v", handoff)
	}
	if handoff := oldParallel.Lease.Release(); handoff != nil {
		t.Fatalf("stale parallel released old backlog: %#v", handoff)
	}
	if handoff := newPrimary.Lease.Release(); handoff != nil {
		t.Fatalf("purged backlog resurfaced in new generation: %#v", handoff)
	}
}

func TestRouteLifecycleAdvanceScopePreservesCurrentGeneration(t *testing.T) {
	t.Parallel()
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	work := testDeferredTurn("new-primary")
	work.sessionID = "session-new"
	primary := dispatcher.Admit("route-1", routeIntentContinue, work)
	if reset := dispatcher.AdvanceScope("route-1", "session-new"); reset != 0 {
		t.Fatalf("matching generation advance canceled %d leases", reset)
	}
	var canceled atomic.Int32
	primary.Lease.BindCancel(func() { canceled.Add(1) })
	if canceled.Load() != 0 {
		t.Fatal("matching generation was canceled")
	}
	primary.Lease.Release()
}

func TestRouteLifecycleAuthoritativeScopeRejectsLateOldGeneration(t *testing.T) {
	t.Parallel()
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	oldTurn := testDeferredTurn("old")
	oldTurn.sessionID = "session-old"
	old := dispatcher.Admit("route-1", routeIntentContinue, oldTurn)
	oldCanceled := make(chan struct{})
	old.Lease.BindCancel(func() { close(oldCanceled) })
	if got := dispatcher.AdvanceScope("route-1", "session-new"); got != 1 {
		t.Fatalf("canceled leases = %d, want 1", got)
	}
	select {
	case <-oldCanceled:
	case <-time.After(time.Second):
		t.Fatal("old generation lease was not canceled")
	}
	newTurn := testDeferredTurn("new")
	newTurn.sessionID = "session-new"
	current := dispatcher.Admit("route-1", routeIntentContinue, newTurn)
	if current.Kind != routeAdmissionStartPrimary || current.Lease == nil {
		t.Fatalf("new generation admission = %#v, want primary", current)
	}
	currentCanceled := make(chan struct{})
	current.Lease.BindCancel(func() { close(currentCanceled) })
	lateOldTurn := testDeferredTurn("late-old")
	lateOldTurn.sessionID = "session-old"
	if lateOld := dispatcher.Admit("route-1", routeIntentContinue, lateOldTurn); lateOld.Kind != routeAdmissionStale {
		t.Fatalf("late old admission = %#v, want stale", lateOld)
	}
	select {
	case <-currentCanceled:
		t.Fatal("late old generation canceled current lease")
	default:
	}
	if scope, known := dispatcher.CurrentScope("route-1"); !known || scope != "session-new" {
		t.Fatalf("current scope = (%q, %v), want session-new", scope, known)
	}
	current.Lease.Release()
}

func TestRouteLifecycleAdvanceScopeFencesRouteWithoutActiveState(t *testing.T) {
	t.Parallel()
	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	if got := dispatcher.AdvanceScope("route-1", "session-new"); got != 0 {
		t.Fatalf("canceled leases = %d, want 0", got)
	}
	lateOldTurn := testDeferredTurn("late-old")
	lateOldTurn.sessionID = "session-old"
	if admission := dispatcher.Admit("route-1", routeIntentContinue, lateOldTurn); admission.Kind != routeAdmissionStale {
		t.Fatalf("late old admission = %#v, want stale", admission)
	}
}
