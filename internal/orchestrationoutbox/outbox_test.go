package orchestrationoutbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/orchestrationbus"
)

type fakeEventStore struct {
	mu        sync.Mutex
	pending   []sqlc.OrchestrationEvent
	published map[string]bool
}

func newFakeEventStore(events ...sqlc.OrchestrationEvent) *fakeEventStore {
	return &fakeEventStore{
		pending:   events,
		published: map[string]bool{},
	}
}

func (s *fakeEventStore) ListUnpublishedOrchestrationRunEvents(_ context.Context, limit int32) ([]sqlc.OrchestrationEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil, nil
	}
	if int(limit) >= len(s.pending) {
		out := s.pending
		s.pending = nil
		return out, nil
	}
	out := s.pending[:limit]
	s.pending = s.pending[limit:]
	return out, nil
}

func (s *fakeEventStore) MarkOrchestrationRunEventPublished(_ context.Context, id pgtype.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published[id.String()] = true
	return nil
}

func makeRow(t *testing.T, runID uuid.UUID, seq int64, eventType string) sqlc.OrchestrationEvent {
	t.Helper()
	return sqlc.OrchestrationEvent{
		ID:               pgUUID(t, uuid.New()),
		RunID:            pgUUID(t, runID),
		Seq:              seq,
		AggregateType:    "run",
		AggregateID:      pgUUID(t, runID),
		AggregateVersion: seq,
		Type:             eventType,
		CorrelationID:    runID.String(),
		Payload:          []byte(`{"foo":"bar"}`),
		CreatedAt:        pgTime(time.Unix(0, seq)),
	}
}

func pgUUID(t *testing.T, id uuid.UUID) pgtype.UUID {
	t.Helper()
	var v pgtype.UUID
	if err := v.Scan(id.String()); err != nil {
		t.Fatalf("uuid scan: %v", err)
	}
	return v
}

func pgTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func TestDispatcherTickPublishesAndMarks(t *testing.T) {
	runA := uuid.New()
	runB := uuid.New()
	rows := []sqlc.OrchestrationEvent{
		makeRow(t, runA, 1, "run.event.task.created"),
		makeRow(t, runA, 2, "run.event.task.completed"),
		makeRow(t, runB, 1, "run.event.task.failed"),
	}

	store := newFakeEventStore(rows...)
	bus := orchestrationbus.NewInMemoryBus(0)
	t.Cleanup(func() { _ = bus.Close() })

	subA, err := bus.SubscribeRunEvents(context.Background(), runA.String())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	subB, err := bus.SubscribeRunEvents(context.Background(), runB.String())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	disp := New(nil, store, bus, Config{BatchSize: 10})
	drained, err := disp.tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if drained {
		t.Fatalf("expected drained=false when batch not full, got true")
	}

	for i, want := range []string{"run.event.task.created", "run.event.task.completed"} {
		select {
		case env := <-subA.Events():
			if env.Type != want {
				t.Fatalf("subA event %d type mismatch: want %s got %s", i, want, env.Type)
			}
			if env.Seq == 0 {
				t.Fatalf("subA event %d missing seq", i)
			}
			if env.SchemaVersion != orchestrationbus.EnvelopeVersion {
				t.Fatalf("subA event %d schema version mismatch: %d", i, env.SchemaVersion)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for subA event %d", i)
		}
	}

	select {
	case env := <-subB.Events():
		if env.Type != "run.event.task.failed" {
			t.Fatalf("subB type mismatch: %s", env.Type)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for subB event")
	}

	if got := len(store.published); got != 3 {
		t.Fatalf("expected 3 published rows, got %d", got)
	}
}

func TestDispatcherSkipsMalformedRow(t *testing.T) {
	bad := sqlc.OrchestrationEvent{
		ID:               pgUUID(t, uuid.New()),
		RunID:            pgUUID(t, uuid.New()),
		Seq:              -1,
		AggregateType:    "run",
		AggregateID:      pgUUID(t, uuid.New()),
		AggregateVersion: 1,
		Type:             "run.event.malformed",
		Payload:          []byte("{}"),
	}
	store := newFakeEventStore(bad)
	bus := orchestrationbus.NewInMemoryBus(0)
	t.Cleanup(func() { _ = bus.Close() })

	disp := New(nil, store, bus, Config{BatchSize: 10})
	if _, err := disp.tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if !store.published[bad.ID.String()] {
		t.Fatalf("expected malformed row to be marked published as poison-pill")
	}
}

type errBus struct{}

func (errBus) PublishRunEvent(context.Context, orchestrationbus.RunEventEnvelope) error {
	return errors.New("boom")
}

func TestDispatcherTickReturnsErrorOnPublishFailure(t *testing.T) {
	rows := []sqlc.OrchestrationEvent{
		makeRow(t, uuid.New(), 1, "run.event.task.created"),
	}
	store := newFakeEventStore(rows...)

	disp := New(nil, store, errBus{}, Config{BatchSize: 10})
	if _, err := disp.tick(context.Background()); err == nil {
		t.Fatalf("expected publish error to surface")
	}
	if len(store.published) != 0 {
		t.Fatalf("event should not be marked published when publish fails")
	}
}

func TestDispatcherNotifyWakesLoop(t *testing.T) {
	runA := uuid.New()
	rows := []sqlc.OrchestrationEvent{
		makeRow(t, runA, 1, "run.event.task.created"),
	}
	store := newFakeEventStore(rows...)
	bus := orchestrationbus.NewInMemoryBus(0)
	t.Cleanup(func() { _ = bus.Close() })

	sub, err := bus.SubscribeRunEvents(context.Background(), runA.String())
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	disp := New(nil, store, bus, Config{BatchSize: 10, PollInterval: time.Hour})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		disp.Run(ctx)
	}()

	disp.Notify()
	select {
	case env := <-sub.Events():
		if env.Type != "run.event.task.created" {
			t.Fatalf("wrong event type: %s", env.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("notify did not wake dispatcher within 2s")
	}

	cancel()
	<-done
}
