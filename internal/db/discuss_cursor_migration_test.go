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

func TestDiscussCursorMigrationSeparatesEventAndLegacySourceCursors(t *testing.T) {
	t.Parallel()

	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0115_discuss_event_cursor.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0115_discuss_event_cursor.down.sql")

	if !strings.Contains(baseline, "consumed_event_cursor BIGINT NOT NULL DEFAULT 0") {
		t.Fatal("canonical schema is missing the durable discuss event cursor")
	}
	for _, required := range []string{
		"ADD COLUMN IF NOT EXISTS consumed_event_cursor BIGINT NOT NULL DEFAULT 0",
		"bot_session_events",
		"event_data->>'event_cursor'",
		"e.received_at_ms <= c.consumed_cursor",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0115 up migration is missing %q", required)
		}
	}
	if !strings.Contains(down, "DROP COLUMN IF EXISTS consumed_event_cursor") {
		t.Fatal("0115 down migration does not remove the event cursor")
	}
	if strings.Contains(down, "DROP COLUMN IF EXISTS consumed_cursor") {
		t.Fatal("0115 down migration removes the legacy source cursor needed by the old binary")
	}
}

func TestDiscussCursorMigrationPostgresPath(t *testing.T) {
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
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "discuss_cursor_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE bot_sessions (id UUID PRIMARY KEY);
CREATE TABLE bot_session_events (
  id UUID PRIMARY KEY,
  session_id UUID NOT NULL,
  event_data JSONB NOT NULL,
  received_at_ms BIGINT NOT NULL
);
CREATE TABLE bot_session_discuss_cursors (
  session_id UUID NOT NULL,
  scope_key TEXT NOT NULL DEFAULT 'default',
  route_id UUID,
  source TEXT NOT NULL DEFAULT '',
  consumed_cursor BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (session_id, scope_key)
);
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	bindTeamMigrationFixture(t, ctx, tx, "bot_session_events", "bot_session_discuss_cursors")
	if _, err := tx.Exec(ctx, `CREATE UNIQUE INDEX discuss_cursor_team_key ON bot_session_discuss_cursors (team_id, session_id, scope_key)`); err != nil {
		t.Fatalf("add team cursor key: %v", err)
	}

	sessionID := uuid.NewString()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_sessions (id) VALUES ($1)`, sessionID); err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_events (id, session_id, event_data, received_at_ms) VALUES
  ($2, $1, '{"received_at_ms":100}'::jsonb, 100),
  ($3, $1, '{"received_at_ms":200,"event_cursor":25}'::jsonb, 200),
  ($4, $1, '{"received_at_ms":300,"event_cursor":"invalid"}'::jsonb, 300),
  ($5, $1, '{"received_at_ms":400,"event_cursor":9007199254740992}'::jsonb, 400),
  ($6, $1, '{"received_at_ms":10000,"event_cursor":30}'::jsonb, 10000)
`, sessionID, uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()); err != nil {
		t.Fatalf("insert legacy session events: %v", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_session_discuss_cursors (session_id, consumed_cursor)
VALUES ($1, 9000)
`, sessionID); err != nil {
		t.Fatalf("insert legacy discuss cursor: %v", err)
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0115_discuss_event_cursor.up.sql")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0115 up: %v", err)
	}
	var sourceCursor, eventCursor int64
	if err := tx.QueryRow(ctx, `
SELECT consumed_cursor, consumed_event_cursor
FROM bot_session_discuss_cursors
WHERE session_id = $1 AND scope_key = 'default'
`, sessionID).Scan(&sourceCursor, &eventCursor); err != nil {
		t.Fatalf("read upgraded cursor: %v", err)
	}
	if sourceCursor != 9000 || eventCursor != 25 {
		t.Fatalf("upgraded cursors = source:%d event:%d, want source:9000 event:25", sourceCursor, eventCursor)
	}

	pgSessionID, err := ParseUUID(sessionID)
	if err != nil {
		t.Fatalf("parse session id: %v", err)
	}
	row, err := sqlc.New(tx).UpsertSessionDiscussCursor(ctx, sqlc.UpsertSessionDiscussCursorParams{
		SessionID:           pgSessionID,
		ScopeKey:            "default",
		Source:              "telegram",
		ConsumedCursor:      12000,
		ConsumedEventCursor: 40,
	})
	if err != nil {
		t.Fatalf("write new-version cursor state: %v", err)
	}
	if row.ConsumedCursor != 12000 || row.ConsumedEventCursor != 40 {
		t.Fatalf("upserted cursors = source:%d event:%d, want source:12000 event:40", row.ConsumedCursor, row.ConsumedEventCursor)
	}
	down := readEmbeddedMigration(t, "postgres/migrations/0115_discuss_event_cursor.down.sql")
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0115 down: %v", err)
	}
	if err := tx.QueryRow(ctx, `
SELECT consumed_cursor
FROM bot_session_discuss_cursors
WHERE session_id = $1 AND scope_key = 'default'
`, sessionID).Scan(&sourceCursor); err != nil {
		t.Fatalf("read downgraded cursor: %v", err)
	}
	if sourceCursor != 12000 {
		t.Fatalf("downgraded legacy source cursor = %d, want 12000", sourceCursor)
	}
}
