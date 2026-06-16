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
	rootTurn1ID := "00000000-0000-0000-0000-000000002301"
	rootTurn2ID := "00000000-0000-0000-0000-000000002302"
	forkTurnID := "00000000-0000-0000-0000-000000002303"
	secondForkTurnID := "00000000-0000-0000-0000-000000002304"

	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, active_branch_id) VALUES (?, ?)`, sessionID, forkBranchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES (?, ?, ?, ?)`, rootBranchID, sessionID, "2026-01-01 00:00:00", "2026-01-01 00:00:00"); err != nil {
		t.Fatalf("insert root branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, fork_from_turn_id, fork_from_turn_seq, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, forkBranchID, sessionID, rootBranchID, assistant1ID, 2, rootTurn1ID, 1, "2026-01-01 00:10:00", "2026-01-01 00:10:00"); err != nil {
		t.Fatalf("insert fork branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, fork_from_turn_id, fork_from_turn_seq, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, secondForkBranchID, sessionID, rootBranchID, assistant2ID, 4, rootTurn2ID, 2, "2026-01-01 00:20:00", "2026-01-01 00:20:00"); err != nil {
		t.Fatalf("insert second fork branch: %v", err)
	}
	insertTurn := func(id, branchID string, seq int, userID, assistantID, createdAt, completedAt string) {
		t.Helper()
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (
  id, session_id, branch_id, turn_seq, request_message_id, final_assistant_message_id, status, created_at, updated_at, completed_at
) VALUES (?, ?, ?, ?, ?, ?, 'completed', ?, ?, ?)`,
			id, sessionID, branchID, seq, userID, assistantID, createdAt, completedAt, completedAt)
		if err != nil {
			t.Fatalf("insert turn %s: %v", id, err)
		}
	}
	insertMessage := func(id, branchID, turnID, role string, seq, turnMessageSeq int, createdAt string) {
		t.Helper()
		_, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, metadata, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?)`,
			id, botID, sessionID, branchID, seq, turnID, turnMessageSeq, role, `{"content":"`+id+`"}`, createdAt)
		if err != nil {
			t.Fatalf("insert message %s: %v", id, err)
		}
	}
	insertTurn(rootTurn1ID, rootBranchID, 1, user1ID, assistant1ID, "2026-01-01 00:01:00", "2026-01-01 00:02:00")
	insertTurn(rootTurn2ID, rootBranchID, 2, user2ID, assistant2ID, "2026-01-01 00:03:00", "2026-01-01 00:04:00")
	insertTurn(forkTurnID, forkBranchID, 1, forkUserID, forkAssistantID, "2026-01-01 00:11:00", "2026-01-01 00:12:00")
	insertTurn(secondForkTurnID, secondForkBranchID, 1, secondForkUserID, secondForkAssistantID, "2026-01-01 00:21:00", "2026-01-01 00:22:00")
	insertMessage(user1ID, rootBranchID, rootTurn1ID, "user", 1, 1, "2026-01-01 00:01:00")
	insertMessage(assistant1ID, rootBranchID, rootTurn1ID, "assistant", 2, 2, "2026-01-01 00:02:00")
	insertMessage(user2ID, rootBranchID, rootTurn2ID, "user", 3, 1, "2026-01-01 00:03:00")
	insertMessage(assistant2ID, rootBranchID, rootTurn2ID, "assistant", 4, 2, "2026-01-01 00:04:00")
	insertMessage(forkUserID, forkBranchID, forkTurnID, "user", 1, 1, "2026-01-01 00:11:00")
	insertMessage(forkAssistantID, forkBranchID, forkTurnID, "assistant", 2, 2, "2026-01-01 00:12:00")
	insertMessage(secondForkUserID, secondForkBranchID, secondForkTurnID, "user", 1, 1, "2026-01-01 00:21:00")
	insertMessage(secondForkAssistantID, secondForkBranchID, secondForkTurnID, "assistant", 2, 2, "2026-01-01 00:22:00")

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
	assertMessageIDs(t, rows, []string{user1ID, assistant1ID, forkUserID, forkAssistantID})

	latest, err := q.ListMessagesLatestBySession(ctx, pgsqlc.ListMessagesLatestBySessionParams{
		SessionID: sessionUUID,
		MaxCount:  10,
	})
	if err != nil {
		t.Fatalf("list latest active fork path: %v", err)
	}
	assertLatestMessageIDs(t, latest, []string{forkAssistantID, forkUserID, assistant1ID, user1ID})

	uncompacted, err := q.ListUncompactedMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list uncompacted active fork path: %v", err)
	}
	assertUncompactedMessageIDs(t, uncompacted, []string{user1ID, assistant1ID, forkUserID, forkAssistantID})

	if _, err := q.SetActiveSessionBranch(ctx, pgsqlc.SetActiveSessionBranchParams{
		SessionID: sessionUUID,
		BranchID:  mustUUID(t, secondForkBranchID),
	}); err != nil {
		t.Fatalf("switch second fork branch: %v", err)
	}
	secondForkRows, err := q.ListMessagesBySession(ctx, sessionUUID)
	if err != nil {
		t.Fatalf("list second fork path: %v", err)
	}
	assertMessageIDs(t, secondForkRows, []string{user1ID, assistant1ID, user2ID, assistant2ID, secondForkUserID, secondForkAssistantID})

	if _, err := q.SetActiveSessionBranch(ctx, pgsqlc.SetActiveSessionBranchParams{
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

	assertTurnSeqs(t, turnRows, []int64{1, 2, 1, 1})
}

func TestSQLiteSessionBranchVisiblePathFallsBackToForkMessageSeq(t *testing.T) {
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
  model_id TEXT,
  compact_id TEXT,
  event_id TEXT,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000009001"
	sessionID := "00000000-0000-0000-0000-000000009002"
	rootBranchID := "00000000-0000-0000-0000-000000009010"
	forkBranchID := "00000000-0000-0000-0000-000000009011"
	rootTurn1ID := "00000000-0000-0000-0000-000000009101"
	rootTurn2ID := "00000000-0000-0000-0000-000000009102"
	forkTurnID := "00000000-0000-0000-0000-000000009103"
	user1ID := "00000000-0000-0000-0000-000000009201"
	assistant1ID := "00000000-0000-0000-0000-000000009202"
	user2ID := "00000000-0000-0000-0000-000000009203"
	assistant2ID := "00000000-0000-0000-0000-000000009204"
	forkUserID := "00000000-0000-0000-0000-000000009301"
	forkAssistantID := "00000000-0000-0000-0000-000000009302"

	execAll(t, conn, `
INSERT INTO bot_sessions (id, active_branch_id) VALUES ('`+sessionID+`', '`+forkBranchID+`');
INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES ('`+rootBranchID+`', '`+sessionID+`', '2026-01-01 00:00:00', '2026-01-01 00:00:00');
INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, created_at, updated_at)
VALUES ('`+forkBranchID+`', '`+sessionID+`', '`+rootBranchID+`', '`+assistant1ID+`', 2, '2026-01-01 00:10:00', '2026-01-01 00:10:00');
INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, request_message_id, final_assistant_message_id, status, created_at, updated_at, completed_at)
VALUES
  ('`+rootTurn1ID+`', '`+sessionID+`', '`+rootBranchID+`', 1, '`+user1ID+`', '`+assistant1ID+`', 'completed', '2026-01-01 00:01:00', '2026-01-01 00:02:00', '2026-01-01 00:02:00'),
  ('`+rootTurn2ID+`', '`+sessionID+`', '`+rootBranchID+`', 2, '`+user2ID+`', '`+assistant2ID+`', 'completed', '2026-01-01 00:03:00', '2026-01-01 00:04:00', '2026-01-01 00:04:00'),
  ('`+forkTurnID+`', '`+sessionID+`', '`+forkBranchID+`', 1, '`+forkUserID+`', '`+forkAssistantID+`', 'completed', '2026-01-01 00:11:00', '2026-01-01 00:12:00', '2026-01-01 00:12:00');
INSERT INTO bot_history_messages (id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, metadata, created_at)
VALUES
  ('`+user1ID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 1, '`+rootTurn1ID+`', 1, 'user', '{"content":"u1"}', '{}', '2026-01-01 00:01:00'),
  ('`+assistant1ID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 2, '`+rootTurn1ID+`', 2, 'assistant', '{"content":"a1"}', '{}', '2026-01-01 00:02:00'),
  ('`+user2ID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 3, '`+rootTurn2ID+`', 1, 'user', '{"content":"u2"}', '{}', '2026-01-01 00:03:00'),
  ('`+assistant2ID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 4, '`+rootTurn2ID+`', 2, 'assistant', '{"content":"a2"}', '{}', '2026-01-01 00:04:00'),
  ('`+forkUserID+`', '`+botID+`', '`+sessionID+`', '`+forkBranchID+`', 1, '`+forkTurnID+`', 1, 'user', '{"content":"fu"}', '{}', '2026-01-01 00:11:00'),
  ('`+forkAssistantID+`', '`+botID+`', '`+sessionID+`', '`+forkBranchID+`', 2, '`+forkTurnID+`', 2, 'assistant', '{"content":"fa"}', '{}', '2026-01-01 00:12:00');
`)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	rows, err := NewQueries(store).ListMessagesBySession(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("list active path: %v", err)
	}
	assertMessageIDs(t, rows, []string{user1ID, assistant1ID, forkUserID, forkAssistantID})
}

func TestSQLiteSessionBranchVisiblePathClampsForkTurnByMessageSeq(t *testing.T) {
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
  model_id TEXT,
  compact_id TEXT,
  event_id TEXT,
  display_text TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000008001"
	sessionID := "00000000-0000-0000-0000-000000008002"
	rootBranchID := "00000000-0000-0000-0000-000000008010"
	forkBranchID := "00000000-0000-0000-0000-000000008011"
	rootTurnID := "00000000-0000-0000-0000-000000008101"
	forkTurnID := "00000000-0000-0000-0000-000000008102"
	userID := "00000000-0000-0000-0000-000000008201"
	assistantID := "00000000-0000-0000-0000-000000008202"
	lateToolID := "00000000-0000-0000-0000-000000008203"
	forkUserID := "00000000-0000-0000-0000-000000008301"
	forkAssistantID := "00000000-0000-0000-0000-000000008302"

	execAll(t, conn, `
INSERT INTO bot_sessions (id, active_branch_id) VALUES ('`+sessionID+`', '`+forkBranchID+`');
INSERT INTO bot_session_branches (id, session_id, created_at, updated_at)
VALUES ('`+rootBranchID+`', '`+sessionID+`', '2026-01-01 00:00:00', '2026-01-01 00:00:00');
INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, fork_from_turn_id, fork_from_turn_seq, created_at, updated_at)
VALUES ('`+forkBranchID+`', '`+sessionID+`', '`+rootBranchID+`', '`+assistantID+`', 2, '`+rootTurnID+`', 1, '2026-01-01 00:10:00', '2026-01-01 00:10:00');
INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, request_message_id, final_assistant_message_id, status, created_at, updated_at, completed_at)
VALUES
  ('`+rootTurnID+`', '`+sessionID+`', '`+rootBranchID+`', 1, '`+userID+`', '`+assistantID+`', 'completed', '2026-01-01 00:01:00', '2026-01-01 00:02:00', '2026-01-01 00:02:00'),
  ('`+forkTurnID+`', '`+sessionID+`', '`+forkBranchID+`', 1, '`+forkUserID+`', '`+forkAssistantID+`', 'completed', '2026-01-01 00:11:00', '2026-01-01 00:12:00', '2026-01-01 00:12:00');
INSERT INTO bot_history_messages (id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, metadata, created_at)
VALUES
  ('`+userID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 1, '`+rootTurnID+`', 1, 'user', '{"content":"u"}', '{}', '2026-01-01 00:01:00'),
  ('`+assistantID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 2, '`+rootTurnID+`', 2, 'assistant', '{"content":"a"}', '{}', '2026-01-01 00:02:00'),
  ('`+lateToolID+`', '`+botID+`', '`+sessionID+`', '`+rootBranchID+`', 3, '`+rootTurnID+`', 3, 'tool', '{"content":"late"}', '{}', '2026-01-01 00:03:00'),
  ('`+forkUserID+`', '`+botID+`', '`+sessionID+`', '`+forkBranchID+`', 1, '`+forkTurnID+`', 1, 'user', '{"content":"fu"}', '{}', '2026-01-01 00:11:00'),
  ('`+forkAssistantID+`', '`+botID+`', '`+sessionID+`', '`+forkBranchID+`', 2, '`+forkTurnID+`', 2, 'assistant', '{"content":"fa"}', '{}', '2026-01-01 00:12:00');
`)

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	rows, err := NewQueries(store).ListMessagesBySession(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("list active path: %v", err)
	}
	assertMessageIDs(t, rows, []string{userID, assistantID, forkUserID, forkAssistantID})
}

func TestSQLiteListSessionBranchesScansTurnForkColumns(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
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
`)

	sessionID := "00000000-0000-0000-0000-000000004001"
	rootBranchID := "00000000-0000-0000-0000-000000004010"
	forkBranchID := "00000000-0000-0000-0000-000000004011"
	messageID := "00000000-0000-0000-0000-000000004101"
	turnID := "00000000-0000-0000-0000-000000004201"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, active_branch_id) VALUES (?, ?)`, sessionID, forkBranchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id, created_at, updated_at) VALUES (?, ?, ?, ?)`, rootBranchID, sessionID, "2026-06-14 11:22:00", "2026-06-14 11:22:00"); err != nil {
		t.Fatalf("insert root branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_session_branches (
  id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq,
  fork_from_turn_id, fork_from_turn_seq, title, created_at, updated_at
) VALUES (?, ?, ?, ?, 2, ?, 1, 'fork title', ?, ?)`,
		forkBranchID,
		sessionID,
		rootBranchID,
		messageID,
		turnID,
		"2026-06-14 11:23:00",
		"2026-06-14 11:23:00",
	); err != nil {
		t.Fatalf("insert fork branch: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	rows, err := q.ListSessionBranches(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("list session branches: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("branches = %d, want 2", len(rows))
	}
	fork := rows[1]
	if fork.ForkFromTurnID.String() != turnID {
		t.Fatalf("fork_from_turn_id = %s, want %s", fork.ForkFromTurnID.String(), turnID)
	}
	if !fork.ForkFromTurnSeq.Valid || fork.ForkFromTurnSeq.Int64 != 1 {
		t.Fatalf("fork_from_turn_seq = %#v, want 1", fork.ForkFromTurnSeq)
	}
	if !fork.CreatedAt.Valid || fork.CreatedAt.Time.Format("2006-01-02 15:04:05") != "2026-06-14 11:23:00" {
		t.Fatalf("created_at = %#v, want 2026-06-14 11:23:00", fork.CreatedAt)
	}
}

func TestSQLiteListSessionBranchesToleratesMigratedColumnSkew(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
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
`)

	sessionID := "00000000-0000-0000-0000-000000005001"
	branchID := "00000000-0000-0000-0000-000000005010"
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, active_branch_id) VALUES (?, ?)`, sessionID, branchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_session_branches (
  id, session_id, fork_from_turn_id, fork_from_turn_seq, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?)`,
		branchID,
		sessionID,
		"2026-06-14 11:23:00",
		"2026-06-14 11:23:00",
		"",
		"",
	); err != nil {
		t.Fatalf("insert skewed branch: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	rows, err := q.ListSessionBranches(ctx, mustUUID(t, sessionID))
	if err != nil {
		t.Fatalf("list session branches with skewed columns: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("branches = %d, want 1", len(rows))
	}
	if rows[0].ForkFromTurnID.Valid {
		t.Fatalf("fork_from_turn_id = %#v, want null for skewed timestamp", rows[0].ForkFromTurnID)
	}
	if rows[0].ForkFromTurnSeq.Valid {
		t.Fatalf("fork_from_turn_seq = %#v, want null for skewed timestamp", rows[0].ForkFromTurnSeq)
	}
}

func TestSQLiteGetMessageForSessionBranchForkScansNoPreviousTurn(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = conn.Close() }()

	execAll(t, conn, `
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
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
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
  turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  turn_message_seq INTEGER,
  role TEXT NOT NULL,
  content TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`)

	botID := "00000000-0000-0000-0000-000000006001"
	sessionID := "00000000-0000-0000-0000-000000006002"
	branchID := "00000000-0000-0000-0000-000000006010"
	userID := "00000000-0000-0000-0000-000000006101"
	assistantID := "00000000-0000-0000-0000-000000006102"
	turnID := "00000000-0000-0000-0000-000000006201"

	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, active_branch_id) VALUES (?, ?)`, sessionID, branchID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id) VALUES (?, ?)`, branchID, sessionID); err != nil {
		t.Fatalf("insert branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (
  id, session_id, branch_id, turn_seq, request_message_id, final_assistant_message_id, status
) VALUES (?, ?, ?, 1, ?, ?, 'completed')`,
		turnID, sessionID, branchID, userID, assistantID); err != nil {
		t.Fatalf("insert turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, role, content, metadata
) VALUES (?, ?, ?, ?, 1, ?, 1, 'user', '{"content":"first"}', '{}')`,
		userID, botID, sessionID, branchID, turnID); err != nil {
		t.Fatalf("insert user message: %v", err)
	}

	store, err := New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	q := NewQueries(store)
	row, err := q.GetMessageForSessionBranchFork(ctx, pgsqlc.GetMessageForSessionBranchForkParams{
		MessageID: mustUUID(t, userID),
		SessionID: mustUUID(t, sessionID),
	})
	if err != nil {
		t.Fatalf("get fork message with no previous turn: %v", err)
	}
	if row.PreviousTurnID.Valid {
		t.Fatalf("previous_turn_id = %#v, want null", row.PreviousTurnID)
	}
	if row.PreviousTurnSeq != 0 {
		t.Fatalf("previous_turn_seq = %d, want 0", row.PreviousTurnSeq)
	}
	if row.TurnSeq != 1 || row.Role != "user" {
		t.Fatalf("fork row = %#v", row)
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

func assertTurnSeqs(t *testing.T, rows []pgsqlc.ListSessionBranchTurnMessagesRow, want []int64) {
	t.Helper()
	got := make([]int64, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.TurnSeq)
	}
	if len(got) != len(want) {
		t.Fatalf("turn seqs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("turn seqs = %#v, want %#v", got, want)
		}
	}
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
