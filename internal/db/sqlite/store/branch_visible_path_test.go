package store

import (
	"context"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	pgsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestSQLiteSessionBranchVisiblePath(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  channel_type TEXT,
  active_branch_id TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE bot_session_branches (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  parent_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  fork_from_message_id TEXT,
  fork_from_seq INTEGER,
  title TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL DEFAULT '',
  avatar_url TEXT NOT NULL DEFAULT ''
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
  sender_channel_identity_id TEXT,
  sender_account_user_id TEXT,
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  usage TEXT,
  model_id TEXT,
  compact_id TEXT,
  event_id TEXT,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000002001"
	sessionID := "00000000-0000-0000-0000-000000002002"
	rootBranchID := "00000000-0000-0000-0000-000000002010"
	forkBranchID := "00000000-0000-0000-0000-000000002011"
	secondForkBranchID := "00000000-0000-0000-0000-000000002012"
	user1ID := "00000000-0000-0000-0000-000000002101"
	assistant1ID := "00000000-0000-0000-0000-000000002102"
	user2ID := "00000000-0000-0000-0000-000000002103"
	assistant2ID := "00000000-0000-0000-0000-000000002104"
	forkUserID := "00000000-0000-0000-0000-000000002201"
	forkAssistantID := "00000000-0000-0000-0000-000000002202"
	secondForkUserID := "00000000-0000-0000-0000-000000002203"
	secondForkAssistantID := "00000000-0000-0000-0000-000000002204"

	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, active_branch_id) VALUES (?, ?)`, sessionID, forkBranchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES (?, ?, ?, ?)`, rootBranchID, sessionID, "2026-01-01 00:00:00", "2026-01-01 00:00:00"); err != nil {
		t.Fatalf("insert root branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, forkBranchID, sessionID, rootBranchID, assistant1ID, 0, "2026-01-01 00:10:00", "2026-01-01 00:10:00"); err != nil {
		t.Fatalf("insert fork branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, secondForkBranchID, sessionID, rootBranchID, assistant2ID, 2, "2026-01-01 00:20:00", "2026-01-01 00:20:00"); err != nil {
		t.Fatalf("insert second fork branch: %v", err)
	}
	insertMessage := func(id, branchID, role string, seq int, createdAt string) {
		t.Helper()
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, branch_id, branch_seq, role, content, metadata, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, '{}', ?)`,
			id, botID, sessionID, branchID, seq, role, `{"content":"`+id+`"}`, createdAt)
		if err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	insertMessage(user1ID, rootBranchID, "user", 1, "2026-01-01 00:01:00")
	insertMessage(assistant1ID, rootBranchID, "assistant", 2, "2026-01-01 00:02:00")
	insertMessage(user2ID, rootBranchID, "user", 3, "2026-01-01 00:03:00")
	insertMessage(assistant2ID, rootBranchID, "assistant", 4, "2026-01-01 00:04:00")
	insertMessage(forkUserID, forkBranchID, "user", 1, "2026-01-01 00:11:00")
	insertMessage(forkAssistantID, forkBranchID, "assistant", 2, "2026-01-01 00:12:00")
	insertMessage(secondForkUserID, secondForkBranchID, "user", 1, "2026-01-01 00:21:00")
	insertMessage(secondForkAssistantID, secondForkBranchID, "assistant", 2, "2026-01-01 00:22:00")

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	sessionUUID := mustUUID(t, sessionID)

	rows, err := q.ListMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list active fork path: %v", err)
	}
	assertMessageIDs(t, rows, []string{forkUserID, forkAssistantID})

	latest, err := q.ListMessagesLatestBySession(ctx, pgsqlc.ListMessagesLatestBySessionParams{
		SessionID: sessionUUID,
		MaxCount:  10,
	})
	if err != nil {
		t.Fatalf("list latest active fork path: %v", err)
	}
	assertLatestMessageIDs(t, latest, []string{forkAssistantID, forkUserID})

	uncompacted, err := q.ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list uncompacted active fork path: %v", err)
	}
	assertUncompactedMessageIDs(t, uncompacted, []string{forkUserID, forkAssistantID})

	if err := q.SetActiveSessionBranch(ctx, pgsqlc.SetActiveSessionBranchParams{
		SessionID: sessionUUID,
		BranchID:  mustUUID(t, secondForkBranchID),
	}); err != nil {
		t.Fatalf("switch second fork branch: %v", err)
	}
	secondForkRows, err := q.ListMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list second fork path: %v", err)
	}
	assertMessageIDs(t, secondForkRows, []string{user1ID, assistant1ID, secondForkUserID, secondForkAssistantID})

	if err := q.SetActiveSessionBranch(ctx, pgsqlc.SetActiveSessionBranchParams{
		SessionID: sessionUUID,
		BranchID:  mustUUID(t, rootBranchID),
	}); err != nil {
		t.Fatalf("switch root branch: %v", err)
	}
	rootRows, err := q.ListMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list root path: %v", err)
	}
	assertMessageIDs(t, rootRows, []string{user1ID, assistant1ID, user2ID, assistant2ID})

	turnRows, err := q.ListSessionBranchTurnMessages(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list branch turn messages: %v", err)
	}
	assertTurnAssistantIDs(t, turnRows, []string{assistant1ID, assistant2ID, forkAssistantID, secondForkAssistantID})

	firstForkPoint, err := q.GetSessionBranchForkPoint(ctx, pgsqlc.GetSessionBranchForkPointParams{
		SessionID: sessionUUID,
		BranchID:  mustUUID(t, rootBranchID),
		BranchSeq: 2,
	})
	if err != nil {
		t.Fatalf("get first fork point: %v", err)
	}
	if firstForkPoint != 0 {
		t.Fatalf("first fork point = %d, want 0", firstForkPoint)
	}
	secondForkPoint, err := q.GetSessionBranchForkPoint(ctx, pgsqlc.GetSessionBranchForkPointParams{
		SessionID: sessionUUID,
		BranchID:  mustUUID(t, rootBranchID),
		BranchSeq: 4,
	})
	if err != nil {
		t.Fatalf("get second fork point: %v", err)
	}
	if secondForkPoint != 2 {
		t.Fatalf("second fork point = %d, want 2", secondForkPoint)
	}
}

func assertMessageIDs(t *testing.T, rows []pgsqlc.ListMessagesBySessionRow, want []string) {
	t.Helper()
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.ID.String())
	}
	assertStringSlice(t, got, want)
}

func assertLatestMessageIDs(t *testing.T, rows []pgsqlc.ListMessagesLatestBySessionRow, want []string) {
	t.Helper()
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.ID.String())
	}
	assertStringSlice(t, got, want)
}

func assertUncompactedMessageIDs(t *testing.T, rows []pgsqlc.ListUncompactedMessagesBySessionRow, want []string) {
	t.Helper()
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.ID.String())
	}
	assertStringSlice(t, got, want)
}

func assertTurnAssistantIDs(t *testing.T, rows []pgsqlc.ListSessionBranchTurnMessagesRow, want []string) {
	t.Helper()
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.AssistantID.String())
	}
	assertStringSlice(t, got, want)
}

func assertStringSlice(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("ids = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %#v, want %#v", got, want)
		}
	}
}
