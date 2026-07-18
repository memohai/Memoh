package inbound

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/conversation"
)

func TestDetectMode(t *testing.T) {
	tests := []struct {
		input    string
		wantMode InboundMode
		wantText string
	}{
		{"hello world", ModeInject, "hello world"},
		{"/btw hello", ModeInject, "hello"},
		{"/now hello", ModeParallel, "hello"},
		{"/next hello", ModeQueue, "hello"},
		{"/BTW hello", ModeInject, "hello"},
		{"/NOW hello", ModeParallel, "hello"},
		{"/NEXT hello", ModeQueue, "hello"},
		{"/now", ModeParallel, ""},
		{"/next", ModeQueue, ""},
		{"/btw", ModeInject, ""},
		{"  /now  hello  ", ModeParallel, "hello"},
		{"/unknown hello", ModeInject, "/unknown hello"},
		{"", ModeInject, ""},
		{"/new session", ModeInject, "/new session"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			mode, text := DetectMode(tt.input)
			if mode != tt.wantMode {
				t.Errorf("DetectMode(%q) mode = %d, want %d", tt.input, mode, tt.wantMode)
			}
			if text != tt.wantText {
				t.Errorf("DetectMode(%q) text = %q, want %q", tt.input, text, tt.wantText)
			}
		})
	}
}

func TestIsStartCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/start", true},
		{"/start@MemohBot", true},
		// Telegram deep links: /start <payload> (and /start@bot <payload>).
		{"/start abc123", true},
		{"/start@MemohBot abc123", true},
		{"/start deep_link_payload", true},
		{"/new", false},
		{"/started", false},
		{"start", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			invocation, err := command.ParseInvocation(command.InvocationInput{
				Text:       tt.input,
				BotAliases: []string{"MemohBot"},
			})
			got := err == nil && invocationHasResource(&invocation, "start")
			if got != tt.want {
				t.Errorf("start invocation for %q = %v, want %v (error: %v)", tt.input, got, tt.want, err)
			}
		})
	}
}

func TestIsModeCommand(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/btw hello", true},
		{"/now hello", true},
		{"/next hello", true},
		{"/btw", true},
		{"/now", true},
		{"/next", true},
		{"/new", false},
		{"/fs list", false},
		{"hello", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := IsModeCommand(tt.input)
			if got != tt.want {
				t.Errorf("IsModeCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRouteDispatcher_InjectWhenActive(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	if d.IsActive(routeID) {
		t.Fatal("expected route to be inactive initially")
	}

	injectCh := d.MarkActive(routeID)
	if injectCh == nil {
		t.Fatal("expected non-nil inject channel")
	}
	if !d.IsActive(routeID) {
		t.Fatal("expected route to be active after MarkActive")
	}

	msg := InjectMessage{Text: "hello", HeaderifiedText: "[User] hello"}
	if d.Inject(routeID, msg) != InjectAccepted {
		t.Fatal("expected inject to succeed when route is active")
	}

	select {
	case got := <-injectCh:
		if got.Text != "hello" {
			t.Errorf("got text %q, want %q", got.Text, "hello")
		}
	default:
		t.Fatal("expected message on inject channel")
	}
}

func TestRouteDispatcher_InjectWhenInactive(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	msg := InjectMessage{Text: "hello"}
	if d.Inject(routeID, msg) != InjectUnavailable {
		t.Fatal("expected inject to fail when route is inactive")
	}
}

func TestRouteDispatcher_QueueAndDrain(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	d.MarkActive(routeID)

	d.Enqueue(routeID, QueuedTask{Text: "task-1"})
	d.Enqueue(routeID, QueuedTask{Text: "task-2"})

	result := d.MarkDone(routeID)
	if len(result.QueuedTasks) != 2 {
		t.Fatalf("expected 2 queued tasks, got %d", len(result.QueuedTasks))
	}
	if result.QueuedTasks[0].Text != "task-1" || result.QueuedTasks[1].Text != "task-2" {
		t.Errorf("unexpected task order: %v", result.QueuedTasks)
	}
	if d.IsActive(routeID) {
		t.Fatal("expected route to be inactive after MarkDone")
	}
}

func TestRouteDispatcherTryAcquireActiveAllowsOneOwner(t *testing.T) {
	t.Parallel()

	d := NewRouteDispatcher(slog.Default())
	const contenders = 32
	var acquired atomic.Int32
	var wg sync.WaitGroup
	wg.Add(contenders)
	for range contenders {
		go func() {
			defer wg.Done()
			if _, ok := d.TryAcquireActive("route"); ok {
				acquired.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := acquired.Load(); got != 1 {
		t.Fatalf("acquired owners = %d, want 1", got)
	}
}

func TestRouteDispatcherTryEnqueueIfActiveDoesNotOrphanInactiveTask(t *testing.T) {
	t.Parallel()

	d := NewRouteDispatcher(slog.Default())
	if d.TryEnqueueIfActive("route", QueuedTask{Text: "orphan"}) {
		t.Fatal("inactive route accepted queued task")
	}
	if _, ok := d.TryAcquireActive("route"); !ok {
		t.Fatal("failed to acquire inactive route")
	}
	if !d.TryEnqueueIfActive("route", QueuedTask{Text: "owned"}) {
		t.Fatal("active route rejected queued task")
	}
}

func TestRouteDispatcherFinishActiveTransfersQueueInFIFOOrder(t *testing.T) {
	t.Parallel()

	d := NewRouteDispatcher(slog.Default())
	if _, ok := d.TryAcquireActive("route"); !ok {
		t.Fatal("failed to acquire route")
	}
	if !d.TryEnqueueIfActive("route", QueuedTask{Text: "B1"}) ||
		!d.TryEnqueueIfActive("route", QueuedTask{Text: "B2"}) {
		t.Fatal("failed to enqueue initial tasks")
	}
	first := d.FinishActive("route")
	if len(first.QueuedTasks) != 1 || first.QueuedTasks[0].Text != "B1" {
		t.Fatalf("first transfer = %#v, want B1", first.QueuedTasks)
	}
	if !d.TryEnqueueIfActive("route", QueuedTask{Text: "B3"}) {
		t.Fatal("failed to enqueue B3 while B1 owned the route")
	}
	second := d.FinishActive("route")
	third := d.FinishActive("route")
	if len(second.QueuedTasks) != 1 || second.QueuedTasks[0].Text != "B2" {
		t.Fatalf("second transfer = %#v, want B2", second.QueuedTasks)
	}
	if len(third.QueuedTasks) != 1 || third.QueuedTasks[0].Text != "B3" {
		t.Fatalf("third transfer = %#v, want B3", third.QueuedTasks)
	}
	d.FinishActive("route")
	if d.IsActive("route") {
		t.Fatal("route remained active after queue drained")
	}
}

func TestRouteDispatcher_OverlappingActiveOwners(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	d.MarkActive(routeID)
	d.MarkActive(routeID)
	d.Enqueue(routeID, QueuedTask{Text: "queued"})
	d.AddPendingPersist(routeID, func(context.Context) {})

	first := d.MarkDone(routeID)
	if len(first.QueuedTasks) != 0 {
		t.Fatalf("expected no queued tasks while an owner remains active, got %d", len(first.QueuedTasks))
	}
	if len(first.PendingPersists) != 0 {
		t.Fatalf("expected no pending persists while an owner remains active, got %d", len(first.PendingPersists))
	}
	if !d.IsActive(routeID) {
		t.Fatal("expected route to stay active until the last owner exits")
	}
	if d.Inject(routeID, InjectMessage{Text: "still active"}) != InjectAccepted {
		t.Fatal("expected inject to remain available while an owner remains active")
	}

	second := d.MarkDone(routeID)
	if d.IsActive(routeID) {
		t.Fatal("expected route to be inactive after the last owner exits")
	}
	if len(second.QueuedTasks) != 1 || second.QueuedTasks[0].Text != "queued" {
		t.Fatalf("expected queued task on final release, got %v", second.QueuedTasks)
	}
	if len(second.PendingPersists) != 1 {
		t.Fatalf("expected pending persist on final release, got %d", len(second.PendingPersists))
	}
}

func TestRouteDispatcher_MarkDoneReturnsNilWhenEmpty(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	d.MarkActive(routeID)
	result := d.MarkDone(routeID)
	if result.QueuedTasks != nil {
		t.Fatalf("expected nil queued tasks, got %v", result.QueuedTasks)
	}
	if result.PendingPersists != nil {
		t.Fatalf("expected nil pending persists, got %v", result.PendingPersists)
	}
}

func TestRouteDispatcher_ConcurrentInject(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	injectCh := d.MarkActive(routeID)

	const numMessages = 10
	var wg sync.WaitGroup
	wg.Add(numMessages)
	for i := 0; i < numMessages; i++ {
		go func() {
			defer wg.Done()
			d.Inject(routeID, InjectMessage{Text: "msg"})
		}()
	}
	wg.Wait()

	count := 0
	for {
		select {
		case <-injectCh:
			count++
		default:
			goto done
		}
	}
done:
	if count != numMessages {
		t.Errorf("expected %d messages, got %d", numMessages, count)
	}
}

func TestRouteDispatcher_ParallelBypass(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	d.MarkActive(routeID)

	// In parallel mode, the caller does not interact with the dispatcher
	// at all — it just starts a new stream directly. Verify the route
	// stays active without interference.
	if !d.IsActive(routeID) {
		t.Fatal("expected route to still be active")
	}

	d.MarkDone(routeID)
	if d.IsActive(routeID) {
		t.Fatal("expected route to be inactive after MarkDone")
	}
}

func TestRouteDispatcher_Cleanup(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())

	d.MarkActive("route-1")
	d.MarkDone("route-1")

	d.MarkActive("route-2")

	d.mu.RLock()
	initialCount := len(d.routes)
	d.mu.RUnlock()
	if initialCount != 2 {
		t.Fatalf("expected 2 routes, got %d", initialCount)
	}

	d.Cleanup(0)

	d.mu.RLock()
	afterCount := len(d.routes)
	d.mu.RUnlock()

	// route-1 is idle → cleaned up; route-2 is active → kept
	if afterCount != 1 {
		t.Fatalf("expected 1 route after cleanup, got %d", afterCount)
	}
	if d.IsActive("route-2") != true {
		t.Fatal("expected route-2 to still be active")
	}
}

func TestRouteDispatcher_InjectChannelFull(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	d.MarkActive(routeID)

	// Fill the inject channel to capacity
	for i := 0; i < injectChBuffer; i++ {
		if d.Inject(routeID, InjectMessage{Text: "fill"}) != InjectAccepted {
			t.Fatalf("expected inject %d to succeed", i)
		}
	}

	// Next inject should fail (channel full)
	if d.Inject(routeID, InjectMessage{Text: "overflow"}) != InjectUnavailable {
		t.Fatal("expected inject to fail when channel is full")
	}
}

func TestRouteDispatcher_QueueWhenInactive(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	// Enqueue without marking active — still stores in queue
	d.Enqueue(routeID, QueuedTask{Text: "task-1"})

	// MarkActive then MarkDone should return the queued task
	d.MarkActive(routeID)
	result := d.MarkDone(routeID)
	if len(result.QueuedTasks) != 1 {
		t.Fatalf("expected 1 queued task, got %d", len(result.QueuedTasks))
	}
}

func TestRouteDispatcher_MultipleMarkActive(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	ch1 := d.MarkActive(routeID)
	ch2 := d.MarkActive(routeID)

	if ch1 == nil || ch2 == nil {
		t.Fatal("expected non-nil channels")
	}

	_ = time.Now()
}

func TestRouteDispatcher_PendingPersistOrder(t *testing.T) {
	d := NewRouteDispatcher(slog.Default())
	routeID := "route-1"

	d.MarkActive(routeID)

	var order []string
	d.AddPendingPersist(routeID, func(_ context.Context) {
		order = append(order, "B")
	})
	d.AddPendingPersist(routeID, func(_ context.Context) {
		order = append(order, "C")
	})

	result := d.MarkDone(routeID)
	if len(result.PendingPersists) != 2 {
		t.Fatalf("expected 2 pending persists, got %d", len(result.PendingPersists))
	}

	// Execute persists — they should run in insertion order (B then C)
	for _, fn := range result.PendingPersists {
		fn(context.Background())
	}
	if len(order) != 2 || order[0] != "B" || order[1] != "C" {
		t.Errorf("expected [B C], got %v", order)
	}
}

func TestRouteDispatcherDeduplicatesEventForActiveOwnerLifetime(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.Default())
	const routeID = "route"
	injectCh := dispatcher.MarkActive(routeID)
	dispatcher.MarkActive(routeID)
	message := InjectMessage{
		Text: "redelivered",
		Source: conversation.InjectedMessageSource{
			EventID: "session-event",
		},
	}

	if result := dispatcher.Inject(routeID, message); result != InjectAccepted {
		t.Fatalf("first Inject() = %v, want accepted", result)
	}
	if result := dispatcher.Inject(routeID, message); result != InjectDuplicate {
		t.Fatalf("duplicate Inject() = %v, want duplicate", result)
	}
	assertInjectedMessageCount(t, injectCh, 1)

	dispatcher.MarkDone(routeID)
	if result := dispatcher.Inject(routeID, message); result != InjectDuplicate {
		t.Fatalf("Inject() while another owner remains = %v, want duplicate", result)
	}
	done := dispatcher.MarkDone(routeID)
	if len(done.InjectedMessages) != 1 || done.InjectedMessages[0].Source.EventID != "session-event" {
		t.Fatalf("completed injected messages = %#v, want session-event", done.InjectedMessages)
	}

	injectCh = dispatcher.MarkActive(routeID)
	if result := dispatcher.Inject(routeID, message); result != InjectAccepted {
		t.Fatalf("Inject() in recovery lifecycle = %v, want accepted", result)
	}
	assertInjectedMessageCount(t, injectCh, 1)
}

func TestRouteDispatcherRejectsUnconsumedInjectionOnStreamEnd(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.Default())
	dispatcher.MarkActive("route")
	persisted := make(chan error, 1)
	message := InjectMessage{
		Text: "not consumed",
		Source: conversation.InjectedMessageSource{
			EventID: "session-event",
		},
		OnPersisted: func(err error) { persisted <- err },
	}
	if result := dispatcher.Inject("route", message); result != InjectAccepted {
		t.Fatalf("Inject() = %v, want accepted", result)
	}
	dispatcher.MarkDone("route")
	if err := <-persisted; err == nil {
		t.Fatal("unconsumed injection reported successful persistence")
	}
}

func TestRouteDispatcherDoesNotClaimRejectedEvent(t *testing.T) {
	t.Parallel()

	dispatcher := NewRouteDispatcher(slog.Default())
	const routeID = "route"
	injectCh := dispatcher.MarkActive(routeID)
	for range injectChBuffer {
		if result := dispatcher.Inject(routeID, InjectMessage{Text: "fill"}); result != InjectAccepted {
			t.Fatalf("fill Inject() = %v, want accepted", result)
		}
	}
	message := InjectMessage{
		Text: "retry",
		Source: conversation.InjectedMessageSource{
			EventID: "retry-event",
		},
	}
	if result := dispatcher.Inject(routeID, message); result != InjectUnavailable {
		t.Fatalf("full-channel Inject() = %v, want unavailable", result)
	}
	<-injectCh
	if result := dispatcher.Inject(routeID, message); result != InjectAccepted {
		t.Fatalf("retry Inject() = %v, want accepted", result)
	}
}

func assertInjectedMessageCount(t *testing.T, injectCh <-chan InjectMessage, want int) {
	t.Helper()

	got := 0
	for {
		select {
		case <-injectCh:
			got++
		default:
			if got != want {
				t.Fatalf("injected messages = %d, want %d", got, want)
			}
			return
		}
	}
}
