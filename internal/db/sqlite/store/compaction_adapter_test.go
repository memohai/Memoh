package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteMarkMessagesCompactedConvertsPostgresParams(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  compact_id TEXT
);
`)

	compactID := "00000000-0000-0000-0000-000000005001"
	messageIDs := []string{
		"00000000-0000-0000-0000-000000005002",
		"00000000-0000-0000-0000-000000005003",
	}
	for _, id := range messageIDs {
		if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_messages (id) VALUES (?)`, id); err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	if err := q.MarkMessagesCompacted(ctx, pgsqlc.MarkMessagesCompactedParams{
		CompactID: mustUUID(t, compactID),
		Column2: []pgtype.UUID{
			mustUUID(t, messageIDs[0]),
			mustUUID(t, messageIDs[1]),
		},
	}); err != nil {
		t.Fatalf("mark compacted: %v", err)
	}

	for _, id := range messageIDs {
		var got string
		if err := conn.QueryRowContext(ctx, `SELECT COALESCE(compact_id, '') FROM bot_history_messages WHERE id = ?`, id).Scan(&got); err != nil {
			t.Fatalf("select message %s: %v", id, err)
		}
		if got != compactID {
			t.Fatalf("message %s compact_id = %q, want %q", id, got, compactID)
		}
	}
}

func TestSQLiteListCompactionLogsByBotConvertsPaginationParams(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bot_history_message_compacts (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  summary TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  usage TEXT,
  model_id TEXT,
  started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TEXT
);
`)

	botID := "00000000-0000-0000-0000-000000006001"
	logID := "00000000-0000-0000-0000-000000006002"
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, status, summary, message_count, started_at)
VALUES (?, ?, 'ok', 'summary', 2, '2026-06-27 01:00:00')`, logID, botID); err != nil {
		t.Fatalf("insert compaction log: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	total, err := q.CountCompactionLogsByBot(ctx, mustUUID(t, botID))
	if err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if total != 1 {
		t.Fatalf("count = %d, want 1", total)
	}

	rows, err := q.ListCompactionLogsByBot(ctx, pgsqlc.ListCompactionLogsByBotParams{
		BotID:  mustUUID(t, botID),
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != mustUUID(t, logID) {
		t.Fatalf("rows = %#v, want log %s", rows, logID)
	}
}
