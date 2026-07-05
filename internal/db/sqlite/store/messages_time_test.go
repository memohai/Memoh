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
CREATE VIEW bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.*
FROM bot_history_messages m
WHERE m.turn_visible = 1;
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
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
			item.id,
			botID,
			sessionID,
			item.role,
			item.content,
			"00000000-0000-0000-0000-000000002005",
			1,
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

func TestSQLiteListMessagesLatestAndBeforeMessageUseTurnOrder(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, sqliteMessageListTestSchema)

	botID := "00000000-0000-0000-0000-000000003001"
	sessionID := "00000000-0000-0000-0000-000000003002"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type) VALUES (?, ?)`, sessionID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	messageIDs := []string{
		"00000000-0000-0000-0000-000000003101",
		"00000000-0000-0000-0000-000000003102",
		"00000000-0000-0000-0000-000000003103",
		"00000000-0000-0000-0000-000000003104",
		"00000000-0000-0000-0000-000000003105",
	}
	turnIDs := []string{
		"00000000-0000-0000-0000-000000003201",
		"00000000-0000-0000-0000-000000003202",
		"00000000-0000-0000-0000-000000003203",
		"00000000-0000-0000-0000-000000003204",
		"00000000-0000-0000-0000-000000003205",
	}
	for i := range messageIDs {
		createdAt := time.Date(2026, 6, 14, 10, i, 0, 0, time.UTC).Format("2006-01-02 15:04:05")
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, turn_id, turn_position, turn_message_seq, turn_visible, created_at)
VALUES (?, ?, ?, 'user', '{}', ?, ?, 1, 1, ?)`, messageIDs[i], botID, sessionID, turnIDs[i], i+1, createdAt); err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
		if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, bot_id, session_id, position, request_message_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, turnIDs[i], botID, sessionID, i+1, messageIDs[i], createdAt, createdAt); err != nil {
			t.Fatalf("insert turn %d: %v", i, err)
		}
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)

	latest, err := q.ListMessagesLatestBySession(ctx, pgsqlc.ListMessagesLatestBySessionParams{
		SessionID: mustUUID(t, sessionID),
		MaxCount:  3,
	})
	if err != nil {
		t.Fatalf("list latest: %v", err)
	}
	gotLatest := sqliteMessageRowIDs(latest)
	wantLatest := []string{messageIDs[4], messageIDs[3], messageIDs[2]}
	if !equalStrings(gotLatest, wantLatest) {
		t.Fatalf("latest ids = %v, want %v", gotLatest, wantLatest)
	}

	before, err := q.ListMessagesBeforeMessageBySession(ctx, pgsqlc.ListMessagesBeforeMessageBySessionParams{
		SessionID:       mustUUID(t, sessionID),
		MaxCount:        2,
		BeforeMessageID: mustUUID(t, messageIDs[2]),
	})
	if err != nil {
		t.Fatalf("list before message: %v", err)
	}
	gotBefore := sqliteBeforeMessageRowIDs(before)
	wantBefore := []string{messageIDs[1], messageIDs[0]}
	if !equalStrings(gotBefore, wantBefore) {
		t.Fatalf("before ids = %v, want %v", gotBefore, wantBefore)
	}
}

func sqliteMessageRowIDs(rows []pgsqlc.ListMessagesLatestBySessionRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.String())
	}
	return ids
}

func sqliteBeforeMessageRowIDs(rows []pgsqlc.ListMessagesBeforeMessageBySessionRow) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID.String())
	}
	return ids
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

const sqliteMessageListTestSchema = `
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
CREATE VIEW bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.*
FROM bot_history_messages m
WHERE m.turn_visible = 1;
`
