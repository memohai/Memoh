package inbound

import (
	"log/slog"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

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
