package flow

import (
	"context"
	"testing"
	"time"
)

func TestIdleCancelFinishWinsOverQueuedFire(t *testing.T) {
	ctx, idle := withIdleTimeout(context.Background(), &idleTimeoutOptions{baseTimeout: time.Hour})
	defer idle.Stop()

	idle.Finish()
	idle.fire()

	if idle.DidFire() {
		t.Fatal("idle timeout fired after terminal completion")
	}
	select {
	case <-ctx.Done():
		t.Fatalf("idle context canceled after terminal completion: %v", ctx.Err())
	default:
	}
}

func TestIdleCancelFireWinsOverFinish(t *testing.T) {
	ctx, idle := withIdleTimeout(context.Background(), &idleTimeoutOptions{baseTimeout: time.Hour})
	defer idle.Stop()

	idle.fire()
	idle.Finish()

	if !idle.DidFire() {
		t.Fatal("terminal completion erased an idle timeout that already fired")
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("idle timeout did not cancel its context")
	}
}

func TestIdleCancelResetDoesNotReactivateFinishedTimer(t *testing.T) {
	_, idle := withIdleTimeout(context.Background(), &idleTimeoutOptions{baseTimeout: time.Hour})
	defer idle.Stop()

	idle.Finish()
	idle.Reset()
	idle.fire()

	if idle.DidFire() {
		t.Fatal("Reset reactivated a finished idle timer")
	}
}
