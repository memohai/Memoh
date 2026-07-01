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
	testBotID     = "00000000-0000-0000-0000-000000002001"
	testSessionID = "00000000-0000-0000-0000-000000002002"
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
  bot_id TEXT,
  default_head_turn_id TEXT,
  deleted_at TEXT
);
CREATE UNIQUE INDEX bot_sessions_id_bot_unique ON bot_sessions(id, bot_id);
CREATE TABLE bot_channel_routes (
  id TEXT PRIMARY KEY
);
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY
);
CREATE TABLE bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT,
  owner_session_id TEXT,
  parent_turn_id TEXT
);
CREATE TABLE bot_session_turn_heads (
  session_id TEXT NOT NULL,
  head_turn_id TEXT NOT NULL,
  bot_id TEXT NOT NULL,
  PRIMARY KEY (session_id, head_turn_id)
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY
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
  FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE,
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
	if _, err := conn.ExecContext(ctx, `INSERT INTO bot_sessions (id, bot_id) VALUES (?, ?)`, testSessionID, testBotID); err != nil {
		t.Fatalf("insert session: %v", err)
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
	req, err := svc.CreatePending(context.Background(), CreatePendingInput{
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
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	return req
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

func TestServiceResolveTargetChecksSelectedHeadPath(t *testing.T) {
	// Not parallel: concurrent :memory: opens race in modernc sqlite's
	// global initializer and fail under -race.

	svc, conn := newSQLiteUserInputServiceWithDB(t)

	pendingTurnID := "00000000-0000-0000-0000-000000002101"
	selectedHeadID := "00000000-0000-0000-0000-000000002102"
	otherHeadID := "00000000-0000-0000-0000-000000002103"
	ctx := context.Background()
	insertTurn := func(id, parentID string) {
		t.Helper()
		var err error
		if parentID == "" {
			_, err = conn.ExecContext(ctx,
				`INSERT INTO bot_history_turns (id, bot_id, owner_session_id, parent_turn_id) VALUES (?, ?, ?, NULL)`,
				id, testBotID, testSessionID,
			)
		} else {
			_, err = conn.ExecContext(ctx,
				`INSERT INTO bot_history_turns (id, bot_id, owner_session_id, parent_turn_id) VALUES (?, ?, ?, ?)`,
				id, testBotID, testSessionID, parentID,
			)
		}
		if err != nil {
			t.Fatalf("insert turn %s: %v", id, err)
		}
	}
	insertHead := func(id string) {
		t.Helper()
		if _, err := conn.ExecContext(ctx,
			`INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id) VALUES (?, ?, ?)`,
			testSessionID, id, testBotID,
		); err != nil {
			t.Fatalf("insert head %s: %v", id, err)
		}
	}
	insertTurn(pendingTurnID, "")
	insertTurn(selectedHeadID, pendingTurnID)
	insertTurn(otherHeadID, "")
	insertHead(selectedHeadID)
	insertHead(otherHeadID)

	req, err := svc.CreatePending(context.Background(), CreatePendingInput{
		BotID:         testBotID,
		SessionID:     testSessionID,
		ToolCallID:    "path-call",
		PersistTurnID: pendingTurnID,
		Input: map[string]any{
			"questions": []any{
				map[string]any{"text": "Proceed?", "kind": QuestionKindText},
			},
		},
	})
	if err != nil {
		t.Fatalf("create path pending: %v", err)
	}

	resolved, err := svc.ResolveTarget(context.Background(), ResolveInput{
		BotID:          testBotID,
		SessionID:      testSessionID,
		BaseHeadTurnID: selectedHeadID,
		ExplicitID:     req.ID,
	})
	if err != nil {
		t.Fatalf("resolve selected path: %v", err)
	}
	if resolved.ID != req.ID {
		t.Fatalf("resolved ID = %q, want %q", resolved.ID, req.ID)
	}

	if _, err := svc.ResolveTarget(context.Background(), ResolveInput{
		BotID:          testBotID,
		SessionID:      testSessionID,
		BaseHeadTurnID: otherHeadID,
		ExplicitID:     req.ID,
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("resolve outside selected path error = %v, want ErrNotFound", err)
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
	req, err := svc.CreatePending(context.Background(), CreatePendingInput{
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
		ExpiresAt: expiresAt,
	})
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	return req
}
