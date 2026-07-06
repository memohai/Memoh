package message

import (
	"context"
	"database/sql"
	"testing"

	"github.com/memohai/memoh/internal/config"
	dbpkg "github.com/memohai/memoh/internal/db"
	sqlitestore "github.com/memohai/memoh/internal/db/sqlite/store"
)

func TestSQLiteHistoryTurnsRetryReplacementKeepsSameSecondRequestVisible(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003001"
		sessionID = "00000000-0000-0000-0000-000000003002"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	user, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("persist user: %v", err)
	}
	assistant, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"old"}`),
	})
	if err != nil {
		t.Fatalf("persist assistant: %v", err)
	}
	if user.CreatedAt.IsZero() || assistant.CreatedAt.IsZero() {
		t.Fatalf("persisted message created_at must parse, got user=%v assistant=%v", user.CreatedAt, assistant.CreatedAt)
	}

	turn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, assistant.ID)
	if err != nil {
		t.Fatalf("visible turn by assistant: %v", err)
	}
	if turn.RequestMessageID != user.ID {
		t.Fatalf("turn request = %s, want %s", turn.RequestMessageID, user.ID)
	}
	if turn.AssistantMessageID != assistant.ID {
		t.Fatalf("turn assistant = %s, want %s", turn.AssistantMessageID, assistant.ID)
	}

	replacement, err := svc.Persist(ctx, PersistInput{
		BotID:           botID,
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist replacement assistant: %v", err)
	}
	if _, err := svc.ReplaceTurn(ctx, sessionID, turn.ID, user.ID, replacement.ID, "retry"); err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	messages, err := svc.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if got := messageIDs(messages); len(got) != 2 || got[0] != user.ID || got[1] != replacement.ID {
		t.Fatalf("visible message ids = %#v, want user + replacement", got)
	}
	if _, err := svc.GetVisibleTurnByMessage(ctx, sessionID, assistant.ID); err == nil {
		t.Fatal("old assistant still has visible turn after replacement")
	}
}

func TestSQLiteHistoryTurnsAssistantBindsRequestedPendingTurn(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003101"
		sessionID = "00000000-0000-0000-0000-000000003102"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	firstUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"first"}`),
	})
	if err != nil {
		t.Fatalf("persist first user: %v", err)
	}
	secondUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"second"}`),
	})
	if err != nil {
		t.Fatalf("persist second user: %v", err)
	}
	assistant, err := svc.Persist(ctx, PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "assistant",
		Content:              []byte(`{"role":"assistant","content":"answer first"}`),
		TurnRequestMessageID: firstUser.ID,
	})
	if err != nil {
		t.Fatalf("persist requested assistant: %v", err)
	}

	turn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, assistant.ID)
	if err != nil {
		t.Fatalf("visible turn by requested assistant: %v", err)
	}
	if turn.RequestMessageID != firstUser.ID {
		t.Fatalf("turn request = %s, want first user %s", turn.RequestMessageID, firstUser.ID)
	}
	if turn.RequestMessageID == secondUser.ID {
		t.Fatalf("assistant was bound to latest pending user %s instead of requested user", secondUser.ID)
	}
	finalAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "assistant",
		Content:              []byte(`{"role":"assistant","content":"final first"}`),
		TurnRequestMessageID: firstUser.ID,
	})
	if err != nil {
		t.Fatalf("persist final assistant for requested turn: %v", err)
	}
	finalTurn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, finalAssistant.ID)
	if err != nil {
		t.Fatalf("visible turn by final assistant: %v", err)
	}
	if finalTurn.ID != turn.ID {
		t.Fatalf("final assistant turn = %s, want first assistant turn %s", finalTurn.ID, turn.ID)
	}
}

func TestSQLiteHistoryTurnsConsecutiveRoundsStaySeparate(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003121"
		sessionID = "00000000-0000-0000-0000-000000003122"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	firstUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"first"}`),
	})
	if err != nil {
		t.Fatalf("persist first user: %v", err)
	}
	firstAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "assistant",
		Content:              []byte(`{"role":"assistant","content":"answer first"}`),
		TurnRequestMessageID: firstUser.ID,
	})
	if err != nil {
		t.Fatalf("persist first assistant: %v", err)
	}
	secondUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"second"}`),
	})
	if err != nil {
		t.Fatalf("persist second user: %v", err)
	}
	secondAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "assistant",
		Content:              []byte(`{"role":"assistant","content":"answer second"}`),
		TurnRequestMessageID: secondUser.ID,
	})
	if err != nil {
		t.Fatalf("persist second assistant: %v", err)
	}

	messages, err := svc.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	want := []string{firstUser.ID, firstAssistant.ID, secondUser.ID, secondAssistant.ID}
	if got := messageIDs(messages); !equalStringSlices(got, want) {
		t.Fatalf("visible message ids = %#v, want %#v", got, want)
	}
	firstTurn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, firstAssistant.ID)
	if err != nil {
		t.Fatalf("visible turn by first assistant: %v", err)
	}
	secondTurn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, secondAssistant.ID)
	if err != nil {
		t.Fatalf("visible turn by second assistant: %v", err)
	}
	if firstTurn.ID == secondTurn.ID {
		t.Fatalf("assistants share turn %s, want separate turns", firstTurn.ID)
	}
	if secondTurn.RequestMessageID != secondUser.ID {
		t.Fatalf("second turn request = %s, want second user %s", secondTurn.RequestMessageID, secondUser.ID)
	}
}

func TestSQLiteHistoryTurnsToolTailAppendsToRequestedTurn(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003111"
		sessionID = "00000000-0000-0000-0000-000000003112"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	firstUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"first"}`),
	})
	if err != nil {
		t.Fatalf("persist first user: %v", err)
	}
	secondUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"second"}`),
	})
	if err != nil {
		t.Fatalf("persist second user: %v", err)
	}
	assistant, err := svc.Persist(ctx, PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "assistant",
		Content:              []byte(`{"role":"assistant","content":"tool call"}`),
		TurnRequestMessageID: firstUser.ID,
	})
	if err != nil {
		t.Fatalf("persist requested assistant: %v", err)
	}
	tool, err := svc.Persist(ctx, PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "tool",
		Content:              []byte(`{"role":"tool","content":"result"}`),
		TurnRequestMessageID: firstUser.ID,
	})
	if err != nil {
		t.Fatalf("persist requested tool tail: %v", err)
	}

	assistantTurn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, assistant.ID)
	if err != nil {
		t.Fatalf("visible turn by assistant: %v", err)
	}
	toolTurn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, tool.ID)
	if err != nil {
		t.Fatalf("visible turn by tool: %v", err)
	}
	if toolTurn.ID != assistantTurn.ID {
		t.Fatalf("tool turn = %s, want assistant turn %s", toolTurn.ID, assistantTurn.ID)
	}
	if toolTurn.RequestMessageID != firstUser.ID {
		t.Fatalf("tool turn request = %s, want first user %s", toolTurn.RequestMessageID, firstUser.ID)
	}
	if toolTurn.RequestMessageID == secondUser.ID {
		t.Fatalf("tool tail was appended to latest pending user %s instead of requested user", secondUser.ID)
	}
}

func TestSQLiteHistoryTurnsReplacementKeepsToolTailAndHidesSupersededTail(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003101"
		sessionID = "00000000-0000-0000-0000-000000003102"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	firstUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"first"}`),
	})
	if err != nil {
		t.Fatalf("persist first user: %v", err)
	}
	firstAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"first answer"}`),
	})
	if err != nil {
		t.Fatalf("persist first assistant: %v", err)
	}
	secondUser, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"second"}`),
	})
	if err != nil {
		t.Fatalf("persist second user: %v", err)
	}
	oldAssistant, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"old answer"}`),
	})
	if err != nil {
		t.Fatalf("persist old assistant: %v", err)
	}
	oldToolTail, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "tool",
		Content:   []byte(`{"role":"tool","content":"old tool tail"}`),
	})
	if err != nil {
		t.Fatalf("persist old tool tail: %v", err)
	}

	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, sessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("visible old turn: %v", err)
	}
	newAssistantAnchor, err := svc.Persist(ctx, PersistInput{
		BotID:           botID,
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new calls tool"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist new assistant anchor: %v", err)
	}
	newToolTail, err := svc.Persist(ctx, PersistInput{
		BotID:           botID,
		SessionID:       sessionID,
		Role:            "tool",
		Content:         []byte(`{"role":"tool","content":"new tool tail"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist new tool tail: %v", err)
	}
	newAssistantFinal, err := svc.Persist(ctx, PersistInput{
		BotID:           botID,
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new final"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist new assistant final: %v", err)
	}
	if _, err := svc.ReplaceTurn(ctx, sessionID, oldTurn.ID, secondUser.ID, newAssistantAnchor.ID, "retry"); err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	messages, err := svc.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	want := []string{
		firstUser.ID,
		firstAssistant.ID,
		secondUser.ID,
		newAssistantAnchor.ID,
		newToolTail.ID,
		newAssistantFinal.ID,
	}
	if got := messageIDs(messages); !equalStringSlices(got, want) {
		t.Fatalf("visible message ids = %#v, want %#v; old tool tail %s must stay hidden", got, want, oldToolTail.ID)
	}
}

func TestSQLiteDeleteByIDsRemovesUnanchoredReplacementMessages(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003201"
		sessionID = "00000000-0000-0000-0000-000000003202"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	user, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"hello"}`),
	})
	if err != nil {
		t.Fatalf("persist user: %v", err)
	}
	assistant, err := svc.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "assistant",
		Content:   []byte(`{"role":"assistant","content":"old"}`),
	})
	if err != nil {
		t.Fatalf("persist assistant: %v", err)
	}
	replacement, err := svc.Persist(ctx, PersistInput{
		BotID:           botID,
		SessionID:       sessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"new"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist replacement assistant: %v", err)
	}
	if err := svc.DeleteByIDs(ctx, []string{replacement.ID}); err != nil {
		t.Fatalf("delete replacement: %v", err)
	}
	messages, err := svc.ListBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if got := messageIDs(messages); !equalStringSlices(got, []string{user.ID, assistant.ID}) {
		t.Fatalf("visible message ids = %#v, want original turn only", got)
	}
}

func TestSQLiteLocateByExternalIDReturnsTargetWindowInVisibleOrder(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003301"
		sessionID = "00000000-0000-0000-0000-000000003302"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	user1, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "user",
		Content:           []byte(`{"role":"user","content":"first"}`),
		ExternalMessageID: "external-user-1",
	})
	if err != nil {
		t.Fatalf("persist user1: %v", err)
	}
	assistant1, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           []byte(`{"role":"assistant","content":"first answer"}`),
		ExternalMessageID: "external-assistant-1",
	})
	if err != nil {
		t.Fatalf("persist assistant1: %v", err)
	}
	user2, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "user",
		Content:           []byte(`{"role":"user","content":"second"}`),
		ExternalMessageID: "external-user-2",
	})
	if err != nil {
		t.Fatalf("persist user2: %v", err)
	}
	target, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           []byte(`{"role":"assistant","content":"target answer"}`),
		ExternalMessageID: "external-target",
	})
	if err != nil {
		t.Fatalf("persist target: %v", err)
	}
	user3, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "user",
		Content:           []byte(`{"role":"user","content":"third"}`),
		ExternalMessageID: "external-user-3",
	})
	if err != nil {
		t.Fatalf("persist user3: %v", err)
	}
	if _, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           []byte(`{"role":"assistant","content":"third answer"}`),
		ExternalMessageID: "external-assistant-3",
	}); err != nil {
		t.Fatalf("persist assistant3: %v", err)
	}

	located, err := svc.LocateByExternalIDBySession(ctx, sessionID, "external-target", 1, 1)
	if err != nil {
		t.Fatalf("locate target: %v", err)
	}
	if located.TargetID != target.ID {
		t.Fatalf("target id = %s, want %s", located.TargetID, target.ID)
	}
	if got, want := messageIDs(located.Messages), []string{user2.ID, target.ID, user3.ID}; !equalStringSlices(got, want) {
		t.Fatalf("located message ids = %#v, want %#v; previous turn %s/%s should be outside the 1-message window", got, want, user1.ID, assistant1.ID)
	}
}

func TestSQLiteLocateByExternalIDZeroWindowReturnsOnlyTarget(t *testing.T) {
	ctx := context.Background()
	conn, queries := newMessageHistoryTurnSQLite(t)
	defer func() { _ = conn.Close() }()
	svc := NewService(nil, queries)

	const (
		botID     = "00000000-0000-0000-0000-000000003401"
		sessionID = "00000000-0000-0000-0000-000000003402"
	)
	insertMessageHistoryTurnFixtures(t, conn, botID, sessionID)

	if _, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "user",
		Content:           []byte(`{"role":"user","content":"before"}`),
		ExternalMessageID: "external-before",
	}); err != nil {
		t.Fatalf("persist before user: %v", err)
	}
	target, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "assistant",
		Content:           []byte(`{"role":"assistant","content":"target"}`),
		ExternalMessageID: "external-target-zero-window",
	})
	if err != nil {
		t.Fatalf("persist target: %v", err)
	}
	if _, err := svc.Persist(ctx, PersistInput{
		BotID:             botID,
		SessionID:         sessionID,
		Role:              "user",
		Content:           []byte(`{"role":"user","content":"after"}`),
		ExternalMessageID: "external-after",
	}); err != nil {
		t.Fatalf("persist after user: %v", err)
	}

	located, err := svc.LocateByExternalIDBySession(ctx, sessionID, "external-target-zero-window", 0, 0)
	if err != nil {
		t.Fatalf("locate target: %v", err)
	}
	if located.TargetID != target.ID {
		t.Fatalf("target id = %s, want %s", located.TargetID, target.ID)
	}
	if got, want := messageIDs(located.Messages), []string{target.ID}; !equalStringSlices(got, want) {
		t.Fatalf("located message ids = %#v, want %#v", got, want)
	}
}

func newMessageHistoryTurnSQLite(t *testing.T) (*sql.DB, *sqlitestore.Queries) {
	t.Helper()
	conn, err := dbpkg.OpenSQLite(context.Background(), config.SQLiteConfig{DSN: ":memory:"})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	conn.SetMaxOpenConns(1)
	if _, err := conn.ExecContext(context.Background(), sqliteHistoryTurnTestSchema); err != nil {
		_ = conn.Close()
		t.Fatalf("create schema: %v", err)
	}
	store, err := sqlitestore.New(conn)
	if err != nil {
		_ = conn.Close()
		t.Fatalf("new sqlite store: %v", err)
	}
	return conn, sqlitestore.NewQueries(store)
}

func insertMessageHistoryTurnFixtures(t *testing.T, conn *sql.DB, botID, sessionID string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), `INSERT INTO bots (id) VALUES (?)`, botID); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := conn.ExecContext(context.Background(), `INSERT INTO bot_sessions (id, bot_id, channel_type) VALUES (?, ?, ?)`, sessionID, botID, "local"); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func messageIDs(messages []Message) []string {
	ids := make([]string, 0, len(messages))
	for _, msg := range messages {
		ids = append(ids, msg.ID)
	}
	return ids
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

const sqliteHistoryTurnTestSchema = `
CREATE TABLE bots (
  id TEXT PRIMARY KEY
);
CREATE TABLE channel_identities (
  id TEXT PRIMARY KEY,
  display_name TEXT,
  avatar_url TEXT
);
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  channel_type TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  next_turn_position INTEGER NOT NULL DEFAULT 1,
  parent_session_id TEXT
);
CREATE TABLE bot_history_messages (
  id TEXT PRIMARY KEY DEFAULT (
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6)))
  ),
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
  compact_id TEXT,
  session_mode TEXT NOT NULL DEFAULT 'chat',
  runtime_type TEXT NOT NULL DEFAULT 'model',
  model_id TEXT,
  event_id TEXT,
  display_text TEXT,
	  turn_id TEXT,
	  turn_position INTEGER,
	  turn_message_seq INTEGER,
	  turn_visible INTEGER NOT NULL DEFAULT 0,
	  turn_superseded_by_turn_id TEXT,
	  turn_superseded_at TEXT,
	  turn_superseded_reason TEXT,
	  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
CREATE TABLE bot_history_message_assets (
  message_id TEXT NOT NULL,
  role TEXT NOT NULL,
  ordinal INTEGER NOT NULL,
  content_hash TEXT NOT NULL,
  name TEXT,
  metadata TEXT NOT NULL DEFAULT '{}'
);
CREATE UNIQUE INDEX idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;
CREATE VIEW bot_history_turns AS
SELECT
  m.turn_id AS id,
  (
    SELECT first_message.bot_id
    FROM bot_history_messages first_message
    WHERE first_message.turn_id = m.turn_id
      AND first_message.session_id = m.session_id
    ORDER BY first_message.turn_message_seq ASC, first_message.created_at ASC, first_message.id ASC
    LIMIT 1
  ) AS bot_id,
  m.session_id,
  m.turn_position AS position,
  COALESCE((
    SELECT request_message.id
    FROM bot_history_messages request_message
    WHERE request_message.turn_id = m.turn_id
      AND request_message.session_id = m.session_id
      AND request_message.role = 'user'
      AND request_message.turn_message_seq = 1
    ORDER BY request_message.created_at ASC, request_message.id ASC
    LIMIT 1
  ), '') AS request_message_id,
  COALESCE((
    SELECT assistant_message.id
    FROM bot_history_messages assistant_message
    WHERE assistant_message.turn_id = m.turn_id
      AND assistant_message.session_id = m.session_id
      AND assistant_message.role = 'assistant'
      AND assistant_message.turn_message_seq = 2
    ORDER BY assistant_message.created_at ASC, assistant_message.id ASC
    LIMIT 1
  ), '') AS assistant_message_id,
  (
    SELECT superseded_message.turn_superseded_by_turn_id
    FROM bot_history_messages superseded_message
    WHERE superseded_message.turn_id = m.turn_id
      AND superseded_message.session_id = m.session_id
      AND superseded_message.turn_superseded_by_turn_id IS NOT NULL
    ORDER BY superseded_message.turn_superseded_at DESC, superseded_message.created_at DESC, superseded_message.id DESC
    LIMIT 1
  ) AS superseded_by_turn_id,
  MAX(m.turn_superseded_at) AS superseded_at,
  (
    SELECT superseded_message.turn_superseded_reason
    FROM bot_history_messages superseded_message
    WHERE superseded_message.turn_id = m.turn_id
      AND superseded_message.session_id = m.session_id
      AND superseded_message.turn_superseded_reason IS NOT NULL
    ORDER BY superseded_message.turn_superseded_at DESC, superseded_message.created_at DESC, superseded_message.id DESC
    LIMIT 1
  ) AS superseded_reason,
  MIN(m.created_at) AS created_at,
  COALESCE(MAX(m.turn_superseded_at), MAX(m.created_at)) AS updated_at
FROM bot_history_messages m
WHERE m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.session_id IS NOT NULL
GROUP BY m.turn_id, m.session_id, m.turn_position;
CREATE VIEW bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.*
FROM bot_history_messages m
WHERE m.turn_visible = 1;

`
