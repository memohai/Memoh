package store

import (
	"context"
	"database/sql"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
)

func TestSQLiteListSessionOwnedTurnsForCleanupReturnsAbandonedOwnedTurns(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	createTurnCleanupSchema(t, conn)

	originSessionID := "00000000-0000-0000-0000-000000000010"
	forkSessionID := "00000000-0000-0000-0000-000000000020"
	originTurnID := "00000000-0000-0000-0000-000000000101"
	forkTurnID := "00000000-0000-0000-0000-000000000201"
	abandonedForkTurnID := "00000000-0000-0000-0000-000000000202"
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO bot_history_turns (id, bot_id, owner_session_id, parent_turn_id, created_at, updated_at)
		 VALUES (?, '00000000-0000-0000-0000-000000000001', ?, NULL, '2026-06-19 00:00:00', '2026-06-19 00:00:00')`,
		originTurnID, originSessionID,
	); err != nil {
		t.Fatalf("insert origin turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO bot_history_turns (id, bot_id, owner_session_id, parent_turn_id, created_at, updated_at)
		 VALUES (?, '00000000-0000-0000-0000-000000000001', ?, ?, '2026-06-19 00:01:00', '2026-06-19 00:01:00')`,
		forkTurnID, forkSessionID, originTurnID,
	); err != nil {
		t.Fatalf("insert fork turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO bot_history_turns (id, bot_id, owner_session_id, parent_turn_id, created_at, updated_at)
		 VALUES (?, '00000000-0000-0000-0000-000000000001', ?, ?, '2026-06-19 00:02:00', '2026-06-19 00:02:00')`,
		abandonedForkTurnID, forkSessionID, originTurnID,
	); err != nil {
		t.Fatalf("insert abandoned fork turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx,
		`INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id) VALUES (?, ?, ?)`,
		forkSessionID, forkTurnID, "00000000-0000-0000-0000-000000000001",
	); err != nil {
		t.Fatalf("insert fork head: %v", err)
	}

	st, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	queries := NewQueries(st)

	rows, err := queries.ListSessionOwnedTurnsForCleanup(ctx, mustUUID(t, forkSessionID))
	if err != nil {
		t.Fatalf("ListSessionOwnedTurnsForCleanup: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("cleanup candidate count = %d, want 2: %#v", len(rows), rows)
	}
	got := map[string]bool{}
	for _, row := range rows {
		got[row.ID.String()] = true
	}
	if !got[forkTurnID] || !got[abandonedForkTurnID] {
		t.Fatalf("cleanup candidates = %#v, want fork-owned turns %s and %s", got, forkTurnID, abandonedForkTurnID)
	}
	if got[originTurnID] {
		t.Fatalf("cleanup candidates included borrowed ancestor %s", originTurnID)
	}
}

func createTurnCleanupSchema(t *testing.T, conn *sql.DB) {
	t.Helper()
	execAll(t, conn, `
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  owner_session_id TEXT,
  parent_turn_id TEXT,
  request_message_id TEXT,
  final_assistant_message_id TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  origin_kind TEXT,
  origin_turn_id TEXT,
  request_group_id TEXT
);
CREATE TABLE bot_session_turn_heads (
  session_id TEXT NOT NULL,
  head_turn_id TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (session_id, head_turn_id)
);
`)
}
