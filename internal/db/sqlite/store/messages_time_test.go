package store

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteListMessagesBeforeBySessionUsesSQLiteTimestampFormat(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY,
  display_name TEXT,
  avatar_url TEXT
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  channel_type TEXT
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  sender_channel_identity_id TEXT,
  sender_account_user_id TEXT,
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  event_id TEXT,
  display_text TEXT,
  turn_id TEXT,
  turn_message_seq INTEGER,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  request_message_id TEXT,
  assistant_message_id TEXT,
  superseded_by_turn_id TEXT,
  superseded_at TEXT,
  superseded_reason TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (session_id, position)
);
CREATE VIEW bot_visible_history_messages AS
SELECT
  t.id AS turn_id,
  t.position AS turn_position,
  m.turn_message_seq,
  m.*
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE t.superseded_at IS NULL;
`)

	botID := "00000000-0000-0000-0000-000000002001"
	sessionID := "00000000-0000-0000-0000-000000002002"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type) VALUES (?, ?)`, sessionID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	for _, item := range []struct {
		id      string
		role    string
		content string
	}{
		{"00000000-0000-0000-0000-000000002003", "user", `{"role":"user","content":"hello"}`},
		{"00000000-0000-0000-0000-000000002004", "assistant", `{"role":"assistant","content":"hi"}`},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_message_seq, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			item.role,
			item.content,
			"00000000-0000-0000-0000-000000002005",
			map[string]int{"user": 1, "assistant": 2}[item.role],
			"2026-06-13 19:53:50",
		)
		if err != nil {
			t.Fatalf("insert message %s: %v", item.id, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, bot_id, session_id, position, request_message_id, assistant_message_id, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?, ?, ?)`,
		"00000000-0000-0000-0000-000000002005",
		botID,
		sessionID,
		"00000000-0000-0000-0000-000000002003",
		"00000000-0000-0000-0000-000000002004",
		"2026-06-13 19:53:50",
		"2026-06-13 19:53:50",
	); err != nil {
		t.Fatalf("insert turn: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	rows, err := q.ListMessagesBeforeBySession(ctx, pgsqlc.ListMessagesBeforeBySessionParams{
		SessionID: mustUUID(t, sessionID),
		CreatedAt: pgtype.Timestamptz{
			Time:  time.Date(2026, 6, 13, 19, 53, 50, 0, time.UTC),
			Valid: true,
		},
		MaxCount: 30,
	})
	if err != nil {
		t.Fatalf("list messages before: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("same-second messages must not be returned by before cursor, got %d", len(rows))
	}
}
