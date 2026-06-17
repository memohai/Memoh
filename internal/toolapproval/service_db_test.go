package toolapproval

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
)

const (
	testApprovalBotID      = "00000000-0000-0000-0000-000000003001"
	testApprovalSessionID  = "00000000-0000-0000-0000-000000003002"
	testApprovalRootBranch = "00000000-0000-0000-0000-000000003101"
	testApprovalForkBranch = "00000000-0000-0000-0000-000000003102"
	testApprovalRootTurn1  = "00000000-0000-0000-0000-000000003201"
	testApprovalRootTurn2  = "00000000-0000-0000-0000-000000003202"
	testApprovalForkTurn   = "00000000-0000-0000-0000-000000003203"
	testApprovalRootMsg1   = "00000000-0000-0000-0000-000000003301"
	testApprovalRootMsg2   = "00000000-0000-0000-0000-000000003302"
	testApprovalForkMsg    = "00000000-0000-0000-0000-000000003303"
)

func newSQLiteToolApprovalServiceWithDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	execToolApprovalTestSchema(t, conn, `
CREATE TABLE bots (
  id TEXT PRIMARY KEY
);
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
CREATE TABLE bot_channel_routes (
  id TEXT PRIMARY KEY
);
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY,
  session_id TEXT REFERENCES bot_sessions(id) ON DELETE SET NULL,
  branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  branch_seq INTEGER,
  turn_id TEXT,
  role TEXT NOT NULL DEFAULT 'assistant'
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  branch_id TEXT NOT NULL REFERENCES bot_session_branches(id) ON DELETE CASCADE,
  turn_seq INTEGER NOT NULL,
  request_message_id TEXT,
  final_assistant_message_id TEXT,
  status TEXT NOT NULL DEFAULT 'running',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE tool_approval_requests (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  operation TEXT NOT NULL,
  tool_input TEXT NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  persist_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  persist_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at TEXT,
  CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id)
);
`)
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id) VALUES (?)`, testApprovalBotID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, active_branch_id) VALUES (?, ?)`, testApprovalSessionID, testApprovalRootBranch); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id) VALUES (?, ?)`, testApprovalRootBranch, testApprovalSessionID); err != nil {
		t.Fatalf("insert root branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, final_assistant_message_id, status)
VALUES (?, ?, ?, 1, ?, 'completed'), (?, ?, ?, 2, ?, 'completed')`,
		testApprovalRootTurn1, testApprovalSessionID, testApprovalRootBranch, testApprovalRootMsg1,
		testApprovalRootTurn2, testApprovalSessionID, testApprovalRootBranch, testApprovalRootMsg2,
	); err != nil {
		t.Fatalf("insert root turns: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, session_id, branch_id, branch_seq, turn_id, role)
VALUES (?, ?, ?, 2, ?, 'assistant'), (?, ?, ?, 4, ?, 'assistant')`,
		testApprovalRootMsg1, testApprovalSessionID, testApprovalRootBranch, testApprovalRootTurn1,
		testApprovalRootMsg2, testApprovalSessionID, testApprovalRootBranch, testApprovalRootTurn2,
	); err != nil {
		t.Fatalf("insert root messages: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, fork_from_turn_id, fork_from_turn_seq)
VALUES (?, ?, ?, ?, 2, ?, 1)`,
		testApprovalForkBranch, testApprovalSessionID, testApprovalRootBranch, testApprovalRootMsg1, testApprovalRootTurn1); err != nil {
		t.Fatalf("insert fork branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, final_assistant_message_id, status)
VALUES (?, ?, ?, 1, ?, 'completed')`,
		testApprovalForkTurn, testApprovalSessionID, testApprovalForkBranch, testApprovalForkMsg); err != nil {
		t.Fatalf("insert fork turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, session_id, branch_id, branch_seq, turn_id, role)
VALUES (?, ?, ?, 2, ?, 'assistant')`,
		testApprovalForkMsg, testApprovalSessionID, testApprovalForkBranch, testApprovalForkTurn); err != nil {
		t.Fatalf("insert fork message: %v", err)
	}

	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return NewService(nil, sqlitestore.NewQueries(store), nil), conn
}

func execToolApprovalTestSchema(t *testing.T, conn *sql.DB, statement string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), statement); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
}

func testApprovalInput(callID, branchID string) CreatePendingInput {
	return testApprovalInputForTurn(callID, branchID, "")
}

func testApprovalInputForTurn(callID, branchID, turnID string) CreatePendingInput {
	return CreatePendingInput{
		BotID:           testApprovalBotID,
		SessionID:       testApprovalSessionID,
		ToolCallID:      callID,
		ToolName:        "exec",
		ToolInput:       map[string]any{"command": "true"},
		PersistBranchID: branchID,
		PersistTurnID:   turnID,
	}
}

func TestServiceResolveTargetFiltersInactiveBranchApprovals(t *testing.T) {
	svc, conn := newSQLiteToolApprovalServiceWithDB(t)
	ctx := context.Background()

	legacy, err := svc.CreatePending(ctx, testApprovalInput("legacy", ""))
	if err != nil {
		t.Fatalf("create legacy approval: %v", err)
	}
	active, err := svc.CreatePending(ctx, testApprovalInput("active", testApprovalRootBranch))
	if err != nil {
		t.Fatalf("create active approval: %v", err)
	}
	inactive, err := svc.CreatePending(ctx, testApprovalInput("inactive", testApprovalForkBranch))
	if err != nil {
		t.Fatalf("create inactive approval: %v", err)
	}
	if _, err := svc.UpdatePromptMessage(ctx, inactive.ID, "", "inactive-reply"); err != nil {
		t.Fatalf("set inactive reply id: %v", err)
	}

	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: legacy.ID}); err != nil || got.ID != legacy.ID {
		t.Fatalf("resolve legacy UUID = (%#v, %v), want %s", got, err, legacy.ID)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: active.ID}); err != nil || got.ID != active.ID {
		t.Fatalf("resolve active UUID = (%#v, %v), want %s", got, err, active.ID)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = NULL WHERE id = ?`, testApprovalSessionID); err != nil {
		t.Fatalf("clear active branch: %v", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: active.ID}); err != nil || got.ID != active.ID {
		t.Fatalf("resolve root UUID via fallback = (%#v, %v), want %s", got, err, active.ID)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: "3"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive short id via fallback error = %v, want ErrNotFound", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testApprovalRootBranch, testApprovalSessionID); err != nil {
		t.Fatalf("restore active branch: %v", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: inactive.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive UUID error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, ExplicitID: inactive.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive no-session UUID error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Approve(ctx, inactive.ID, "", "inactive"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approve inactive error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Reject(ctx, inactive.ID, "", "inactive"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reject inactive error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: "3"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive short id error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ReplyExternalMessageID: "inactive-reply"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive reply error = %v, want ErrNotFound", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID}); err != nil || got.ID != active.ID {
		t.Fatalf("latest visible approval = (%#v, %v), want active %s", got, err, active.ID)
	}
	pending, err := svc.ListPendingBySession(ctx, testApprovalBotID, testApprovalSessionID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending count = %d, want legacy+active", len(pending))
	}
	for _, req := range pending {
		if req.ID == inactive.ID {
			t.Fatal("inactive branch approval appeared in pending list")
		}
	}

	cancelled, err := svc.CancelPendingForSession(ctx, testApprovalBotID, testApprovalSessionID, "session closed")
	if err != nil {
		t.Fatalf("cancel pending: %v", err)
	}
	if len(cancelled) != 3 {
		t.Fatalf("cancelled count = %d, want legacy+active+inactive", len(cancelled))
	}
	if got, err := svc.Get(ctx, inactive.ID); err != nil || got.Status != StatusCancelled {
		t.Fatalf("inactive approval after cancel = (%#v, %v), want cancelled", got, err)
	}
}

func TestServicePendingApprovalUsesActiveVisibleTurnPath(t *testing.T) {
	svc, conn := newSQLiteToolApprovalServiceWithDB(t)
	ctx := context.Background()

	rootBoundary, err := svc.CreatePending(ctx, testApprovalInputForTurn("root-boundary", testApprovalRootBranch, testApprovalRootTurn1))
	if err != nil {
		t.Fatalf("create root boundary approval: %v", err)
	}
	rootAfterFork, err := svc.CreatePending(ctx, testApprovalInputForTurn("root-after-fork", testApprovalRootBranch, testApprovalRootTurn2))
	if err != nil {
		t.Fatalf("create root after fork approval: %v", err)
	}
	forkApproval, err := svc.CreatePending(ctx, testApprovalInputForTurn("fork-visible", testApprovalForkBranch, testApprovalForkTurn))
	if err != nil {
		t.Fatalf("create fork approval: %v", err)
	}
	if _, err := svc.UpdatePromptMessage(ctx, rootAfterFork.ID, "", "root-after-fork-reply"); err != nil {
		t.Fatalf("set root-after-fork reply: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testApprovalForkBranch, testApprovalSessionID); err != nil {
		t.Fatalf("activate fork branch: %v", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: rootBoundary.ID}); err != nil || got.ID != rootBoundary.ID {
		t.Fatalf("resolve root boundary on fork = (%#v, %v), want %s", got, err, rootBoundary.ID)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: rootAfterFork.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve root after fork on fork error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ReplyExternalMessageID: "root-after-fork-reply"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve hidden reply error = %v, want ErrNotFound", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID}); err != nil || got.ID != forkApproval.ID {
		t.Fatalf("latest visible on fork = (%#v, %v), want %s", got, err, forkApproval.ID)
	}
	pending, err := svc.ListPendingBySession(ctx, testApprovalBotID, testApprovalSessionID)
	if err != nil {
		t.Fatalf("list fork pending: %v", err)
	}
	assertApprovalIDs(t, pending, []string{rootBoundary.ID, forkApproval.ID})

	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testApprovalRootBranch, testApprovalSessionID); err != nil {
		t.Fatalf("activate root branch: %v", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID, ExplicitID: forkApproval.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve fork approval on root error = %v, want ErrNotFound", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testApprovalBotID, SessionID: testApprovalSessionID}); err != nil || got.ID != rootAfterFork.ID {
		t.Fatalf("latest visible on root = (%#v, %v), want %s", got, err, rootAfterFork.ID)
	}
}

func assertApprovalIDs(t *testing.T, got []Request, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("approval ids len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("approval ids[%d] = %s, want %s; all = %#v", i, got[i].ID, want[i], got)
		}
	}
}
