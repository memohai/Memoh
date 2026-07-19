package db

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSessionEventIngestMigrationAddsDurableCursorAndLeaseContracts(t *testing.T) {
	t.Parallel()

	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	baselineDown := readEmbeddedMigration(t, "postgres/migrations/0001_init.down.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0117_session_event_ingest.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0117_session_event_ingest.down.sql")
	queries, err := os.ReadFile("../../db/postgres/queries/session_events.sql")
	if err != nil {
		t.Fatalf("read session event queries: %v", err)
	}

	for name, source := range map[string]string{
		"canonical schema": baseline,
		"0117 migration":   up,
	} {
		for _, required := range []string{
			"bot_session_event_cursor_seq",
			"MAXVALUE 9007199254740991",
			"delivery_claim_token UUID",
			"delivery_claimed_until TIMESTAMPTZ",
			"delivery_completed_at TIMESTAMPTZ",
		} {
			if !strings.Contains(source, required) {
				t.Fatalf("%s is missing %q", name, required)
			}
		}
	}
	for _, required := range []string{
		"event_data->>'event_cursor'",
		"received_at_ms",
		"EXTRACT(EPOCH FROM clock_timestamp()) * 1000",
		"setval('bot_session_event_cursor_seq'",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0117 migration is missing cursor seed contract %q", required)
		}
	}
	if !strings.Contains(baselineDown, "DROP SEQUENCE IF EXISTS bot_session_event_cursor_seq") {
		t.Fatal("canonical down migration does not remove the event cursor sequence")
	}
	for _, required := range []string{
		"DROP COLUMN IF EXISTS delivery_completed_at",
		"DROP COLUMN IF EXISTS delivery_claimed_until",
		"DROP COLUMN IF EXISTS delivery_claim_token",
		"DROP SEQUENCE IF EXISTS bot_session_event_cursor_seq",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0117 down migration is missing %q", required)
		}
	}

	querySource := string(queries)
	for _, required := range []string{
		"-- name: NextSessionEventCursor :one",
		"nextval('bot_session_event_cursor_seq')",
		"-- name: ClaimSessionEventDelivery :one",
		"locked.delivery_claimed_until <= clock_timestamp()",
		"-- name: RenewSessionEventDelivery :one",
		"-- name: CompleteSessionEventDelivery :execrows",
		"AND event.delivery_claimed_until > clock_timestamp()",
		"delivery_completed_at = clock_timestamp()",
		"-- name: ReleaseSessionEventDelivery :execrows",
	} {
		if !strings.Contains(querySource, required) {
			t.Fatalf("session event queries are missing %q", required)
		}
	}
	snapshotStart := strings.Index(querySource, "-- name: ListSessionEventsBySession :many")
	if snapshotStart < 0 {
		t.Fatal("session event queries are missing ListSessionEventsBySession")
	}
	snapshotTail := querySource[snapshotStart:]
	if nextQuery := strings.Index(snapshotTail[1:], "-- name:"); nextQuery >= 0 {
		snapshotTail = snapshotTail[:nextQuery+1]
	}
	for _, required := range []string{
		"event.delivery_completed_at IS NOT NULL",
		"FROM bot_history_messages history",
		"history.event_id = event.id",
		"history.session_id = event.session_id",
		"history.role = 'user'",
	} {
		if !strings.Contains(snapshotTail, required) {
			t.Fatalf("ListSessionEventsBySession is missing projectable-event contract %q", required)
		}
	}
}

func TestSessionEventIngestMigrationPostgresPath(t *testing.T) {
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

	schema := "session_event_ingest_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
  received_at_ms BIGINT NOT NULL
);
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  session_id UUID,
  event_id UUID,
  role TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  turn_id UUID,
  turn_message_seq BIGINT
);
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	bindTeamMigrationFixture(t, ctx, tx, "bot_session_events", "bot_history_messages")

	const legacyCursor = int64(1_800_000_000_000)
	eventID := uuid.NewString()
	sessionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events (id, session_id, event_kind, event_data, received_at_ms)
VALUES ($1, $2, 'message', jsonb_build_object('event_cursor', $3::bigint), $3)
`, eventID, sessionID, legacyCursor); err != nil {
		t.Fatalf("insert legacy event: %v", err)
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0117_session_event_ingest.up.sql")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0117 up: %v", err)
	}
	queries := sqlc.New(tx)
	firstCursor, err := queries.NextSessionEventCursor(ctx)
	if err != nil {
		t.Fatalf("allocate first cursor: %v", err)
	}
	secondCursor, err := queries.NextSessionEventCursor(ctx)
	if err != nil {
		t.Fatalf("allocate second cursor: %v", err)
	}
	if firstCursor <= legacyCursor || secondCursor != firstCursor+1 || secondCursor > 1<<53-1 {
		t.Fatalf("allocated cursors = %d/%d after legacy %d", firstCursor, secondCursor, legacyCursor)
	}

	pgEventID, err := ParseUUID(eventID)
	if err != nil {
		t.Fatalf("parse event id: %v", err)
	}
	firstToken := pgUUID(t, uuid.NewString())
	secondToken := pgUUID(t, uuid.NewString())
	claim := func(token pgtype.UUID) error {
		_, claimErr := queries.ClaimSessionEventDelivery(ctx, sqlc.ClaimSessionEventDeliveryParams{
			EventID: pgEventID, ClaimToken: token, LeaseMs: time.Minute.Milliseconds(),
		})
		return claimErr
	}
	if err := claim(firstToken); err != nil {
		t.Fatalf("claim first delivery: %v", err)
	}
	if err := claim(secondToken); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("competing fresh claim error = %v, want no rows", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE bot_session_events SET delivery_claimed_until = now() - INTERVAL '1 second' WHERE id = $1
`, eventID); err != nil {
		t.Fatalf("expire first claim: %v", err)
	}
	if err := claim(secondToken); err != nil {
		t.Fatalf("reclaim expired delivery: %v", err)
	}
	rows, err := queries.ReleaseSessionEventDelivery(ctx, sqlc.ReleaseSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: firstToken,
	})
	if err != nil || rows != 0 {
		t.Fatalf("release stale claim = %d, %v, want 0/nil", rows, err)
	}
	rows, err = queries.CompleteSessionEventDelivery(ctx, sqlc.CompleteSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: secondToken,
	})
	if err != nil || rows != 0 {
		t.Fatalf("complete claim without history = %d, %v, want 0/nil", rows, err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (id, session_id, event_id, role, metadata)
VALUES ($1, $2, $3, 'user', '{}'::jsonb)
`, uuid.NewString(), sessionID, eventID); err != nil {
		t.Fatalf("insert durable delivery history: %v", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE bot_session_events SET delivery_claimed_until = now() - INTERVAL '1 second' WHERE id = $1
`, eventID); err != nil {
		t.Fatalf("expire current claim: %v", err)
	}
	if _, err := queries.RenewSessionEventDelivery(ctx, sqlc.RenewSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: secondToken, LeaseMs: time.Minute.Milliseconds(),
	}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("renew expired claim error = %v, want no rows", err)
	}
	rows, err = queries.CompleteSessionEventDelivery(ctx, sqlc.CompleteSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: secondToken,
	})
	if err != nil || rows != 0 {
		t.Fatalf("complete expired claim = %d, %v, want 0/nil", rows, err)
	}
	if err := claim(secondToken); err != nil {
		t.Fatalf("reclaim expired current delivery: %v", err)
	}
	rows, err = queries.CompleteSessionEventDelivery(ctx, sqlc.CompleteSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: secondToken,
	})
	if err != nil || rows != 1 {
		t.Fatalf("complete current claim = %d, %v, want 1/nil", rows, err)
	}
	if err := claim(firstToken); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("claim completed delivery error = %v, want no rows", err)
	}

	for index, eventKind := range []string{"edit", "delete", "service"} {
		passiveEventID := uuid.NewString()
		if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events (id, session_id, event_kind, event_data, received_at_ms)
VALUES ($1, $2, $3, '{}'::jsonb, $4)
`, passiveEventID, sessionID, eventKind, legacyCursor+int64(index)+1); err != nil {
			t.Fatalf("insert %s event: %v", eventKind, err)
		}
		pgPassiveEventID, err := ParseUUID(passiveEventID)
		if err != nil {
			t.Fatalf("parse %s event id: %v", eventKind, err)
		}
		if _, err := queries.ClaimSessionEventDelivery(ctx, sqlc.ClaimSessionEventDeliveryParams{
			EventID: pgPassiveEventID, ClaimToken: firstToken, LeaseMs: time.Minute.Milliseconds(),
		}); err != nil {
			t.Fatalf("claim %s event delivery: %v", eventKind, err)
		}
		rows, err = queries.CompleteSessionEventDelivery(ctx, sqlc.CompleteSessionEventDeliveryParams{
			EventID: pgPassiveEventID, ClaimToken: firstToken,
		})
		if err != nil || rows != 1 {
			t.Fatalf("complete %s event without history = %d, %v, want 1/nil", eventKind, rows, err)
		}
	}
}

func pgUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	parsed, err := ParseUUID(value)
	if err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return parsed
}
