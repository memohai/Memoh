package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSessionEventDeliveryStatePendingHistoryPostgresPath(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "session_event_delivery_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE bot_session_events (
  id UUID PRIMARY KEY,
  session_id UUID NOT NULL,
  event_kind TEXT NOT NULL,
  event_data JSONB NOT NULL,
  external_message_id TEXT,
  delivery_completed_at TIMESTAMPTZ
);
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  session_id UUID,
  event_id UUID,
  role TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  turn_id UUID,
  turn_message_seq BIGINT NOT NULL,
  turn_visible BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
`); err != nil {
		t.Fatalf("create delivery schema: %v", err)
	}
	bindTeamMigrationFixture(t, ctx, tx, "bot_session_events", "bot_history_messages")

	sessionID := uuid.NewString()
	eventID := uuid.NewString()
	messageID := uuid.NewString()
	turnID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events (id, session_id, event_kind, event_data, external_message_id)
VALUES ($1, $2, 'message', '{"session_id":"ignored","message_id":"delivery-1"}'::jsonb, 'delivery-1')
`, eventID, sessionID); err != nil {
		t.Fatalf("insert session event: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (id, session_id, event_id, role, metadata, turn_id, turn_message_seq)
VALUES ($1, $2, $3, 'user', '{"pipeline_delivery_state":"pending"}'::jsonb, $4, 0)
`, messageID, sessionID, eventID, turnID); err != nil {
		t.Fatalf("insert pending history: %v", err)
	}

	pgEventID, err := ParseUUID(eventID)
	if err != nil {
		t.Fatalf("parse event id: %v", err)
	}
	row, err := sqlc.New(tx).GetSessionEventDeliveryState(ctx, pgEventID)
	if err != nil {
		t.Fatalf("read pending delivery: %v", err)
	}
	if row.HistoryMessageID.String() != messageID || !row.HistoryDeliveryPending || row.HistoryPersisted {
		t.Fatalf("pending delivery = message:%s pending:%t complete:%t, want %s/true/false", row.HistoryMessageID.String(), row.HistoryDeliveryPending, row.HistoryPersisted, messageID)
	}

	responseID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (id, session_id, role, turn_id, turn_message_seq)
VALUES ($1, $2, 'assistant', $3, 1);
`, responseID, sessionID, turnID); err != nil {
		t.Fatalf("insert queued response: %v", err)
	}
	row, err = sqlc.New(tx).GetSessionEventDeliveryState(ctx, pgEventID)
	if err != nil {
		t.Fatalf("read completed delivery: %v", err)
	}
	if !row.HistoryDeliveryPending || !row.HistoryPersisted {
		t.Fatal("pending delivery with assistant response lost its pending or persisted state")
	}

	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = $1`, responseID); err != nil {
		t.Fatalf("delete queued response: %v", err)
	}
	pgMessageID, err := ParseUUID(messageID)
	if err != nil {
		t.Fatalf("parse message id: %v", err)
	}
	updated, err := sqlc.New(tx).CompletePendingHistoryDelivery(ctx, pgMessageID)
	if err != nil {
		t.Fatalf("complete pending delivery: %v", err)
	}
	if updated != 1 {
		t.Fatalf("complete pending delivery updated %d messages", updated)
	}
	row, err = sqlc.New(tx).GetSessionEventDeliveryState(ctx, pgEventID)
	if err != nil {
		t.Fatalf("read immediate delivery: %v", err)
	}
	if row.HistoryDeliveryPending || !row.HistoryPersisted {
		t.Fatal("completed linked history did not clear its pending state")
	}
}
