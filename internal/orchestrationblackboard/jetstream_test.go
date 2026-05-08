package orchestrationblackboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// jetStreamTestStore returns a JetStreamStore backed by the NATS server at
// TEST_NATS_URL. The bucket name is suffixed with the test name so
// concurrent tests do not interfere. When TEST_NATS_URL is unset the
// caller is asked to skip; this mirrors the TEST_POSTGRES_DSN pattern
// already used by other Postgres-backed tests in the tree.
func jetStreamTestStore(t *testing.T) *JetStreamStore {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("TEST_NATS_URL"))
	if url == "" {
		t.Skip("TEST_NATS_URL not set; skipping JetStream blackboard tests")
	}
	bucket := fmt.Sprintf("MEMOH_BB_TEST_%d", time.Now().UnixNano())
	store, err := NewJetStreamStore(context.Background(), slog.Default(), JetStreamConfig{
		URL:    url,
		Bucket: bucket,
	})
	if err != nil {
		t.Fatalf("NewJetStreamStore: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if store.js != nil {
			_ = store.js.DeleteKeyValue(ctx, bucket)
		}
		_ = store.Close()
	})
	return store
}

func TestJetStreamStorePutGetRoundTrip(t *testing.T) {
	store := jetStreamTestStore(t)
	ctx := context.Background()
	key := RunKey("run-jstest", NamespaceContext, "goal")
	if _, err := store.Put(ctx, key, mustValue(t, map[string]string{"text": "hello"})); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := store.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Value.WriterID != "orch-1" {
		t.Fatalf("writer round trip: %q", got.Value.WriterID)
	}
}

func TestJetStreamStoreCASInsertAndUpdate(t *testing.T) {
	store := jetStreamTestStore(t)
	ctx := context.Background()
	key := TaskKey("task-jstest", NamespaceResult, "summary")

	if _, err := store.CompareAndSwap(ctx, key, 1, mustValue(t, "data")); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("expected revision conflict on missing key with non-zero expected; got %v", err)
	}

	rev, err := store.CompareAndSwap(ctx, key, 0, mustValue(t, "data"))
	if err != nil {
		t.Fatalf("insert via CAS: %v", err)
	}
	if _, err := store.CompareAndSwap(ctx, key, 0, mustValue(t, "data2")); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("CAS with stale expected should conflict; got %v", err)
	}
	if _, err := store.CompareAndSwap(ctx, key, rev, mustValue(t, "data3")); err != nil {
		t.Fatalf("CAS with current revision should succeed: %v", err)
	}
}

func TestJetStreamStoreList(t *testing.T) {
	store := jetStreamTestStore(t)
	ctx := context.Background()
	keys := []Key{
		TaskKey("task-1", NamespaceResult, "summary"),
		TaskKey("task-1", NamespaceResult, "details"),
		TaskKey("task-2", NamespaceResult, "summary"),
		TaskKey("task-1", NamespaceArtifacts, "doc"),
	}
	for _, k := range keys {
		if _, err := store.Put(ctx, k, mustValue(t, "x")); err != nil {
			t.Fatalf("Put %s: %v", k.String(), err)
		}
	}
	listed, err := store.List(ctx, TaskKey("task-1", NamespaceResult))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := len(listed), 2; got != want {
		t.Fatalf("List returned %d, want %d (entries=%v)", got, want, listed)
	}
}

func TestJetStreamStoreDelete(t *testing.T) {
	store := jetStreamTestStore(t)
	ctx := context.Background()
	key := TaskKey("task-jstest", NamespaceProgress, "step")
	if _, err := store.Put(ctx, key, mustValue(t, "x")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := store.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete should be ErrNotFound, got %v", err)
	}
}

func TestFactoryFallsBackToInMemory(t *testing.T) {
	store, err := New(context.Background(), slog.Default(), FactoryConfig{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, ok := store.(*InMemoryStore); !ok {
		t.Fatalf("empty url should yield InMemoryStore, got %T", store)
	}
}
