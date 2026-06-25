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
  default_head_turn_id TEXT,
  deleted_at TEXT,
  channel_type TEXT
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  parent_turn_id TEXT
);
CREATE TABLE bot_session_turn_heads (
  session_id TEXT NOT NULL,
  head_turn_id TEXT NOT NULL,
  PRIMARY KEY (session_id, head_turn_id)
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  turn_id TEXT,
  turn_message_seq INTEGER,
  sender_channel_identity_id TEXT,
  sender_account_user_id TEXT,
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  event_id TEXT,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000002001"
	sessionID := "00000000-0000-0000-0000-000000002002"
	turnID := "00000000-0000-0000-0000-000000002005"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id) VALUES (?)`, turnID); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, default_head_turn_id, channel_type) VALUES (?, ?, ?)`, sessionID, turnID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_turn_heads (session_id, head_turn_id) VALUES (?, ?)`, sessionID, turnID); err != nil {
		t.Fatalf("insert session turn head: %v", err)
	}
	for index, item := range []struct {
		id      string
		role    string
		content string
	}{
		{"00000000-0000-0000-0000-000000002003", "user", `{"role":"user","content":"hello"}`},
		{"00000000-0000-0000-0000-000000002004", "assistant", `{"role":"assistant","content":"hi"}`},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			turnID,
			index+1,
			item.role,
			item.content,
			"2026-06-13 19:53:50",
		)
		if err != nil {
			t.Fatalf("insert message %s: %v", item.id, err)
		}
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
