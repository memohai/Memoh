package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteCreateMessageWithHistoryTurnRollsBackOnTurnFailure(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteCreateMessageWithHistoryTurnAtomicSchema)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	botID := "00000000-0000-0000-0000-000000004001"
	sessionID := "00000000-0000-0000-0000-000000004002"
	turnID := "00000000-0000-0000-0000-000000004003"
	messageID := "00000000-0000-0000-0000-000000004004"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id, next_turn_position) VALUES (?, ?, 7)`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, bot_id, session_id, position) VALUES (?, ?, ?, 1)`, turnID, botID, sessionID); err != nil {
		t.Fatalf("insert existing turn: %v", err)
	}

	_, err = q.CreateMessageWithHistoryTurn(ctx, pgsqlc.CreateMessageWithHistoryTurnParams{
		MessageID:      pgUUIDFromString(t, messageID),
		BotID:          pgUUIDFromString(t, botID),
		SessionID:      pgUUIDFromString(t, sessionID),
		Role:           "user",
		Content:        []byte(`{"type":"text","content":"rollback"}`),
		Metadata:       []byte(`{}`),
		Usage:          []byte(`{}`),
		SessionMode:    "chat",
		RuntimeType:    "model",
		TurnID:         pgUUIDFromString(t, turnID),
		TurnMessageSeq: pgtype.Int8{Int64: 1, Valid: true},
	})
	if err == nil {
		t.Fatal("create message with duplicate turn id succeeded, want error")
	}

	var messageCount int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM bot_history_messages WHERE id = ?`, messageID).Scan(&messageCount); err != nil {
		t.Fatalf("count message: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("message count after rollback = %d, want 0", messageCount)
	}
	var nextPosition int64
	if err := conn.QueryRowContext(ctx, `SELECT next_turn_position FROM bot_sessions WHERE id = ?`, sessionID).Scan(&nextPosition); err != nil {
		t.Fatalf("select next position: %v", err)
	}
	if nextPosition != 7 {
		t.Fatalf("next_turn_position after rollback = %d, want 7", nextPosition)
	}
}

func pgUUIDFromString(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var out pgtype.UUID
	if err := out.Scan(value); err != nil {
		t.Fatalf("parse uuid %q: %v", value, err)
	}
	return out
}

const sqliteCreateMessageWithHistoryTurnAtomicSchema = `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  next_turn_position INTEGER NOT NULL DEFAULT 1
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
  model_id TEXT,
  event_id TEXT,
  display_text TEXT,
  turn_id TEXT,
  turn_position INTEGER,
  turn_message_seq INTEGER,
  turn_visible INTEGER NOT NULL DEFAULT 0,
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
`
