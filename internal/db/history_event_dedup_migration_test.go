package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHistoryEventDedupMigrationContract(t *testing.T) {
	t.Parallel()

	const rollbackMarker = "_migration_0115_history_event_dedup"
	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0115_history_event_dedup.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0115_history_event_dedup.down.sql")

	for name, sql := range map[string]string{"baseline": baseline, "0115 up": up} {
		for _, required := range []string{
			"idx_bot_history_messages_event_id_unique",
			"ON bot_history_messages(event_id)",
			"WHERE event_id IS NOT NULL",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s is missing %q", name, required)
			}
		}
	}
	for _, required := range []string{
		"ROW_NUMBER() OVER",
		"PARTITION BY event_id",
		rollbackMarker,
		"jsonb_set",
		"event_id = NULL",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0115 up is missing duplicate-link cleanup %q", required)
		}
	}
	if !strings.Contains(down, "DROP INDEX IF EXISTS idx_bot_history_messages_event_id_unique") {
		t.Fatal("0115 down does not remove the event link uniqueness constraint")
	}
	for _, required := range []string{rollbackMarker, "event_id = (", "metadata -"} {
		if !strings.Contains(down, required) {
			t.Fatalf("0115 down is missing duplicate-link restoration %q", required)
		}
	}
	if strings.Contains(baseline, rollbackMarker) {
		t.Fatal("canonical schema contains the migration-only rollback marker")
	}
}

func TestHistoryEventDedupMigrationPostgresPath(t *testing.T) {
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

	schema := "history_event_dedup_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  event_id UUID,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL
);
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	bindTeamMigrationFixture(t, ctx, tx, "bot_history_messages")

	eventID := uuid.NewString()
	firstID := uuid.NewString()
	secondID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
INSERT INTO bot_history_messages (id, event_id, metadata, created_at) VALUES
  ($1, $4, '{}'::jsonb, '2026-01-01T00:00:00Z'),
  ($2, $4, '{"keep":"value"}'::jsonb, '2026-01-02T00:00:00Z'),
  ($3, NULL, '{}'::jsonb, '2026-01-03T00:00:00Z');
`, firstID, secondID, uuid.NewString(), eventID); err != nil {
		t.Fatalf("insert legacy duplicates: %v", err)
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0115_history_event_dedup.up.sql")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0115 up: %v", err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("reapply 0115 up: %v", err)
	}
	var linkedID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM bot_history_messages WHERE event_id = $1`, eventID).Scan(&linkedID); err != nil {
		t.Fatalf("read retained event link: %v", err)
	}
	if linkedID != firstID {
		t.Fatalf("retained event link = %s, want earliest message %s", linkedID, firstID)
	}
	var eventCleared bool
	var rollbackEventID, rollbackMessageID, preservedMetadata string
	if err := tx.QueryRow(ctx, `
SELECT
  event_id IS NULL,
  metadata->'_migration_0115_history_event_dedup'->>'event_id',
  metadata->'_migration_0115_history_event_dedup'->>'message_id',
  metadata->>'keep'
FROM bot_history_messages
WHERE id = $1
`, secondID).Scan(&eventCleared, &rollbackEventID, &rollbackMessageID, &preservedMetadata); err != nil {
		t.Fatalf("read duplicate rollback marker: %v", err)
	}
	if !eventCleared || rollbackEventID != eventID || rollbackMessageID != secondID || preservedMetadata != "value" {
		t.Fatalf(
			"deduplicated row = cleared:%t event:%q message:%q metadata:%q",
			eventCleared,
			rollbackEventID,
			rollbackMessageID,
			preservedMetadata,
		)
	}
	if _, err := tx.Exec(ctx, "SAVEPOINT duplicate_event_link"); err != nil {
		t.Fatalf("create duplicate check savepoint: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, event_id, created_at) VALUES ($1, $2, now())`, uuid.NewString(), eventID); err == nil {
		t.Fatal("duplicate event link succeeded after 0115 up")
	}
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT duplicate_event_link"); err != nil {
		t.Fatalf("restore after duplicate check: %v", err)
	}

	down := readEmbeddedMigration(t, "postgres/migrations/0115_history_event_dedup.down.sql")
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0115 down: %v", err)
	}
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("reapply 0115 down: %v", err)
	}
	var restoredLinks int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM bot_history_messages WHERE event_id = $1`, eventID).Scan(&restoredLinks); err != nil {
		t.Fatalf("count restored event links: %v", err)
	}
	if restoredLinks != 2 {
		t.Fatalf("restored event links = %d, want 2", restoredLinks)
	}
	var rollbackMarkerPresent bool
	if err := tx.QueryRow(ctx, `
SELECT metadata ? '_migration_0115_history_event_dedup'
FROM bot_history_messages
WHERE id = $1
`, secondID).Scan(&rollbackMarkerPresent); err != nil {
		t.Fatalf("read restored metadata: %v", err)
	}
	if rollbackMarkerPresent {
		t.Fatal("0115 down left the migration rollback marker behind")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, event_id, created_at) VALUES ($1, $2, now())`, uuid.NewString(), eventID); err != nil {
		t.Fatalf("duplicate event link failed after 0115 down: %v", err)
	}
}
