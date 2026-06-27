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
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
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
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
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

func TestSQLiteListUncompactedMessagesNormalizesEmptyCompactIDAndOrdersTies(t *testing.T) {
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
CREATE TABLE bot_channel_routes (
  id TEXT PRIMARY KEY,
  conversation_type TEXT,
  metadata TEXT NOT NULL DEFAULT '{}',
  default_reply_target TEXT
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  route_id TEXT,
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
  event_id TEXT,
  display_text TEXT,
  compact_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000003001"
	sessionID := "00000000-0000-0000-0000-000000003002"
	compactedID := "00000000-0000-0000-0000-0000000030ff"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type) VALUES (?, ?)`, sessionID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	for _, item := range []struct {
		id        string
		compactID any
	}{
		{"00000000-0000-0000-0000-000000003004", ""},
		{"00000000-0000-0000-0000-000000003003", nil},
		{"00000000-0000-0000-0000-000000003005", compactedID},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, compact_id, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			"user",
			`"hello"`,
			item.compactID,
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

	rows, err := q.ListUncompactedMessagesBySession(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("list uncompacted messages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want null and empty compact_id rows", len(rows))
	}
	want := []pgtype.UUID{
		mustUUID(t, "00000000-0000-0000-0000-000000003003"),
		mustUUID(t, "00000000-0000-0000-0000-000000003004"),
	}
	got := []pgtype.UUID{rows[0].ID, rows[1].ID}
	if got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ordered ids = %#v, want %#v", got, want)
	}
	for _, row := range rows {
		if row.CompactID.Valid {
			t.Fatalf("compact id should be normalized to null for uncompacted row: %#v", row.CompactID)
		}
	}
}
