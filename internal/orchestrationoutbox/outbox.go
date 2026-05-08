// Package orchestrationoutbox publishes committed orchestration run events to
// the orchestration bus. It is the durable outbox half of the Stage 1 NATS
// integration described in PLAN.md: the kernel writes events under transaction
// in Postgres (the source of truth) and this dispatcher fans them out to
// JetStream so other processes (workerd / verifyd / future blackboard) can
// react without polling.
//
// The dispatcher is intentionally simple:
//
//   - It polls orchestration_events for rows where published_at IS NULL.
//   - For each batch it converts the row into a bus envelope and publishes it.
//   - On successful publish the row is marked published_at = NOW().
//
// JetStream deduplicates by EventID (the row's UUID) so retries after crashes
// or partial failures are safe. When the bus is the in-process implementation
// the same flow still works and lets WatchRun consumers replace polling with
// push-based subscriptions.
package orchestrationoutbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/postgres/sqlc"
	"github.com/memohai/memoh/internal/orchestrationbus"
)

// Default tunables. The polling interval is intentionally generous (compared
// to the previous WatchRun 500ms loop) because the outbox sees every committed
// event regardless of consumer subscription state, so under steady load the
// only added latency is the polling delay.
const (
	DefaultBatchSize    = 64
	DefaultPollInterval = 250 * time.Millisecond
)

// Config controls dispatcher behaviour.
type Config struct {
	BatchSize    int
	PollInterval time.Duration
}

func (c Config) effectiveBatchSize() int32 {
	size := c.BatchSize
	if size <= 0 {
		size = DefaultBatchSize
	}
	if size > math.MaxInt32 {
		size = math.MaxInt32
	}
	return int32(size)
}

func (c Config) effectivePollInterval() time.Duration {
	if c.PollInterval <= 0 {
		return DefaultPollInterval
	}
	return c.PollInterval
}

// EventStore is the storage view consumed by the dispatcher. It is a small
// subset of *sqlc.Queries so tests can supply a fake.
type EventStore interface {
	ListUnpublishedOrchestrationRunEvents(ctx context.Context, limit int32) ([]sqlc.OrchestrationEvent, error)
	MarkOrchestrationRunEventPublished(ctx context.Context, id pgtype.UUID) error
}

// Dispatcher implements the run-event outbox loop. It is safe to construct
// even when no consumers are connected: the bus drops events for absent
// subscribers, while Postgres remains the durable record.
type Dispatcher struct {
	logger  *slog.Logger
	queries EventStore
	bus     orchestrationbus.RunEventPublisher
	cfg     Config

	wake chan struct{}
}

// New constructs a Dispatcher. The dispatcher requires a queries handle for
// scanning orchestration_events and a bus to publish to.
func New(logger *slog.Logger, queries EventStore, bus orchestrationbus.RunEventPublisher, cfg Config) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		logger:  logger.With(slog.String("component", "orchestration.outbox")),
		queries: queries,
		bus:     bus,
		cfg:     cfg,
		wake:    make(chan struct{}, 1),
	}
}

// Notify hints the dispatcher that new events may be available. The dispatcher
// already polls on its own, but callers (e.g. the kernel right after appending
// an event) can use this to drop the publish latency to roughly the time it
// takes to wake up the loop.
func (d *Dispatcher) Notify() {
	if d == nil {
		return
	}
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

// Run processes the outbox until ctx is cancelled. It is intended to be
// launched as a background goroutine. Errors during a tick are logged and the
// loop continues with the next tick.
func (d *Dispatcher) Run(ctx context.Context) {
	if d == nil || d.queries == nil || d.bus == nil {
		return
	}
	interval := d.cfg.effectivePollInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	d.logger.Info("orchestration outbox dispatcher started",
		slog.Duration("poll_interval", interval),
		slog.Int("batch_size", int(d.cfg.effectiveBatchSize())),
	)

	for {
		drained, err := d.tick(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			d.logger.Warn("orchestration outbox tick failed", slog.Any("error", err))
		}

		if drained {
			// Stay hot: a full batch was processed, more rows likely waiting.
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-d.wake:
		case <-ticker.C:
		}
	}
}

// tick processes one batch. It returns true when the batch was full, signalling
// the caller to loop immediately rather than wait for the next tick.
func (d *Dispatcher) tick(ctx context.Context) (bool, error) {
	limit := d.cfg.effectiveBatchSize()
	rows, err := d.queries.ListUnpublishedOrchestrationRunEvents(ctx, limit)
	if err != nil {
		return false, fmt.Errorf("list unpublished events: %w", err)
	}
	if len(rows) == 0 {
		return false, nil
	}

	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		env, err := envelopeFromRow(row)
		if err != nil {
			d.logger.Error("skip malformed event row",
				slog.String("event_id", row.ID.String()),
				slog.Any("error", err),
			)
			// We still mark the row published to avoid a poison-pill row
			// blocking the dispatcher forever; downstream consumers can
			// always re-derive state from Postgres.
			if markErr := d.queries.MarkOrchestrationRunEventPublished(ctx, row.ID); markErr != nil {
				return false, fmt.Errorf("mark malformed event published: %w", markErr)
			}
			continue
		}
		if err := d.bus.PublishRunEvent(ctx, env); err != nil {
			return false, fmt.Errorf("publish event %s: %w", env.EventID, err)
		}
		if err := d.queries.MarkOrchestrationRunEventPublished(ctx, row.ID); err != nil {
			return false, fmt.Errorf("mark event published: %w", err)
		}
	}
	return len(rows) == int(limit), nil
}

// envelopeFromRow translates a stored event row into a bus envelope. It is
// exported via the helper so tests can reuse it for fixtures.
func envelopeFromRow(row sqlc.OrchestrationEvent) (orchestrationbus.RunEventEnvelope, error) {
	seq, err := uint64FromInt64(row.Seq, "event seq")
	if err != nil {
		return orchestrationbus.RunEventEnvelope{}, err
	}
	aggVer, err := uint64FromInt64(row.AggregateVersion, "aggregate_version")
	if err != nil {
		return orchestrationbus.RunEventEnvelope{}, err
	}

	payload := decodePayload(row.Payload)

	return orchestrationbus.RunEventEnvelope{
		SchemaVersion:    orchestrationbus.EnvelopeVersion,
		EventID:          row.ID.String(),
		RunID:            row.RunID.String(),
		TaskID:           uuidString(row.TaskID),
		AttemptID:        uuidString(row.AttemptID),
		CheckpointID:     uuidString(row.CheckpointID),
		Seq:              seq,
		AggregateType:    row.AggregateType,
		AggregateID:      row.AggregateID.String(),
		AggregateVersion: aggVer,
		Type:             row.Type,
		CausationEventID: uuidString(row.CausationEventID),
		CorrelationID:    row.CorrelationID,
		IdempotencyKey:   row.IdempotencyKey,
		Payload:          payload,
		CreatedAt:        db.TimeFromPg(row.CreatedAt),
		PublishedAt:      time.Now().UTC(),
	}, nil
}

func uuidString(v pgtype.UUID) string {
	if !v.Valid {
		return ""
	}
	return v.String()
}

func decodePayload(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}

func uint64FromInt64(v int64, label string) (uint64, error) {
	if v < 0 {
		return 0, fmt.Errorf("orchestrationoutbox: %s is negative: %d", label, v)
	}
	return uint64(v), nil
}
