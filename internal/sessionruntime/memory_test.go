package sessionruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/conversation"
)

func TestMemoryBackendRejectsUnserializableSnapshotWithoutCorruptingState(t *testing.T) {
	backend := NewMemoryBackend()
	key := Key{BotID: "bot-1", SessionID: "session-1"}
	if _, changed, err := backend.Update(context.Background(), key, func(Snapshot, bool) (Snapshot, bool, error) {
		return Snapshot{BotID: key.BotID, SessionID: key.SessionID, Seq: 1, Queue: []QueuedRunView{}}, true, nil
	}); err != nil || !changed {
		t.Fatalf("seed snapshot: changed=%v err=%v", changed, err)
	}

	if _, changed, err := backend.Update(context.Background(), key, func(snapshot Snapshot, _ bool) (Snapshot, bool, error) {
		snapshot.Seq = 2
		snapshot.CurrentRunView = &CurrentRunView{
			StreamID: "stream-1",
			Status:   RunStatusRunning,
			Messages: []conversation.UIMessage{{ID: 1, Type: conversation.UIMessageTool, Input: make(chan struct{})}},
		}
		return snapshot, true, nil
	}); err == nil || changed {
		t.Fatalf("unserializable update: changed=%v err=%v", changed, err)
	}

	snapshot, ok, err := backend.Load(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("load preserved snapshot: ok=%v err=%v", ok, err)
	}
	if snapshot.Seq != 1 || snapshot.CurrentRunView != nil {
		t.Fatalf("snapshot corrupted after rejected update: %#v", snapshot)
	}
}

func TestMemoryBackendRejectsCanceledOperations(t *testing.T) {
	backend := NewMemoryBackend()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	key := Key{BotID: "bot-1", SessionID: "session-1"}

	if _, _, err := backend.Load(ctx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("Load error = %v, want context.Canceled", err)
	}
	if _, _, err := backend.Update(ctx, key, func(snapshot Snapshot, _ bool) (Snapshot, bool, error) {
		return snapshot, true, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Update error = %v, want context.Canceled", err)
	}
	if _, err := backend.Subscribe(ctx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("Subscribe error = %v, want context.Canceled", err)
	}
}

func TestMemoryBackendDoesNotImplementDistributedCoordination(t *testing.T) {
	t.Parallel()

	if _, ok := any(NewMemoryBackend()).(DistributedBackend); ok {
		t.Fatal("MemoryBackend unexpectedly implements DistributedBackend")
	}
}
