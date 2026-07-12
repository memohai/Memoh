package inbound

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestRouteLifecycleAdmitsSinglePrimaryAndInjectionTicket(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	if primary.Kind != routeAdmissionStartPrimary || primary.Lease == nil || primary.Lease.Inbox() == nil {
		t.Fatalf("primary admission = %#v", primary)
	}
	injection := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
	if injection.Kind != routeAdmissionInject || injection.Ticket == nil {
		t.Fatalf("injection admission = %#v", injection)
	}

	message := conversation.InjectMessage{Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"}}
	if got := injection.Ticket.Deliver(message); got != injectionDeliveryAccepted {
		t.Fatalf("delivery = %v", got)
	}
	select {
	case got := <-primary.Lease.Inbox():
		if got.Receipt.ID != "receipt-1" {
			t.Fatalf("inbox receipt = %#v", got.Receipt)
		}
	default:
		t.Fatal("primary inbox did not receive injection")
	}

	handoff := primary.Lease.Release()
	if handoff == nil || handoff.work.id != "injected" || handoff.lease == nil {
		t.Fatalf("uncommitted injection handoff = %#v", handoff)
	}
	select {
	case _, ok := <-primary.Lease.Inbox():
		if ok {
			t.Fatal("released primary inbox remained open")
		}
	default:
		t.Fatal("released primary inbox was not closed")
	}
	handoff.Abort()
}

func TestRouteLifecycleDurableCommitSuppressesFallback(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	injection := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
	message := conversation.InjectMessage{Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"}}
	if injection.Ticket.Deliver(message) != injectionDeliveryAccepted {
		t.Fatal("injection was not accepted")
	}
	<-primary.Lease.Inbox()
	if !primary.Lease.CommitPersisted("receipt-1") {
		t.Fatal("durable receipt commit was rejected")
	}
	if handoff := primary.Lease.Release(); handoff != nil {
		t.Fatalf("committed injection fell back: %#v", handoff)
	}
	if primary.Lease.CommitPersisted("receipt-1") {
		t.Fatal("stale duplicate commit was accepted")
	}
}

func TestRouteLifecycleReleaseBeforeDeliveryTransfersOwnershipOnce(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	injection := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
	handoff := primary.Lease.Release()
	if handoff == nil || handoff.work.id != "injected" {
		t.Fatalf("reserved ticket handoff = %#v", handoff)
	}
	message := conversation.InjectMessage{Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"}}
	if got := injection.Ticket.Deliver(message); got != injectionDeliveryDeferred {
		t.Fatalf("stale ticket delivery = %v", got)
	}
	if duplicate := primary.Lease.Release(); duplicate != nil {
		t.Fatalf("double release created duplicate handoff: %#v", duplicate)
	}
	if next := handoff.Abort(); next != nil {
		t.Fatalf("stale delivery duplicated backlog: %#v", next)
	}
}

func TestRouteLifecyclePreservesMixedFallbackFIFO(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	ticket := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
	queued := dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("queued"))
	if queued.Kind != routeAdmissionQueued {
		t.Fatalf("queue admission = %#v", queued)
	}
	if ticket.Ticket.Deliver(conversation.InjectMessage{
		Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"},
	}) != injectionDeliveryAccepted {
		t.Fatal("injection was not delivered")
	}
	<-primary.Lease.Inbox()

	first := primary.Lease.Release()
	if first == nil || first.work.id != "injected" {
		t.Fatalf("first handoff = %#v", first)
	}
	second := first.Abort()
	if second == nil || second.work.id != "queued" {
		t.Fatalf("second handoff = %#v", second)
	}
	if third := second.Abort(); third != nil {
		t.Fatalf("unexpected third handoff = %#v", third)
	}
}

func TestRouteLifecycleQueueCannotStrandAcrossRelease(t *testing.T) {
	t.Parallel()

	t.Run("queue linearizes first", func(t *testing.T) {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
		if got := dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("queued")); got.Kind != routeAdmissionQueued {
			t.Fatalf("queue admission = %#v", got)
		}
		handoff := primary.Lease.Release()
		if handoff == nil || handoff.work.id != "queued" {
			t.Fatalf("queue-before-release handoff = %#v", handoff)
		}
		handoff.Abort()
	})

	t.Run("release linearizes first", func(t *testing.T) {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
		if handoff := primary.Lease.Release(); handoff != nil {
			t.Fatalf("empty release handoff = %#v", handoff)
		}
		got := dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("queued"))
		if got.Kind != routeAdmissionStartPrimary || got.Lease == nil {
			t.Fatalf("release-before-queue admission = %#v", got)
		}
	})
}

func TestRouteLifecycleParallelLeaseDoesNotOwnInjection(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	idleParallel := dispatcher.Admit("route-1", routeIntentParallel, testDeferredTurn("idle-now"))
	if idleParallel.Kind != routeAdmissionStartPrimary || idleParallel.Lease.Inbox() == nil {
		t.Fatalf("idle /now admission = %#v", idleParallel)
	}
	busyParallel := dispatcher.Admit("route-1", routeIntentParallel, testDeferredTurn("busy-now"))
	if busyParallel.Kind != routeAdmissionStartParallel || busyParallel.Lease == nil || busyParallel.Lease.Inbox() != nil {
		t.Fatalf("busy /now admission = %#v", busyParallel)
	}
	injection := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
	if injection.Kind != routeAdmissionInject {
		t.Fatalf("injection admission = %#v", injection)
	}
	if injection.Ticket.Deliver(conversation.InjectMessage{
		Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"},
	}) != injectionDeliveryAccepted {
		t.Fatal("injection was not delivered to primary")
	}
	select {
	case <-idleParallel.Lease.Inbox():
	default:
		t.Fatal("primary did not receive injection")
	}
	if handoff := idleParallel.Lease.Release(); handoff != nil {
		t.Fatalf("primary promoted while busy lease remained: %#v", handoff)
	}
	handoff := busyParallel.Lease.Release()
	if handoff == nil || handoff.work.id != "injected" {
		t.Fatalf("final busy release handoff = %#v", handoff)
	}
	handoff.Abort()
}

func TestRouteLifecycleEpochFencesInboxReleaseAndCancel(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	first := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("first"))
	var firstCanceled atomic.Int32
	first.Lease.BindCancel(func() { firstCanceled.Add(1) })
	if got := dispatcher.CancelAll("route-1"); got != 1 || firstCanceled.Load() != 1 {
		t.Fatalf("CancelAll first = %d canceled=%d", got, firstCanceled.Load())
	}
	if handoff := first.Lease.Release(); handoff != nil {
		t.Fatalf("first empty handoff = %#v", handoff)
	}
	second := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("second"))
	if second.Lease == nil || second.Lease.Inbox() == first.Lease.Inbox() {
		t.Fatalf("epoch inbox was reused: first=%p second=%p", first.Lease.Inbox(), second.Lease.Inbox())
	}
	select {
	case _, ok := <-first.Lease.Inbox():
		if ok {
			t.Fatal("first epoch inbox remained open")
		}
	default:
		t.Fatal("first epoch inbox was not closed")
	}
	if duplicate := first.Lease.Release(); duplicate != nil {
		t.Fatalf("stale release affected second epoch: %#v", duplicate)
	}
	var secondCanceled atomic.Int32
	second.Lease.BindCancel(func() { secondCanceled.Add(1) })
	if got := dispatcher.CancelAll("route-1"); got != 1 || secondCanceled.Load() != 1 || firstCanceled.Load() != 1 {
		t.Fatalf("CancelAll second = %d first=%d second=%d", got, firstCanceled.Load(), secondCanceled.Load())
	}
	second.Lease.Release()
}

func TestRouteLifecycleStartOnlyRejectsPrimaryButStartsAlongsideBusyOnly(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	busy := dispatcher.Admit("route-1", routeIntentParallel, testDeferredTurn("parallel"))
	if got := dispatcher.Admit("route-1", routeIntentStartOnly, testDeferredTurn("skill")); got.Kind != routeAdmissionRejected {
		t.Fatalf("start-only primary admission = %#v", got)
	}
	primary.Lease.Release()
	got := dispatcher.Admit("route-1", routeIntentStartOnly, testDeferredTurn("skill"))
	if got.Kind != routeAdmissionStartPrimary || got.Lease == nil || got.Lease.Inbox() == nil {
		t.Fatalf("start-only alongside busy-only admission = %#v", got)
	}
	got.Lease.Release()
	busy.Lease.Release()
}

func TestRouteLifecycleStartOnlyDoesNotBypassBusyBacklog(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	busy := dispatcher.Admit("route-1", routeIntentParallel, testDeferredTurn("parallel"))
	if got := dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("queued")); got.Kind != routeAdmissionQueued {
		t.Fatalf("queue admission = %#v", got)
	}
	if handoff := primary.Lease.Release(); handoff != nil {
		t.Fatalf("primary promoted while busy lease remained: %#v", handoff)
	}
	if got := dispatcher.Admit("route-1", routeIntentStartOnly, testDeferredTurn("skill")); got.Kind != routeAdmissionRejected {
		t.Fatalf("start-only bypassed backlog: %#v", got)
	}
	handoff := busy.Lease.Release()
	if handoff == nil || handoff.work.id != "queued" {
		t.Fatalf("backlog handoff = %#v", handoff)
	}
	handoff.Abort()
}

func TestRouteLifecycleCancelBeforeBindReachesEveryLease(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	busy := dispatcher.Admit("route-1", routeIntentParallel, testDeferredTurn("parallel"))
	if got := dispatcher.CancelAll("route-1"); got != 2 {
		t.Fatalf("CancelAll() = %d, want 2", got)
	}
	var primaryCanceled atomic.Int32
	var busyCanceled atomic.Int32
	primary.Lease.BindCancel(func() { primaryCanceled.Add(1) })
	busy.Lease.BindCancel(func() { busyCanceled.Add(1) })
	if primaryCanceled.Load() != 1 || busyCanceled.Load() != 1 {
		t.Fatalf("late cancel binding = primary:%d busy:%d", primaryCanceled.Load(), busyCanceled.Load())
	}
	primary.Lease.Release()
	busy.Lease.Release()
}

func TestRouteLifecycleWrongEpochCannotCommitFallback(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	first := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("first"))
	injection := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
	if injection.Ticket.Deliver(conversation.InjectMessage{
		Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"},
	}) != injectionDeliveryAccepted {
		t.Fatal("injection was not accepted")
	}
	<-first.Lease.Inbox()
	handoff := first.Lease.Release()
	if handoff == nil {
		t.Fatal("uncommitted receipt did not fall back")
	}
	if first.Lease.CommitPersisted("receipt-1") || handoff.lease.CommitPersisted("receipt-1") {
		t.Fatal("wrong epoch suppressed fallback")
	}
	handoff.Abort()
}

func TestRouteLifecycleFullInboxDefersOnceAndDrainsOldEpoch(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	overflow := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("overflow"))
	dispatcher.lifecycleMu.Lock()
	leaseState := dispatcher.routeLifecycle["route-1"].leases[primary.Lease.leaseID]
	for i := 0; i < cap(leaseState.inbox); i++ {
		leaseState.inbox <- conversation.InjectMessage{Text: "filler"}
	}
	dispatcher.lifecycleMu.Unlock()
	if got := overflow.Ticket.Deliver(conversation.InjectMessage{
		Text: "overflow", Receipt: conversation.UserMessageReceipt{ID: "receipt-overflow"},
	}); got != injectionDeliveryDeferred {
		t.Fatalf("overflow delivery = %v", got)
	}
	handoff := primary.Lease.Release()
	if handoff == nil || handoff.work.id != "overflow" {
		t.Fatalf("overflow handoff = %#v", handoff)
	}
	select {
	case _, ok := <-primary.Lease.Inbox():
		if ok {
			t.Fatal("released epoch retained buffered injection")
		}
	default:
		t.Fatal("released epoch inbox was not closed")
	}
	if duplicate := handoff.Abort(); duplicate != nil {
		t.Fatalf("overflow duplicated fallback: %#v", duplicate)
	}
}

func TestRouteLifecycleCleanupCannotAliasStaleHandles(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	first := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("first"))
	staleTicket := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("stale-injection"))
	handoff := first.Lease.Release()
	if handoff == nil || handoff.work.id != "stale-injection" {
		t.Fatalf("stale work handoff = %#v", handoff)
	}
	handoff.Abort()
	if _, retained := dispatcher.routeLifecycle["route-1"]; retained {
		t.Fatal("quiescent route lifecycle was retained")
	}

	second := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("second"))
	if second.Lease.leaseID == first.Lease.leaseID || second.Lease.epoch == first.Lease.epoch {
		t.Fatalf("global fence reused: first=%#v second=%#v", first.Lease, second.Lease)
	}
	if got := staleTicket.Ticket.Deliver(conversation.InjectMessage{
		Text: "stale", Receipt: conversation.UserMessageReceipt{ID: "stale-receipt"},
	}); got != injectionDeliveryDeferred {
		t.Fatalf("stale ticket delivery = %v", got)
	}
	select {
	case got := <-second.Lease.Inbox():
		t.Fatalf("stale ticket reached new epoch: %#v", got)
	default:
	}
	if stale := first.Lease.Release(); stale != nil || first.Lease.CommitPersisted("stale-receipt") {
		t.Fatalf("stale lease affected new epoch: handoff=%#v", stale)
	}
	second.Lease.Release()
}

func TestRouteLifecycleHandoffStartFailureReleasesAndContinues(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
	primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
	dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("fails-to-start"))
	dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("starts"))
	handoff := primary.Lease.Release()
	if handoff == nil {
		t.Fatal("queued handoff is nil")
	}
	startErr := errors.New("open stream failed")
	var attempts []string
	var started *routeLease
	err := handoff.Start(func(lease *routeLease, work *deferredTurn) error {
		attempts = append(attempts, work.id)
		if work.id == "fails-to-start" {
			return startErr
		}
		started = lease
		return nil
	})
	if !errors.Is(err, startErr) || len(attempts) != 2 || attempts[0] != "fails-to-start" || attempts[1] != "starts" || started == nil {
		t.Fatalf("handoff start err=%v attempts=%v started=%#v", err, attempts, started)
	}
	if err := handoff.Start(func(*routeLease, *deferredTurn) error { return nil }); !errors.Is(err, errRouteHandoffConsumed) {
		t.Fatalf("duplicate handoff start error = %v", err)
	}
	started.Release()
}

func TestRouteLifecycleConcurrentDeliveryAndReleaseOwnsFallbackOnce(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
		injection := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("injected"))
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var delivery injectionDeliveryResult
		var handoff *routeHandoff
		go func() {
			defer wait.Done()
			<-start
			delivery = injection.Ticket.Deliver(conversation.InjectMessage{
				Text: "injected", Receipt: conversation.UserMessageReceipt{ID: "receipt-1"},
			})
		}()
		go func() {
			defer wait.Done()
			<-start
			handoff = primary.Lease.Release()
		}()
		close(start)
		wait.Wait()
		if delivery != injectionDeliveryAccepted && delivery != injectionDeliveryDeferred {
			t.Fatalf("iteration %d delivery = %v", iteration, delivery)
		}
		if handoff == nil || handoff.work.id != "injected" {
			t.Fatalf("iteration %d handoff = %#v", iteration, handoff)
		}
		if duplicate := handoff.Abort(); duplicate != nil {
			t.Fatalf("iteration %d duplicate fallback = %#v", iteration, duplicate)
		}
	}
}

func TestRouteLifecycleConcurrentQueueAndReleaseCannotStrand(t *testing.T) {
	t.Parallel()

	for iteration := 0; iteration < 100; iteration++ {
		dispatcher := NewRouteDispatcher(slog.New(slog.DiscardHandler))
		primary := dispatcher.Admit("route-1", routeIntentContinue, testDeferredTurn("primary"))
		start := make(chan struct{})
		var wait sync.WaitGroup
		wait.Add(2)
		var admission routeAdmission
		var handoff *routeHandoff
		go func() {
			defer wait.Done()
			<-start
			admission = dispatcher.Admit("route-1", routeIntentQueue, testDeferredTurn("queued"))
		}()
		go func() {
			defer wait.Done()
			<-start
			handoff = primary.Lease.Release()
		}()
		close(start)
		wait.Wait()
		switch admission.Kind {
		case routeAdmissionQueued:
			if handoff == nil || handoff.work.id != "queued" {
				t.Fatalf("iteration %d queued admission handoff = %#v", iteration, handoff)
			}
			handoff.Abort()
		case routeAdmissionStartPrimary:
			if handoff != nil || admission.Lease == nil {
				t.Fatalf("iteration %d start admission=%#v handoff=%#v", iteration, admission, handoff)
			}
			admission.Lease.Release()
		default:
			t.Fatalf("iteration %d admission = %#v", iteration, admission)
		}
	}
}

func testDeferredTurn(id string) *deferredTurn {
	return &deferredTurn{id: id, ctx: context.Background()}
}
