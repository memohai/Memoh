package session

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	message "github.com/memohai/memoh/internal/message"
)

func TestPostgresForkFromAssistantMessageCopiesVisibleTurns(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresSessionTestTx(t, ctx)
	setupPostgresSessionForkFixtures(t, ctx, tx)

	svc := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)), nil)
	fork, err := svc.ForkFromAssistantMessage(ctx, ForkFromAssistantInput{
		BotID:           postgresSessionTestBotID,
		SessionID:       postgresSessionTestSessionID,
		MessageID:       postgresSessionTestAssistant2ID,
		Title:           "Forked",
		CreatedByUserID: postgresSessionTestUserID,
	})
	if err != nil {
		t.Fatalf("fork assistant message: %v", err)
	}

	forkedFrom, ok := fork.Metadata["forked_from"].(map[string]any)
	if !ok {
		t.Fatalf("fork metadata missing forked_from: %#v", fork.Metadata)
	}
	if got := forkedFrom["session_id"]; got != postgresSessionTestSessionID {
		t.Fatalf("fork source session_id = %#v, want %s", got, postgresSessionTestSessionID)
	}
	if got := forkedFrom["message_id"]; got != postgresSessionTestAssistant2ID {
		t.Fatalf("fork source message_id = %#v, want %s", got, postgresSessionTestAssistant2ID)
	}
	forkMessageID, _ := forkedFrom["fork_message_id"].(string)
	if forkMessageID == "" || forkMessageID == postgresSessionTestAssistant2ID {
		t.Fatalf("fork_message_id = %#v, want copied assistant message id", forkedFrom["fork_message_id"])
	}

	var nextTurnPosition int64
	if err := tx.QueryRow(ctx, `
		SELECT next_turn_position
		FROM bot_sessions
		WHERE id = $1
	`, fork.ID).Scan(&nextTurnPosition); err != nil {
		t.Fatalf("read fork next_turn_position: %v", err)
	}
	if nextTurnPosition != 3 {
		t.Fatalf("next_turn_position = %d, want 3", nextTurnPosition)
	}

	rows, err := tx.Query(ctx, `
		SELECT role, turn_position, turn_message_seq
		FROM bot_history_messages
		WHERE session_id = $1
		ORDER BY turn_position ASC, turn_message_seq ASC
	`, fork.ID)
	if err != nil {
		t.Fatalf("list fork messages: %v", err)
	}
	var got []string
	for rows.Next() {
		var role string
		var position, seq int64
		if err := rows.Scan(&role, &position, &seq); err != nil {
			t.Fatalf("scan fork message: %v", err)
		}
		got = append(got, fmt.Sprintf("%s:%d:%d", role, position, seq))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fork messages: %v", err)
	}
	rows.Close()
	want := []string{"user:1:1", "assistant:1:2", "user:2:1", "assistant:2:2"}
	if len(got) != len(want) {
		t.Fatalf("fork messages = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fork messages = %#v, want %#v", got, want)
		}
	}

	var anchorRole string
	var anchorPosition, anchorSeq int64
	if err := tx.QueryRow(ctx, `
		SELECT role, turn_position, turn_message_seq
		FROM bot_history_messages
		WHERE id = $1 AND session_id = $2
	`, forkMessageID, fork.ID).Scan(&anchorRole, &anchorPosition, &anchorSeq); err != nil {
		t.Fatalf("read fork anchor message: %v", err)
	}
	if anchorRole != "assistant" || anchorPosition != 2 || anchorSeq != 2 {
		t.Fatalf("fork anchor = %s:%d:%d, want assistant:2:2", anchorRole, anchorPosition, anchorSeq)
	}
	var anchorAssetCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM bot_history_message_assets
		WHERE message_id = $1
		  AND role = 'attachment'
		  AND ordinal = 1
		  AND content_hash = 'sha256:postgres-session-test-asset'
		  AND name = 'fixture.txt'
	`, forkMessageID).Scan(&anchorAssetCount); err != nil {
		t.Fatalf("count fork anchor assets: %v", err)
	}
	if anchorAssetCount != 1 {
		t.Fatalf("fork anchor asset count = %d, want 1", anchorAssetCount)
	}

	messageSvc := message.NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
	newUser, err := messageSvc.Persist(ctx, message.PersistInput{
		BotID:     postgresSessionTestBotID,
		SessionID: fork.ID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"third"}`),
	})
	if err != nil {
		t.Fatalf("persist fork follow-up user: %v", err)
	}
	newAssistant, err := messageSvc.Persist(ctx, message.PersistInput{
		BotID:                postgresSessionTestBotID,
		SessionID:            fork.ID,
		Role:                 "assistant",
		TurnRequestMessageID: newUser.ID,
		Content:              []byte(`{"role":"assistant","content":"third reply"}`),
	})
	if err != nil {
		t.Fatalf("persist fork follow-up assistant: %v", err)
	}

	rows, err = tx.Query(ctx, `
		SELECT id, role, turn_position, turn_message_seq
		FROM bot_history_messages
		WHERE id = ANY($1::uuid[])
		ORDER BY turn_message_seq ASC
	`, []string{newUser.ID, newAssistant.ID})
	if err != nil {
		t.Fatalf("list fork follow-up messages: %v", err)
	}
	got = got[:0]
	for rows.Next() {
		var id string
		var role string
		var position, seq int64
		if err := rows.Scan(&id, &role, &position, &seq); err != nil {
			t.Fatalf("scan fork follow-up message: %v", err)
		}
		got = append(got, fmt.Sprintf("%s:%d:%d", role, position, seq))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fork follow-up messages: %v", err)
	}
	rows.Close()
	want = []string{"user:3:1", "assistant:3:2"}
	if len(got) != len(want) {
		t.Fatalf("fork follow-up messages = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("fork follow-up messages = %#v, want %#v", got, want)
		}
	}
}

func TestPostgresForkFromAssistantMessageRejectsInvalidTargetWithoutSideEffects(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresSessionTestTx(t, ctx)
	setupPostgresSessionForkFixtures(t, ctx, tx)

	svc := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)), nil)
	beforeSessions := countPostgresSessionTestRows(t, ctx, tx, "bot_sessions")
	beforeMessages := countPostgresSessionTestRows(t, ctx, tx, "bot_history_messages")

	for _, messageID := range []string{postgresSessionTestUser1ID, postgresSessionTestHiddenAssistantID} {
		_, err := svc.ForkFromAssistantMessage(ctx, ForkFromAssistantInput{
			BotID:     postgresSessionTestBotID,
			SessionID: postgresSessionTestSessionID,
			MessageID: messageID,
			Title:     "Should Not Exist",
		})
		if !errors.Is(err, ErrForkSourceNotReply) {
			t.Fatalf("fork invalid message %s error = %v, want ErrForkSourceNotReply", messageID, err)
		}

		if got := countPostgresSessionTestRows(t, ctx, tx, "bot_sessions"); got != beforeSessions {
			t.Fatalf("bot_sessions count after invalid fork = %d, want %d", got, beforeSessions)
		}
		if got := countPostgresSessionTestRows(t, ctx, tx, "bot_history_messages"); got != beforeMessages {
			t.Fatalf("bot_history_messages count after invalid fork = %d, want %d", got, beforeMessages)
		}
	}
}

func TestPostgresRepairIncompleteForkSessionsMigration(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresSessionTestTx(t, ctx)
	setupPostgresSessionForkFixtures(t, ctx, tx)

	const (
		pureResidualID      = "00000000-0000-0000-0000-000000075301"
		withFollowUpID      = "00000000-0000-0000-0000-000000075302"
		validForkID         = "00000000-0000-0000-0000-000000075303"
		pureResidualMessage = "00000000-0000-0000-0000-000000075311"
		followUpCopyMessage = "00000000-0000-0000-0000-000000075312"
		followUpNewMessage  = "00000000-0000-0000-0000-000000075313"
		validForkMessage    = "00000000-0000-0000-0000-000000075314"
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type, title, metadata, created_at, updated_at)
		VALUES
			(
				$2, $1, 'local', 'bad fork',
				'{"forked_from":{"session_id":"00000000-0000-0000-0000-000000075103","message_id":"00000000-0000-0000-0000-000000075114"}}'::jsonb,
				now(), now()
			),
			(
				$3, $1, 'local', 'bad fork with follow-up',
				'{"forked_from":{"session_id":"00000000-0000-0000-0000-000000075103","message_id":"00000000-0000-0000-0000-000000075114"}}'::jsonb,
				now(), now()
			),
			(
				$4, $1, 'local', 'valid fork',
				'{"forked_from":{"session_id":"00000000-0000-0000-0000-000000075103","message_id":"00000000-0000-0000-0000-000000075114","fork_message_id":"00000000-0000-0000-0000-000000075314"}}'::jsonb,
				now(), now()
			)
	`, postgresSessionTestBotID, pureResidualID, withFollowUpID, validForkID); err != nil {
		t.Fatalf("insert repair fork sessions: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content,
			turn_id, turn_position, turn_message_seq, turn_visible, created_at
		)
		VALUES
			(
				$3, $1, $2, 'assistant', '{"role":"assistant","content":"copied"}'::jsonb,
				'00000000-0000-0000-0000-000000075321', 1, 2, true,
				(SELECT created_at - interval '1 minute' FROM bot_sessions WHERE id = $2)
			),
			(
				$5, $1, $4, 'assistant', '{"role":"assistant","content":"copied"}'::jsonb,
				'00000000-0000-0000-0000-000000075322', 1, 2, true,
				(SELECT created_at - interval '1 minute' FROM bot_sessions WHERE id = $4)
			),
			(
				$6, $1, $4, 'user', '{"role":"user","content":"follow-up"}'::jsonb,
				'00000000-0000-0000-0000-000000075323', 1, 1, true,
				(SELECT created_at + interval '1 minute' FROM bot_sessions WHERE id = $4)
			),
			(
				$8, $1, $7, 'assistant', '{"role":"assistant","content":"valid copied"}'::jsonb,
				'00000000-0000-0000-0000-000000075324', 1, 2, true,
				(SELECT created_at - interval '1 minute' FROM bot_sessions WHERE id = $7)
			)
	`, postgresSessionTestBotID, pureResidualID, pureResidualMessage, withFollowUpID, followUpCopyMessage, followUpNewMessage, validForkID, validForkMessage); err != nil {
		t.Fatalf("insert repair fork messages: %v", err)
	}

	sql, err := os.ReadFile("../../db/postgres/migrations/0105_repair_superseded_message_visibility.up.sql")
	if err != nil {
		t.Fatalf("read fork repair migration: %v", err)
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("run fork repair migration: %v", err)
	}

	assertPostgresSessionDeleted(t, ctx, tx, pureResidualID, true)
	assertPostgresSessionDeleted(t, ctx, tx, withFollowUpID, false)
	assertPostgresSessionDeleted(t, ctx, tx, validForkID, false)
}

const (
	postgresSessionTestUserID            = "00000000-0000-0000-0000-000000075101"
	postgresSessionTestBotID             = "00000000-0000-0000-0000-000000075102"
	postgresSessionTestSessionID         = "00000000-0000-0000-0000-000000075103"
	postgresSessionTestUser1ID           = "00000000-0000-0000-0000-000000075111"
	postgresSessionTestAssistant1ID      = "00000000-0000-0000-0000-000000075112"
	postgresSessionTestUser2ID           = "00000000-0000-0000-0000-000000075113"
	postgresSessionTestAssistant2ID      = "00000000-0000-0000-0000-000000075114"
	postgresSessionTestHiddenAssistantID = "00000000-0000-0000-0000-000000075115"
)

func beginPostgresSessionTestTx(t *testing.T, ctx context.Context) pgx.Tx {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip postgres integration test: TEST_POSTGRES_DSN is not set")
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip postgres integration test: cannot connect to database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("skip postgres integration test: database ping failed: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

func setupPostgresSessionForkFixtures(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()
	name := fmt.Sprintf("postgres-session-test-%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, role, is_active)
		VALUES ($1, $2, 'admin', true)
	`, postgresSessionTestUserID, name); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bots (id, owner_user_id, name)
		VALUES ($1, $2, $3)
	`, postgresSessionTestBotID, postgresSessionTestUserID, name); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type, title, metadata)
		VALUES ($1, $2, 'local', 'Source', '{"source":"fixture"}'::jsonb)
	`, postgresSessionTestSessionID, postgresSessionTestBotID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content,
			turn_id, turn_position, turn_message_seq, turn_visible, turn_superseded_at, created_at
		)
		VALUES
			(
				$3, $1, $2, 'user', '{"role":"user","content":"first"}'::jsonb,
				'00000000-0000-0000-0000-000000075201', 1, 1, true, NULL, now() - interval '4 minutes'
			),
			(
				$4, $1, $2, 'assistant', '{"role":"assistant","content":"first reply"}'::jsonb,
				'00000000-0000-0000-0000-000000075201', 1, 2, true, NULL, now() - interval '3 minutes'
			),
			(
				$5, $1, $2, 'user', '{"role":"user","content":"second"}'::jsonb,
				'00000000-0000-0000-0000-000000075202', 2, 1, true, NULL, now() - interval '2 minutes'
			),
			(
				$6, $1, $2, 'assistant', '{"role":"assistant","content":"second reply"}'::jsonb,
				'00000000-0000-0000-0000-000000075202', 2, 2, true, NULL, now() - interval '1 minutes'
			),
			(
				$7, $1, $2, 'assistant', '{"role":"assistant","content":"hidden"}'::jsonb,
				'00000000-0000-0000-0000-000000075203', 3, 2, false, now(), now()
			)
	`, postgresSessionTestBotID, postgresSessionTestSessionID, postgresSessionTestUser1ID, postgresSessionTestAssistant1ID, postgresSessionTestUser2ID, postgresSessionTestAssistant2ID, postgresSessionTestHiddenAssistantID); err != nil {
		t.Fatalf("insert messages: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_message_assets (message_id, role, ordinal, content_hash, name, metadata)
		VALUES ($1, 'attachment', 1, 'sha256:postgres-session-test-asset', 'fixture.txt', '{"source":"fixture"}'::jsonb)
	`, postgresSessionTestAssistant2ID); err != nil {
		t.Fatalf("insert message asset: %v", err)
	}
}

func countPostgresSessionTestRows(t *testing.T, ctx context.Context, tx pgx.Tx, table string) int {
	t.Helper()
	if table != "bot_sessions" && table != "bot_history_messages" {
		t.Fatalf("unexpected test table %q", table)
	}
	query := fmt.Sprintf("SELECT count(*) FROM %s WHERE bot_id = $1", table)
	var count int
	if err := tx.QueryRow(ctx, query, postgresSessionTestBotID).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func assertPostgresSessionDeleted(t *testing.T, ctx context.Context, tx pgx.Tx, sessionID string, wantDeleted bool) {
	t.Helper()
	var deleted bool
	if err := tx.QueryRow(ctx, `
		SELECT deleted_at IS NOT NULL
		FROM bot_sessions
		WHERE id = $1
	`, sessionID).Scan(&deleted); err != nil {
		t.Fatalf("read session deleted state: %v", err)
	}
	if deleted != wantDeleted {
		t.Fatalf("session %s deleted = %v, want %v", sessionID, deleted, wantDeleted)
	}
}
