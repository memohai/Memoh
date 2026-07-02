package store

import (
	"context"
	"strconv"
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
CREATE TABLE bot_history_message_compacts (
  id TEXT PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'pending'
);
`)

	botID := "00000000-0000-0000-0000-000000003001"
	sessionID := "00000000-0000-0000-0000-000000003002"
	compactedID := "00000000-0000-0000-0000-0000000030ff"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type) VALUES (?, ?)`, sessionID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_message_compacts (id, status) VALUES (?, 'ok')`, compactedID); err != nil {
		t.Fatalf("insert compact log: %v", err)
	}
	for _, item := range []struct {
		id        string
		compactID any
		metadata  string
	}{
		{"00000000-0000-0000-0000-000000003004", "", "{}"},
		{"00000000-0000-0000-0000-000000003003", nil, "{}"},
		{"00000000-0000-0000-0000-000000003005", compactedID, "{}"},
		{"00000000-0000-0000-0000-000000003006", nil, `{"trigger_mode":"passive_sync"}`},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, compact_id, metadata, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			"user",
			`"hello"`,
			item.compactID,
			item.metadata,
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
		t.Fatalf("rows = %d, want active null and empty compact_id rows", len(rows))
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

func TestSQLiteListUncompactedMessagesReclaimsRowsWithoutCompletedLog(t *testing.T) {
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
CREATE TABLE bot_history_message_compacts (
  id TEXT PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'pending'
);
`)

	botID := "00000000-0000-0000-0000-000000005001"
	sessionID := "00000000-0000-0000-0000-000000005002"
	okLog := "00000000-0000-0000-0000-0000000050aa"
	pendingLog := "00000000-0000-0000-0000-0000000050bb"
	errorLog := "00000000-0000-0000-0000-0000000050cc"
	missingLog := "00000000-0000-0000-0000-0000000050dd"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type) VALUES (?, ?)`, sessionID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	for _, log := range []struct{ id, status string }{
		{okLog, "ok"}, {pendingLog, "pending"}, {errorLog, "error"},
	} {
		if _, err := conn.ExecContext(ctx, `INSERT INTO bot_history_message_compacts (id, status) VALUES (?, ?)`, log.id, log.status); err != nil {
			t.Fatalf("insert compact log %s: %v", log.id, err)
		}
	}
	for i, compactID := range []string{okLog, pendingLog, errorLog, missingLog} {
		id := "00000000-0000-0000-0000-00000000510" + strconv.Itoa(i)
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, compact_id, created_at)
VALUES (?, ?, ?, 'user', '"hello"', ?, ?)`,
			id, botID, sessionID, compactID, "2026-06-13 19:53:5"+strconv.Itoa(i),
		)
		if err != nil {
			t.Fatalf("insert message %s: %v", id, err)
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
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want pending/error/missing-log rows reclaimed and ok-log row excluded", len(rows))
	}
	for _, row := range rows {
		if row.CompactID == mustUUID(t, okLog) {
			t.Fatal("row compacted by a completed log must stay excluded")
		}
	}
}

func TestSQLiteListActiveMessagesSinceExcludesPassiveSync(t *testing.T) {
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
  compact_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000004001"
	sessionID := "00000000-0000-0000-0000-000000004002"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, channel_type) VALUES (?, ?)`, sessionID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	for _, item := range []struct {
		id       string
		metadata string
	}{
		{"00000000-0000-0000-0000-000000004003", "{}"},
		{"00000000-0000-0000-0000-000000004004", `{"trigger_mode":"passive_sync"}`},
	} {
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, metadata, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.id,
			botID,
			sessionID,
			"user",
			`"hello"`,
			item.metadata,
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
	since := pgtype.Timestamptz{
		Time:  time.Date(2026, 6, 13, 19, 53, 49, 0, time.UTC),
		Valid: true,
	}

	activeByBot, err := q.ListActiveMessagesSince(ctx, pgsqlc.ListActiveMessagesSinceParams{
		BotID:     mustUUID(t, botID),
		CreatedAt: since,
	})
	if err != nil {
		t.Fatalf("list active messages: %v", err)
	}
	if len(activeByBot) != 1 || activeByBot[0].ID != mustUUID(t, "00000000-0000-0000-0000-000000004003") {
		t.Fatalf("active by bot rows = %#v, want only non-passive row", activeByBot)
	}

	activeBySession, err := q.ListActiveMessagesSinceBySession(ctx, pgsqlc.ListActiveMessagesSinceBySessionParams{
		SessionID: mustUUID(t, sessionID),
		CreatedAt: since,
	})
	if err != nil {
		t.Fatalf("list active messages by session: %v", err)
	}
	if len(activeBySession) != 1 || activeBySession[0].ID != mustUUID(t, "00000000-0000-0000-0000-000000004003") {
		t.Fatalf("active by session rows = %#v, want only non-passive row", activeBySession)
	}
}
