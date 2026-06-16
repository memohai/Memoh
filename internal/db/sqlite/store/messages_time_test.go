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
  channel_type TEXT,
  active_branch_id TEXT
);
CREATE TABLE bot_session_branches (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  parent_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  fork_from_message_id TEXT,
  fork_from_seq INTEGER,
  fork_from_turn_id TEXT,
  fork_from_turn_seq INTEGER,
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL REFERENCES bot_session_branches(id) ON DELETE CASCADE,
  turn_seq INTEGER NOT NULL,
  request_message_id TEXT,
  final_assistant_message_id TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TEXT
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
  turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
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
	branchID := "00000000-0000-0000-0000-000000002010"
	turnID := "00000000-0000-0000-0000-000000002011"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type, active_branch_id) VALUES (?, ?, ?)`, sessionID, "local", branchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES (?, ?, ?, ?)`, branchID, sessionID, "2026-06-13 19:53:50", "2026-06-13 19:53:50"); err != nil {
		t.Fatalf("insert branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, status, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, 'completed', ?, ?, ?)`, turnID, sessionID, branchID, 1, "2026-06-13 19:53:50", "2026-06-13 19:53:50", "2026-06-13 19:53:50"); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	for _, item := range []struct {
		id      string
		role    string
		content string
		seq     int
	}{
		{"00000000-0000-0000-0000-000000002003", "user", `{"role":"user","content":"hello"}`, 1},
		{"00000000-0000-0000-0000-000000002004", "assistant", `{"role":"assistant","content":"hi"}`, 2},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			branchID,
			item.seq,
			turnID,
			item.seq,
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

func TestSQLiteListMessagesBeforeBySessionUsesCursorIDForSameSecond(t *testing.T) {
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
  channel_type TEXT,
  active_branch_id TEXT
);
CREATE TABLE bot_session_branches (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  parent_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  fork_from_message_id TEXT,
  fork_from_seq INTEGER,
  fork_from_turn_id TEXT,
  fork_from_turn_seq INTEGER,
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL REFERENCES bot_session_branches(id) ON DELETE CASCADE,
  turn_seq INTEGER NOT NULL,
  request_message_id TEXT,
  final_assistant_message_id TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  completed_at TEXT
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
  turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
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

	botID := "00000000-0000-0000-0000-000000003001"
	sessionID := "00000000-0000-0000-0000-000000003002"
	branchID := "00000000-0000-0000-0000-000000003010"
	turnID := "00000000-0000-0000-0000-000000003011"
	userID := "00000000-0000-0000-0000-000000003003"
	assistantID := "00000000-0000-0000-0000-000000003004"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type, active_branch_id) VALUES (?, ?, ?)`, sessionID, "local", branchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES (?, ?, ?, ?)`, branchID, sessionID, "2026-06-13 19:53:50", "2026-06-13 19:53:50"); err != nil {
		t.Fatalf("insert branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, status, created_at, updated_at, completed_at) VALUES (?, ?, ?, ?, 'completed', ?, ?, ?)`, turnID, sessionID, branchID, 1, "2026-06-13 19:53:50", "2026-06-13 19:53:50", "2026-06-13 19:53:50"); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	for _, item := range []struct {
		id   string
		role string
		seq  int
	}{
		{userID, "user", 1},
		{assistantID, "assistant", 2},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			branchID,
			item.seq,
			turnID,
			item.seq,
			item.role,
			`{"role":"`+item.role+`","content":"same second"}`,
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
		BeforeID:  mustUUID(t, assistantID),
		CreatedAt: pgtype.Timestamptz{
			Time:  time.Date(2026, 6, 13, 19, 53, 50, 0, time.UTC),
			Valid: true,
		},
		MaxCount: 30,
	})
	if err != nil {
		t.Fatalf("list messages before with cursor id: %v", err)
	}
	if len(rows) != 1 || rows[0].ID.String() != userID {
		t.Fatalf("same-second cursor rows = %#v, want only %s", rows, userID)
	}
}
