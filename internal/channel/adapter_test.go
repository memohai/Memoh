package channel

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestBaseConnectionStopIsIdempotent(t *testing.T) {
	t.Parallel()

	var stops atomic.Int32
	conn := NewConnection(ChannelConfig{ID: "cfg-1", BotID: "bot-1", ChannelType: ChannelType("test")}, func(context.Context) error {
		stops.Add(1)
		return nil
	})

	if err := conn.Stop(context.Background()); err != nil {
		t.Fatalf("first stop failed: %v", err)
	}
	if err := conn.Stop(context.Background()); err != nil {
		t.Fatalf("second stop failed: %v", err)
	}
	if got := stops.Load(); got != 1 {
		t.Fatalf("expected stop func to run once, got %d", got)
	}
	if conn.Running() {
		t.Fatal("expected connection to be stopped")
	}
}
