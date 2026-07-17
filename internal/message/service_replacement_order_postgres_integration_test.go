package message

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
)

func TestPostgresReplaceTurnRetryPreservesExplicitResponseOrder(t *testing.T) {
	ctx := context.Background()
	tx, svc := newPostgresReplacementOrderService(t, ctx)
	user := persistPostgresReplacementMessage(t, ctx, svc, "user", "hello", false)
	oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old", false)
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	firstAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "tool call", true)
	tool := persistPostgresReplacementMessage(t, ctx, svc, "tool", "result", true)
	finalAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "new", true)
	orphan := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "unrelated", true)
	if _, err := tx.Exec(ctx, `
		UPDATE bot_history_messages
		SET created_at = created_at + interval '1 second'
		WHERE id = $1
	`, orphan.ID); err != nil {
		t.Fatalf("move orphan after replacement batch: %v", err)
	}

	replacement, err := svc.ReplaceTurnOrdered(
		ctx,
		postgresMessageTestSessionID,
		oldTurn.ID,
		user.ID,
		firstAssistant.ID,
		[]string{user.ID, firstAssistant.ID, tool.ID, finalAssistant.ID},
		"retry",
	)
	if err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	assertPostgresVisibleMessageIDs(t, ctx, svc, user.ID, firstAssistant.ID, tool.ID, finalAssistant.ID)
	assertPostgresReplacementTurnOrder(t, ctx, tx, replacement.ID,
		postgresReplacementTurnMessage{ID: user.ID, Role: "user", Sequence: 1},
		postgresReplacementTurnMessage{ID: firstAssistant.ID, Role: "assistant", Sequence: 2},
		postgresReplacementTurnMessage{ID: tool.ID, Role: "tool", Sequence: 3},
		postgresReplacementTurnMessage{ID: finalAssistant.ID, Role: "assistant", Sequence: 4},
	)
	assertPostgresMessageVisibility(t, ctx, tx, oldAssistant.ID, false, true)
	assertPostgresReplacementMessageUnlinked(t, ctx, tx, orphan.ID)
}

func TestPostgresReplaceTurnEditPreservesMidTurnUserOrder(t *testing.T) {
	ctx := context.Background()
	tx, svc := newPostgresReplacementOrderService(t, ctx)
	oldUser := persistPostgresReplacementMessage(t, ctx, svc, "user", "old prompt", false)
	oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old answer", false)
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	newUser := persistPostgresReplacementMessage(t, ctx, svc, "user", "new prompt", true)
	newAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "before decoration", true)
	decoration := persistPostgresReplacementMessage(t, ctx, svc, "user", "injected decoration", true)
	finalAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "new answer", true)

	replacement, err := svc.ReplaceTurnOrdered(
		ctx,
		postgresMessageTestSessionID,
		oldTurn.ID,
		newUser.ID,
		newAssistant.ID,
		[]string{newUser.ID, newAssistant.ID, decoration.ID, finalAssistant.ID},
		"edit",
	)
	if err != nil {
		t.Fatalf("replace turn: %v", err)
	}

	assertPostgresVisibleMessageIDs(t, ctx, svc, newUser.ID, newAssistant.ID, decoration.ID, finalAssistant.ID)
	assertPostgresReplacementTurnOrder(t, ctx, tx, replacement.ID,
		postgresReplacementTurnMessage{ID: newUser.ID, Role: "user", Sequence: 1},
		postgresReplacementTurnMessage{ID: newAssistant.ID, Role: "assistant", Sequence: 2},
		postgresReplacementTurnMessage{ID: decoration.ID, Role: "user", Sequence: 3},
		postgresReplacementTurnMessage{ID: finalAssistant.ID, Role: "assistant", Sequence: 4},
	)
	assertPostgresMessageVisibility(t, ctx, tx, oldUser.ID, false, true)
	assertPostgresMessageVisibility(t, ctx, tx, oldAssistant.ID, false, true)
}

func TestPostgresPersistReplacementRoundRollsBackMessagesWhenReplaceFails(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)
	queries := &postgresAtomicReplacementQueries{
		Queries: postgresstore.NewQueries(dbsqlc.New(tx)),
		tx:      tx,
	}
	publisher := &recordingPublisher{}
	svc := NewService(nil, queries, publisher)
	user := persistPostgresReplacementMessage(t, ctx, svc, "user", "hello", false)
	oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old", false)
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	var beforeCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM bot_history_messages WHERE session_id = $1`, postgresMessageTestSessionID).Scan(&beforeCount); err != nil {
		t.Fatalf("count messages before replacement: %v", err)
	}
	publisher.events = nil

	_, err = svc.PersistReplacementRound(ctx, ReplacementRoundRequest{
		Messages: []PersistInput{
			{
				BotID:           postgresMessageTestBotID,
				SessionID:       postgresMessageTestSessionID,
				Role:            "assistant",
				Content:         []byte(`{"role":"assistant","content":"replacement"}`),
				SessionMode:     "chat",
				RuntimeType:     "model",
				SkipHistoryTurn: true,
			},
		},
		OldTurnID:                "00000000-0000-0000-0000-000000074199",
		ExistingRequestMessageID: user.ID,
		Reason:                   "retry",
	})
	if err == nil {
		t.Fatal("PersistReplacementRound() error = nil, want replace failure")
	}
	var afterCount int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM bot_history_messages WHERE session_id = $1`, postgresMessageTestSessionID).Scan(&afterCount); err != nil {
		t.Fatalf("count messages after replacement: %v", err)
	}
	if afterCount != beforeCount {
		t.Fatalf("message count after failed replacement = %d, want %d", afterCount, beforeCount)
	}
	latest, latestErr := svc.GetLatestVisibleTurnBySession(ctx, postgresMessageTestSessionID)
	if latestErr != nil {
		t.Fatalf("get latest visible turn after replacement failure: %v", latestErr)
	}
	if latest.ID != oldTurn.ID {
		t.Fatalf("latest visible turn = %q, want old turn %q", latest.ID, oldTurn.ID)
	}
	assertPostgresVisibleMessageIDs(t, ctx, svc, user.ID, oldAssistant.ID)
	if len(publisher.events) != 0 {
		t.Fatalf("published events after failed replacement = %d, want 0", len(publisher.events))
	}
}

func TestPostgresReplaceTurnOrderedRejectsInvalidSequence(t *testing.T) {
	tests := []struct {
		name     string
		sequence func(userID, oldAssistantID, replacementID string) []string
	}{
		{
			name: "missing sequence",
			sequence: func(_, _, _ string) []string {
				return nil
			},
		},
		{
			name: "duplicate message",
			sequence: func(userID, _, replacementID string) []string {
				return []string{userID, replacementID, replacementID}
			},
		},
		{
			name: "missing request anchor",
			sequence: func(_, _, replacementID string) []string {
				return []string{replacementID}
			},
		},
		{
			name: "missing assistant anchor",
			sequence: func(userID, _, _ string) []string {
				return []string{userID}
			},
		},
		{
			name: "message already linked outside allowed request anchor",
			sequence: func(userID, oldAssistantID, replacementID string) []string {
				return []string{userID, replacementID, oldAssistantID}
			},
		},
		{
			name: "request anchor is not first",
			sequence: func(userID, _, replacementID string) []string {
				return []string{replacementID, userID}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx, svc := newPostgresReplacementOrderService(t, ctx)
			user := persistPostgresReplacementMessage(t, ctx, svc, "user", "hello", false)
			oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old", false)
			oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
			if err != nil {
				t.Fatalf("get old turn: %v", err)
			}
			replacement := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "new", true)

			_, err = svc.ReplaceTurnOrdered(
				ctx,
				postgresMessageTestSessionID,
				oldTurn.ID,
				user.ID,
				replacement.ID,
				tt.sequence(user.ID, oldAssistant.ID, replacement.ID),
				"retry",
			)
			if err == nil {
				t.Fatal("ReplaceTurnOrdered() error = nil, want invalid sequence error")
			}
			assertPostgresVisibleMessageIDs(t, ctx, svc, user.ID, oldAssistant.ID)
			assertPostgresReplacementMessageUnlinked(t, ctx, tx, replacement.ID)
		})
	}
}

func TestPostgresReplaceHistoryTurnSQLRejectsInvalidSequence(t *testing.T) {
	tests := []struct {
		name     string
		sequence func(requestID, oldAssistantID, replacementID pgtype.UUID) []pgtype.UUID
	}{
		{
			name: "missing sequence",
			sequence: func(_, _, _ pgtype.UUID) []pgtype.UUID {
				return nil
			},
		},
		{
			name: "duplicate message",
			sequence: func(requestID, _, replacementID pgtype.UUID) []pgtype.UUID {
				return []pgtype.UUID{requestID, replacementID, replacementID}
			},
		},
		{
			name: "missing request anchor",
			sequence: func(_, _, replacementID pgtype.UUID) []pgtype.UUID {
				return []pgtype.UUID{replacementID}
			},
		},
		{
			name: "missing assistant anchor",
			sequence: func(requestID, _, _ pgtype.UUID) []pgtype.UUID {
				return []pgtype.UUID{requestID}
			},
		},
		{
			name: "request anchor is not first",
			sequence: func(requestID, _, replacementID pgtype.UUID) []pgtype.UUID {
				return []pgtype.UUID{replacementID, requestID}
			},
		},
		{
			name: "message cannot be relinked",
			sequence: func(requestID, oldAssistantID, replacementID pgtype.UUID) []pgtype.UUID {
				return []pgtype.UUID{requestID, replacementID, oldAssistantID}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			tx, svc := newPostgresReplacementOrderService(t, ctx)
			user := persistPostgresReplacementMessage(t, ctx, svc, "user", "hello", false)
			oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old", false)
			oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
			if err != nil {
				t.Fatalf("get old turn: %v", err)
			}
			replacement := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "new", true)
			requestID := mustTestUUID(t, user.ID)
			oldAssistantID := mustTestUUID(t, oldAssistant.ID)
			replacementID := mustTestUUID(t, replacement.ID)

			_, err = dbsqlc.New(tx).ReplaceHistoryTurn(ctx, dbsqlc.ReplaceHistoryTurnParams{
				SessionID:             mustTestUUID(t, postgresMessageTestSessionID),
				OldTurnID:             mustTestUUID(t, oldTurn.ID),
				ReplacementMessageIds: tt.sequence(requestID, oldAssistantID, replacementID),
				RequestMessageID:      requestID,
				AssistantMessageID:    replacementID,
				SupersededAt:          pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
				SupersededReason:      pgtype.Text{String: "retry", Valid: true},
			})
			if err == nil {
				t.Fatal("ReplaceHistoryTurn() error = nil, want invalid sequence error")
			}
			assertPostgresVisibleMessageIDs(t, ctx, svc, user.ID, oldAssistant.ID)
			assertPostgresReplacementMessageUnlinked(t, ctx, tx, replacement.ID)
		})
	}
}

func TestPostgresReplaceHistoryTurnSQLRejectsForeignSessionMessage(t *testing.T) {
	ctx := context.Background()
	tx, svc := newPostgresReplacementOrderService(t, ctx)
	user := persistPostgresReplacementMessage(t, ctx, svc, "user", "hello", false)
	oldAssistant := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "old", false)
	oldTurn, err := svc.GetVisibleTurnByMessage(ctx, postgresMessageTestSessionID, oldAssistant.ID)
	if err != nil {
		t.Fatalf("get old turn: %v", err)
	}
	replacement := persistPostgresReplacementMessage(t, ctx, svc, "assistant", "new", true)
	const foreignSessionID = "00000000-0000-0000-0000-000000074199"
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type)
		VALUES ($1, $2, 'local')
	`, foreignSessionID, postgresMessageTestBotID); err != nil {
		t.Fatalf("insert foreign session: %v", err)
	}
	foreign, err := svc.Persist(ctx, PersistInput{
		BotID:           postgresMessageTestBotID,
		SessionID:       foreignSessionID,
		Role:            "tool",
		Content:         []byte(`{"role":"tool","content":"foreign"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist foreign session message: %v", err)
	}
	requestID := mustTestUUID(t, user.ID)
	replacementID := mustTestUUID(t, replacement.ID)
	_, err = dbsqlc.New(tx).ReplaceHistoryTurn(ctx, dbsqlc.ReplaceHistoryTurnParams{
		SessionID:             mustTestUUID(t, postgresMessageTestSessionID),
		OldTurnID:             mustTestUUID(t, oldTurn.ID),
		ReplacementMessageIds: []pgtype.UUID{requestID, replacementID, mustTestUUID(t, foreign.ID)},
		RequestMessageID:      requestID,
		AssistantMessageID:    replacementID,
		SupersededAt:          pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		SupersededReason:      pgtype.Text{String: "retry", Valid: true},
	})
	if err == nil {
		t.Fatal("ReplaceHistoryTurn() error = nil, want foreign session error")
	}
	assertPostgresVisibleMessageIDs(t, ctx, svc, user.ID, oldAssistant.ID)
	assertPostgresReplacementMessageUnlinked(t, ctx, tx, replacement.ID)
	assertPostgresReplacementMessageUnlinked(t, ctx, tx, foreign.ID)
}

func newPostgresReplacementOrderService(t *testing.T, ctx context.Context) (pgx.Tx, *DBService) {
	t.Helper()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)
	return tx, NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
}

type postgresAtomicReplacementQueries struct {
	*postgresstore.Queries
	tx pgx.Tx
}

func (*postgresAtomicReplacementQueries) SupportsTransactions() bool { return true }

func (q *postgresAtomicReplacementQueries) InTx(ctx context.Context, fn func(dbstore.Queries) error) error {
	tx, err := q.tx.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(postgresstore.NewQueries(dbsqlc.New(tx))); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func persistPostgresReplacementMessage(t *testing.T, ctx context.Context, svc *DBService, role, text string, skipHistoryTurn bool) Message {
	t.Helper()
	message, err := svc.Persist(ctx, PersistInput{
		BotID:           postgresMessageTestBotID,
		SessionID:       postgresMessageTestSessionID,
		Role:            role,
		Content:         []byte(`{"role":"` + role + `","content":"` + text + `"}`),
		SkipHistoryTurn: skipHistoryTurn,
	})
	if err != nil {
		t.Fatalf("persist %s message: %v", role, err)
	}
	return message
}

type postgresReplacementTurnMessage struct {
	ID       string
	Role     string
	Sequence int64
}

func assertPostgresReplacementTurnOrder(t *testing.T, ctx context.Context, tx pgx.Tx, turnID string, want ...postgresReplacementTurnMessage) {
	t.Helper()
	rows, err := tx.Query(ctx, `
		SELECT id::text, role, turn_message_seq
		FROM bot_history_messages
		WHERE turn_id = $1 AND turn_visible = true
		ORDER BY turn_message_seq
	`, turnID)
	if err != nil {
		t.Fatalf("query replacement turn messages: %v", err)
	}
	defer rows.Close()

	got := make([]postgresReplacementTurnMessage, 0, len(want))
	for rows.Next() {
		var message postgresReplacementTurnMessage
		if err := rows.Scan(&message.ID, &message.Role, &message.Sequence); err != nil {
			t.Fatalf("scan replacement turn message: %v", err)
		}
		got = append(got, message)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read replacement turn messages: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("replacement turn messages = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("replacement turn messages = %#v, want %#v", got, want)
		}
	}
}

func assertPostgresReplacementMessageUnlinked(t *testing.T, ctx context.Context, tx pgx.Tx, messageID string) {
	t.Helper()
	var linked bool
	if err := tx.QueryRow(ctx, `
		SELECT turn_id IS NOT NULL
		FROM bot_history_messages
		WHERE id = $1
	`, messageID).Scan(&linked); err != nil {
		t.Fatalf("read orphan turn link: %v", err)
	}
	if linked {
		t.Fatalf("message %s was linked to a replacement turn", messageID)
	}
}
