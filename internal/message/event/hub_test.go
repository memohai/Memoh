package event

import (
	"testing"
	"time"
)

func TestHubPublishScopedByBotID(t *testing.T) {
	hub := NewHub()
	subA, cancelA := hub.Subscribe("bot-a", 8)
	defer cancelA()
	subB, cancelB := hub.Subscribe("bot-b", 8)
	defer cancelB()

	hub.Publish(Event{Type: EventTypeMessageCreated, BotID: "bot-a"})

	select {
	case <-subA.Events:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected event for bot-a subscriber")
	}

	select {
	case <-subB.Events:
		t.Fatalf("did not expect bot-b subscriber to receive bot-a event")
	case <-time.After(120 * time.Millisecond):
	}
}

func TestHubCancelUnsubscribe(t *testing.T) {
	hub := NewHub()
	sub, cancel := hub.Subscribe("bot-a", 8)
	cancel()

	select {
	case _, ok := <-sub.Events:
		if ok {
			t.Fatalf("expected stream to be closed after cancel")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for stream close")
	}
}

func TestHubSlowSubscriberDropsEventsWithoutBlocking(t *testing.T) {
	hub := NewHub()
	sub, cancel := hub.Subscribe("bot-a", 2)
	defer cancel()

	published := make(chan struct{})
	go func() {
		defer close(published)
		for i := 0; i < 5; i++ {
			hub.Publish(Event{Type: EventTypeMessageCreated, BotID: "bot-a"})
		}
	}()

	select {
	case <-published:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("Publish blocked on full subscriber buffer")
	}

	select {
	case <-sub.Events:
	default:
		t.Fatalf("expected at least one event in buffer")
	}

	if got := sub.DroppedSinceLastRead(); got != 3 {
		t.Fatalf("DroppedSinceLastRead = %d, want 3", got)
	}
	if got := sub.DroppedSinceLastRead(); got != 0 {
		t.Fatalf("DroppedSinceLastRead after reset = %d, want 0", got)
	}
}
