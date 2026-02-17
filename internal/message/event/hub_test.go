package event

import (
	"testing"
	"time"
)

func TestHubPublishScopedByBotID(t *testing.T) {
	hub := NewHub()
	_, botAStream, cancelA := hub.Subscribe("bot-a", 8)
	defer cancelA()
	_, botBStream, cancelB := hub.Subscribe("bot-b", 8)
	defer cancelB()

	hub.Publish(Event{Type: TypeMessageCreated, BotID: "bot-a"})

	select {
	case <-botAStream:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected event for bot-a subscriber")
	}

	select {
	case <-botBStream:
		t.Fatalf("did not expect bot-b subscriber to receive bot-a event")
	case <-time.After(120 * time.Millisecond):
	}
}

func TestHubCancelUnsubscribe(t *testing.T) {
	hub := NewHub()
	_, stream, cancel := hub.Subscribe("bot-a", 8)
	cancel()

	select {
	case _, ok := <-stream:
		if ok {
			t.Fatalf("expected stream to be closed after cancel")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for stream close")
	}
}

func TestHubSlowSubscriberDoesNotBlockPublish(t *testing.T) {
	hub := NewHub()
	_, stream, cancel := hub.Subscribe("bot-a", 1)
	defer cancel()

	hub.Publish(Event{Type: TypeMessageCreated, BotID: "bot-a"})
	hub.Publish(Event{Type: TypeMessageCreated, BotID: "bot-a"})
	hub.Publish(Event{Type: TypeMessageCreated, BotID: "bot-a"})

	select {
	case <-stream:
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("expected at least one event in buffer")
	}
}
