package userinput

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/memohai/memoh/internal/config"
	"github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
	"github.com/memohai/memoh/internal/decision"
)

const (
	testBotID        = "00000000-0000-0000-0000-000000002001"
	testSessionID    = "00000000-0000-0000-0000-000000002002"
	testRootBranchID = "00000000-0000-0000-0000-000000002101"
	testForkBranchID = "00000000-0000-0000-0000-000000002102"
	testRootTurn1ID  = "00000000-0000-0000-0000-000000002201"
	testRootTurn2ID  = "00000000-0000-0000-0000-000000002202"
	testForkTurnID   = "00000000-0000-0000-0000-000000002203"
	testRootMsg1ID   = "00000000-0000-0000-0000-000000002301"
	testRootMsg2ID   = "00000000-0000-0000-0000-000000002302"
	testForkMsgID    = "00000000-0000-0000-0000-000000002303"
)

func newSQLiteUserInputService(t *testing.T) *Service {
	t.Helper()
	svc, _ := newSQLiteUserInputServiceWithDB(t)
	return svc
}

func newSQLiteUserInputServiceWithDB(t *testing.T) (*Service, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.OpenSQLite(ctx, config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// One :memory: database per pooled connection - force a single shared
	// connection so the waiter goroutine sees the schema.
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = conn.Close() })

	execTestSchema(t, conn, `
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
CREATE TABLE user_input_requests (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL DEFAULT 'ask_user',
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  input_json TEXT NOT NULL,
  ui_payload_json TEXT NOT NULL DEFAULT '{}',
  result_json TEXT NOT NULL DEFAULT '{}',
  provider_metadata TEXT NOT NULL DEFAULT '{}',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  responded_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  tool_result_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  persist_branch_id TEXT REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  persist_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  responded_at TEXT,
  canceled_at TEXT,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT user_input_tool_name_check CHECK (tool_name = 'ask_user'),
  CONSTRAINT user_input_status_check CHECK (status IN ('pending', 'submitted', 'canceled', 'expired', 'failed')),
  CONSTRAINT user_input_short_id_unique UNIQUE (session_id, short_id)
);
CREATE UNIQUE INDEX user_input_tool_call_legacy_unique
  ON user_input_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX user_input_tool_call_turn_unique
  ON user_input_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;
`)
	if _, err := conn.ExecContext(ctx, `INSERT INTO bots (id) VALUES (?)`, testBotID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id) VALUES (?)`, testSessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_session_branches (id, session_id) VALUES (?, ?)`, testRootBranchID, testSessionID); err != nil {
		t.Fatalf("insert root branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, final_assistant_message_id, status)
VALUES (?, ?, ?, 1, ?, 'completed'), (?, ?, ?, 2, ?, 'completed')`,
		testRootTurn1ID, testSessionID, testRootBranchID, testRootMsg1ID,
		testRootTurn2ID, testSessionID, testRootBranchID, testRootMsg2ID,
	); err != nil {
		t.Fatalf("insert root turns: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, session_id, branch_id, branch_seq, turn_id, role)
VALUES (?, ?, ?, 2, ?, 'assistant'), (?, ?, ?, 4, ?, 'assistant')`,
		testRootMsg1ID, testSessionID, testRootBranchID, testRootTurn1ID,
		testRootMsg2ID, testSessionID, testRootBranchID, testRootTurn2ID,
	); err != nil {
		t.Fatalf("insert root messages: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_session_branches (id, session_id, parent_branch_id, fork_from_message_id, fork_from_seq, fork_from_turn_id, fork_from_turn_seq)
VALUES (?, ?, ?, ?, 2, ?, 1)`,
		testForkBranchID, testSessionID, testRootBranchID, testRootMsg1ID, testRootTurn1ID); err != nil {
		t.Fatalf("insert fork branch: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_turns (id, session_id, branch_id, turn_seq, final_assistant_message_id, status)
VALUES (?, ?, ?, 1, ?, 'completed')`,
		testForkTurnID, testSessionID, testForkBranchID, testForkMsgID); err != nil {
		t.Fatalf("insert fork turn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `
INSERT INTO bot_history_messages (id, session_id, branch_id, branch_seq, turn_id, role)
VALUES (?, ?, ?, 2, ?, 'assistant')`,
		testForkMsgID, testSessionID, testForkBranchID, testForkTurnID); err != nil {
		t.Fatalf("insert fork message: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testRootBranchID, testSessionID); err != nil {
		t.Fatalf("set active branch: %v", err)
	}

	store, err := sqlitestore.New(conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return NewService(nil, sqlitestore.NewQueries(store)), conn
}

func execTestSchema(t *testing.T, conn *sql.DB, statement string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), statement); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
}

func createTestPending(t *testing.T, svc *Service, expiresAt *time.Time) Request {
	t.Helper()
	req, err := svc.CreatePending(context.Background(), testPendingInput("call-1", "", expiresAt))
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	return req
}

func testPendingInput(callID, branchID string, expiresAt *time.Time) CreatePendingInput {
	return testPendingInputForTurn(callID, branchID, "", expiresAt)
}

func testPendingInputForTurn(callID, branchID, turnID string, expiresAt *time.Time) CreatePendingInput {
	return CreatePendingInput{
		BotID:      testBotID,
		SessionID:  testSessionID,
		ToolCallID: callID,
		Input: map[string]any{
			"questions": []any{
				map[string]any{
					"text": "Which plan?",
					"kind": QuestionKindSingleSelect,
					"options": []any{
						map[string]any{"label": "Plan A"},
						map[string]any{"label": "Plan B"},
					},
				},
			},
		},
		PersistBranchID: branchID,
		PersistTurnID:   turnID,
		ExpiresAt:       expiresAt,
	}
}

func TestServiceSubmitLifecycleNotifiesWaiter(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	req := createTestPending(t, svc, nil)
	if req.Status != StatusPending || len(req.UIPayload.Questions) != 1 {
		t.Fatalf("unexpected pending request: %#v", req)
	}
	if req.UIPayload.Questions[0].ID != "q1" || req.UIPayload.Questions[0].Options[0].ID != "q1.o1" {
		t.Fatalf("unexpected normalized payload: %#v", req.UIPayload)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), decision.DefaultFallbackInterval/5)
	defer cancel()
	waited := make(chan Request, 1)
	waitErr := make(chan error, 1)
	go func() {
		resolved, err := svc.WaitForResponse(waitCtx, req.ID)
		if err != nil {
			waitErr <- err
			return
		}
		waited <- resolved
	}()

	// The waiter registry must reflect the blocked WaitForResponse: this is
	// the signal responders use to refuse answers nobody would consume.
	deadline := time.Now().Add(2 * time.Second)
	for !svc.HasWaiter(req.ID) {
		if time.Now().After(deadline) {
			t.Fatal("waiter never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}

	submitted, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o2"}}},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if submitted.Status != StatusSubmitted {
		t.Fatalf("submitted status = %q", submitted.Status)
	}
	answers, ok := submitted.Result["answers"].([]any)
	if !ok || len(answers) != 1 {
		t.Fatalf("unexpected result answers: %#v", submitted.Result)
	}

	// The wait context is shorter than the fallback ticker, so a timely return
	// here proves the Submit broadcast woke the waiter, not the polling safety
	// net.
	select {
	case resolved := <-waited:
		if resolved.Status != StatusSubmitted {
			t.Fatalf("waited status = %q", resolved.Status)
		}
	case err := <-waitErr:
		t.Fatalf("wait for response: %v", err)
	}
	if svc.HasWaiter(req.ID) {
		t.Fatal("waiter must unregister after the wait ends")
	}

	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("second submit error = %v, want ErrAlreadyDecided", err)
	}
}

func TestServiceCreatePendingDoesNotReuseTerminalRequest(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	req := createTestPending(t, svc, nil)
	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	_, err := svc.CreatePending(context.Background(), CreatePendingInput{
		BotID:      testBotID,
		SessionID:  testSessionID,
		ToolCallID: "call-1",
		Input: map[string]any{
			"questions": []any{
				map[string]any{
					"text": "Which plan?",
					"kind": QuestionKindSingleSelect,
					"options": []any{
						map[string]any{"label": "Plan A"},
						map[string]any{"label": "Plan B"},
					},
				},
			},
		},
	})
	if !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("CreatePending() error = %v, want ErrAlreadyDecided", err)
	}
}

func TestServiceWaitForRegisteredResponseUsesExistingWaiter(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	req := createTestPending(t, svc, nil)
	release := svc.RegisterWaiter(req.ID)
	defer release()

	waitCtx, cancel := context.WithTimeout(context.Background(), decision.DefaultFallbackInterval/5)
	defer cancel()
	waited := make(chan Request, 1)
	waitErr := make(chan error, 1)
	go func() {
		resolved, err := svc.WaitForRegisteredResponse(waitCtx, req.ID)
		if err != nil {
			waitErr <- err
			return
		}
		waited <- resolved
	}()

	if !svc.HasWaiter(req.ID) {
		t.Fatal("registered waiter was lost")
	}

	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case resolved := <-waited:
		if resolved.Status != StatusSubmitted {
			t.Fatalf("waited status = %q", resolved.Status)
		}
	case err := <-waitErr:
		t.Fatalf("wait for registered response: %v", err)
	case <-waitCtx.Done():
		t.Fatal("registered waiter was not notified")
	}
}

func TestServiceCancelNotifiesWaiter(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	req := createTestPending(t, svc, nil)

	waitCtx, cancel := context.WithTimeout(context.Background(), decision.DefaultFallbackInterval/5)
	defer cancel()
	waited := make(chan Request, 1)
	waitErr := make(chan error, 1)
	go func() {
		resolved, err := svc.WaitForResponse(waitCtx, req.ID)
		if err != nil {
			waitErr <- err
			return
		}
		waited <- resolved
	}()

	deadline := time.Now().Add(2 * time.Second)
	for !svc.HasWaiter(req.ID) {
		if time.Now().After(deadline) {
			t.Fatal("waiter never registered")
		}
		time.Sleep(5 * time.Millisecond)
	}

	canceled, err := svc.Cancel(context.Background(), CancelInput{RequestID: req.ID, Reason: "user input timed out"})
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if canceled.Status != StatusCanceled || canceled.Result["reason"] != "user input timed out" {
		t.Fatalf("unexpected canceled request: %#v", canceled)
	}

	select {
	case resolved := <-waited:
		if resolved.Status != StatusCanceled {
			t.Fatalf("waited status = %q", resolved.Status)
		}
	case err := <-waitErr:
		t.Fatalf("wait for response: %v", err)
	case <-waitCtx.Done():
		t.Fatal("waiter was not notified before the fallback ticker")
	}
}

func TestServiceCreatePendingIsIdempotentPerToolCall(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	first := createTestPending(t, svc, nil)
	second, err := svc.CreatePending(context.Background(), CreatePendingInput{
		BotID:      testBotID,
		SessionID:  testSessionID,
		ToolCallID: "call-1",
		Input: map[string]any{
			"questions": []any{
				map[string]any{"text": "Updated question?", "kind": QuestionKindText},
			},
		},
	})
	if err != nil {
		t.Fatalf("create duplicate pending: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("duplicate tool call created request %s, want existing %s", second.ID, first.ID)
	}
	if second.ShortID != first.ShortID {
		t.Fatalf("duplicate tool call short_id = %d, want %d", second.ShortID, first.ShortID)
	}

	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: second.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", Text: "done"}},
	}); err != nil {
		t.Fatalf("submit duplicate row: %v", err)
	}
	_, err = svc.CreatePending(context.Background(), CreatePendingInput{
		BotID:      testBotID,
		SessionID:  testSessionID,
		ToolCallID: "call-1",
		Input: map[string]any{
			"questions": []any{
				map[string]any{"text": "Third question?", "kind": QuestionKindText},
			},
		},
	})
	if !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("create duplicate after submit error = %v, want ErrAlreadyDecided", err)
	}
}

func TestServiceResolveTargetFiltersInactiveBranchRequests(t *testing.T) {
	svc, conn := newSQLiteUserInputServiceWithDB(t)
	ctx := context.Background()

	legacy, err := svc.CreatePending(ctx, testPendingInput("legacy", "", nil))
	if err != nil {
		t.Fatalf("create legacy pending: %v", err)
	}
	active, err := svc.CreatePending(ctx, testPendingInput("active", testRootBranchID, nil))
	if err != nil {
		t.Fatalf("create active pending: %v", err)
	}
	inactive, err := svc.CreatePending(ctx, testPendingInput("inactive", testForkBranchID, nil))
	if err != nil {
		t.Fatalf("create inactive pending: %v", err)
	}
	if _, err := svc.UpdatePromptMessage(ctx, inactive.ID, "", "inactive-reply"); err != nil {
		t.Fatalf("set inactive reply id: %v", err)
	}

	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: legacy.ID}); err != nil || got.ID != legacy.ID {
		t.Fatalf("resolve legacy UUID = (%#v, %v), want %s", got, err, legacy.ID)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: active.ID}); err != nil || got.ID != active.ID {
		t.Fatalf("resolve active UUID = (%#v, %v), want %s", got, err, active.ID)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = NULL WHERE id = ?`, testSessionID); err != nil {
		t.Fatalf("clear active branch: %v", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: active.ID}); err != nil || got.ID != active.ID {
		t.Fatalf("resolve root UUID via fallback = (%#v, %v), want %s", got, err, active.ID)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: "3"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive short id via fallback error = %v, want ErrNotFound", err)
	}
	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testRootBranchID, testSessionID); err != nil {
		t.Fatalf("restore active branch: %v", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: inactive.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive UUID error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, ExplicitID: inactive.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive no-session UUID error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Submit(ctx, SubmitInput{RequestID: inactive.ID, Answers: []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("submit inactive request error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Cancel(ctx, CancelInput{RequestID: inactive.ID, Reason: "inactive"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancel inactive request error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: "3"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive short id error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ReplyExternalMessageID: "inactive-reply"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve inactive reply error = %v, want ErrNotFound", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID}); err != nil || got.ID != active.ID {
		t.Fatalf("latest visible pending = (%#v, %v), want active %s", got, err, active.ID)
	}
	pending, err := svc.ListPendingBySession(ctx, testBotID, testSessionID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending count = %d, want legacy+active", len(pending))
	}
	for _, req := range pending {
		if req.ID == inactive.ID {
			t.Fatal("inactive branch request appeared in pending list")
		}
	}

	cancelled, err := svc.CancelPendingForSession(ctx, testBotID, testSessionID, "session closed")
	if err != nil {
		t.Fatalf("cancel pending: %v", err)
	}
	if len(cancelled) != 3 {
		t.Fatalf("cancelled count = %d, want legacy+active+inactive", len(cancelled))
	}
	if got, err := svc.Get(ctx, inactive.ID); err != nil || got.Status != StatusCanceled {
		t.Fatalf("inactive request after cancel = (%#v, %v), want canceled", got, err)
	}
}

func TestServicePendingUserInputUsesActiveVisibleTurnPath(t *testing.T) {
	svc, conn := newSQLiteUserInputServiceWithDB(t)
	ctx := context.Background()

	rootBoundary, err := svc.CreatePending(ctx, testPendingInputForTurn("root-boundary", testRootBranchID, testRootTurn1ID, nil))
	if err != nil {
		t.Fatalf("create root boundary request: %v", err)
	}
	rootAfterFork, err := svc.CreatePending(ctx, testPendingInputForTurn("root-after-fork", testRootBranchID, testRootTurn2ID, nil))
	if err != nil {
		t.Fatalf("create root after fork request: %v", err)
	}
	forkRequest, err := svc.CreatePending(ctx, testPendingInputForTurn("fork-visible", testForkBranchID, testForkTurnID, nil))
	if err != nil {
		t.Fatalf("create fork request: %v", err)
	}
	if _, err := svc.UpdatePromptMessage(ctx, rootAfterFork.ID, "", "root-after-fork-reply"); err != nil {
		t.Fatalf("set root-after-fork reply: %v", err)
	}

	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testForkBranchID, testSessionID); err != nil {
		t.Fatalf("activate fork branch: %v", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: rootBoundary.ID}); err != nil || got.ID != rootBoundary.ID {
		t.Fatalf("resolve root boundary on fork = (%#v, %v), want %s", got, err, rootBoundary.ID)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: rootAfterFork.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve root after fork on fork error = %v, want ErrNotFound", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ReplyExternalMessageID: "root-after-fork-reply"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve hidden reply error = %v, want ErrNotFound", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID}); err != nil || got.ID != forkRequest.ID {
		t.Fatalf("latest visible on fork = (%#v, %v), want %s", got, err, forkRequest.ID)
	}
	pending, err := svc.ListPendingBySession(ctx, testBotID, testSessionID)
	if err != nil {
		t.Fatalf("list fork pending: %v", err)
	}
	assertUserInputIDs(t, pending, []string{rootBoundary.ID, forkRequest.ID})

	if _, err := conn.ExecContext(ctx, `UPDATE bot_sessions SET active_branch_id = ? WHERE id = ?`, testRootBranchID, testSessionID); err != nil {
		t.Fatalf("activate root branch: %v", err)
	}
	if _, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID, ExplicitID: forkRequest.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve fork request on root error = %v, want ErrNotFound", err)
	}
	if got, err := svc.ResolveTarget(ctx, ResolveInput{BotID: testBotID, SessionID: testSessionID}); err != nil || got.ID != rootAfterFork.ID {
		t.Fatalf("latest visible on root = (%#v, %v), want %s", got, err, rootAfterFork.ID)
	}
}

func TestServiceCreatePendingUserInputIsIdempotentPerTurn(t *testing.T) {
	svc, conn := newSQLiteUserInputServiceWithDB(t)
	ctx := context.Background()

	first, err := svc.CreatePending(ctx, testPendingInputForTurn("replayed-call", testRootBranchID, testRootTurn1ID, nil))
	if err != nil {
		t.Fatalf("create first turn request: %v", err)
	}
	duplicate, err := svc.CreatePending(ctx, testPendingInputForTurn("replayed-call", testRootBranchID, testRootTurn1ID, nil))
	if err != nil {
		t.Fatalf("create duplicate turn request: %v", err)
	}
	if duplicate.ID != first.ID {
		t.Fatalf("duplicate turn request ID = %s, want %s", duplicate.ID, first.ID)
	}

	if _, err := svc.Submit(ctx, SubmitInput{
		RequestID: first.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); err != nil {
		t.Fatalf("submit first request: %v", err)
	}
	secondTurn, err := svc.CreatePending(ctx, testPendingInputForTurn("replayed-call", testRootBranchID, testRootTurn2ID, nil))
	if err != nil {
		t.Fatalf("create same call on second turn: %v", err)
	}
	if secondTurn.ID == first.ID {
		t.Fatalf("second turn reused first request %s", first.ID)
	}
	if secondTurn.ShortID == first.ShortID {
		t.Fatalf("second turn short_id = %d, want a new session-scoped short id", secondTurn.ShortID)
	}

	var firstPersist, secondPersist string
	if err := conn.QueryRowContext(ctx, `SELECT persist_turn_id FROM user_input_requests WHERE id = ?`, first.ID).Scan(&firstPersist); err != nil {
		t.Fatalf("select first persist turn: %v", err)
	}
	if err := conn.QueryRowContext(ctx, `SELECT persist_turn_id FROM user_input_requests WHERE id = ?`, secondTurn.ID).Scan(&secondPersist); err != nil {
		t.Fatalf("select second persist turn: %v", err)
	}
	if firstPersist != testRootTurn1ID || secondPersist != testRootTurn2ID {
		t.Fatalf("persist turns = %q/%q, want %q/%q", firstPersist, secondPersist, testRootTurn1ID, testRootTurn2ID)
	}
}

func assertUserInputIDs(t *testing.T, got []Request, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("user input ids len = %d, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].ID != want[i] {
			t.Fatalf("user input ids[%d] = %s, want %s; all = %#v", i, got[i].ID, want[i], got)
		}
	}
}

func TestServiceWaitPrefersResolutionOverContextCancel(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	req := createTestPending(t, svc, nil)

	waitCtx, cancelWait := context.WithCancel(context.Background())
	defer cancelWait()
	waited := make(chan Request, 1)
	waitErr := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		resolved, err := svc.WaitForResponse(waitCtx, req.ID)
		if err != nil {
			waitErr <- err
			return
		}
		waited <- resolved
	}()
	<-started

	// Submit commits (and buffers the notification) before the context is
	// canceled; even if ctx.Done wins the select, the waiter must deliver the
	// answer, never ctx.Err().
	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	cancelWait()

	select {
	case resolved := <-waited:
		if resolved.Status != StatusSubmitted {
			t.Fatalf("waited status = %q", resolved.Status)
		}
	case err := <-waitErr:
		t.Fatalf("wait returned error despite committed answer: %v", err)
	case <-time.After(4 * time.Second):
		t.Fatal("waiter did not return")
	}
}

func TestServiceCancelAfterDecisionReturnsAlreadyDecided(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	req := createTestPending(t, svc, nil)

	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}

	// The guarded update matches no row; the error must disambiguate to
	// ErrAlreadyDecided instead of pretending the request does not exist.
	if _, err := svc.Cancel(context.Background(), CancelInput{RequestID: req.ID, Reason: "late"}); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("cancel after decision error = %v, want ErrAlreadyDecided", err)
	}
}

func TestServiceExpiredRequestIsClosed(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	expired := time.Now().Add(-time.Minute)
	req := createTestPending(t, svc, &expired)

	got, err := svc.Get(context.Background(), req.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != StatusExpired {
		t.Fatalf("status = %q, want %q", got.Status, StatusExpired)
	}

	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: req.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); !errors.Is(err, ErrAlreadyDecided) {
		t.Fatalf("submit expired error = %v, want ErrAlreadyDecided", err)
	}

	if _, err := svc.ResolveTarget(context.Background(), ResolveInput{
		BotID:     testBotID,
		SessionID: testSessionID,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve target error = %v, want ErrNotFound", err)
	}

	pending, err := svc.ListPendingBySession(context.Background(), testBotID, testSessionID)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending = %d, want 0", len(pending))
	}

	// A future expiry must keep the request answerable.
	future := time.Now().Add(time.Hour)
	live := createTestPendingWithCallID(t, svc, &future, "call-2")
	gotLive, err := svc.Get(context.Background(), live.ID)
	if err != nil {
		t.Fatalf("get live: %v", err)
	}
	if gotLive.Status != StatusPending {
		t.Fatalf("live status = %q, want pending", gotLive.Status)
	}
	if _, err := svc.Submit(context.Background(), SubmitInput{
		RequestID: live.ID,
		Answers:   []QuestionAnswer{{QuestionID: "q1", OptionIDs: []string{"q1.o1"}}},
	}); err != nil {
		t.Fatalf("submit live: %v", err)
	}
}

func TestServiceACPMCPMarkerRoundtrip(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc := newSQLiteUserInputService(t)
	marked, err := svc.CreatePending(context.Background(), CreatePendingInput{
		BotID:            testBotID,
		SessionID:        testSessionID,
		ToolCallID:       "acp-mcp-call",
		ProviderMetadata: map[string]any{"source": ProviderSourceACPMCP},
		Input: map[string]any{
			"questions": []any{
				map[string]any{"text": "Proceed?", "kind": QuestionKindText},
			},
		},
	})
	if err != nil {
		t.Fatalf("create acp pending: %v", err)
	}
	got, err := svc.Get(context.Background(), marked.ID)
	if err != nil {
		t.Fatalf("get acp pending: %v", err)
	}
	if !IsACPMCPRequest(got) {
		t.Fatalf("IsACPMCPRequest = false after round trip, metadata = %#v", got.ProviderMetadata)
	}

	plain := createTestPendingWithCallID(t, svc, nil, "native-call")
	gotPlain, err := svc.Get(context.Background(), plain.ID)
	if err != nil {
		t.Fatalf("get native pending: %v", err)
	}
	if IsACPMCPRequest(gotPlain) {
		t.Fatalf("native request misclassified as ACP/MCP: %#v", gotPlain.ProviderMetadata)
	}
}

func createTestPendingWithCallID(t *testing.T, svc *Service, expiresAt *time.Time, callID string) Request {
	t.Helper()
	req, err := svc.CreatePending(context.Background(), testPendingInput(callID, "", expiresAt))
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	return req
}
