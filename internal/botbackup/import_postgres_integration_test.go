package botbackup

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestPostgresRestoreHistoryTurnPreservesDecoratedAssistantOrder(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresBotBackupTestTx(t, ctx)
	userID := newBotBackupTestUUID()
	botID := newBotBackupTestUUID()
	sessionID := newBotBackupTestUUID()
	setupPostgresBotBackupTestFixtures(t, ctx, tx, userID, botID, sessionID)

	oldBotID := newBotBackupTestUUID()
	oldSessionID := newBotBackupTestUUID()
	oldTurnID := newBotBackupTestUUID()
	roles := []string{"user", "user", "assistant", "tool", "assistant"}
	oldMessageIDs := make([]pgtype.UUID, len(roles))
	newMessageIDs := make([]pgtype.UUID, len(roles))
	messageMap := make(map[string]pgtype.UUID, len(roles))
	baseTime := time.Now().UTC().Add(-time.Hour)
	archivedOrder := []int{4, 1, 0, 3, 2}
	createdAt := make([]time.Time, len(roles))
	for rank, messageIndex := range archivedOrder {
		createdAt[messageIndex] = baseTime.Add(time.Duration(rank) * time.Second)
	}
	for i, role := range roles {
		oldMessageIDs[i] = newBotBackupTestUUID()
		newMessageIDs[i] = newBotBackupTestUUID()
		messageMap[oldMessageIDs[i].String()] = newMessageIDs[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO bot_history_messages (id, bot_id, session_id, role, content, created_at)
			VALUES ($1, $2, $3, $4, '{}'::jsonb, $5)
		`, newMessageIDs[i], botID, sessionID, role, createdAt[i]); err != nil {
			t.Fatalf("insert restored message %d: %v", i, err)
		}
	}

	archived := make([]dbsqlc.ListAllMessagesForBackupRow, 0, len(archivedOrder))
	for _, i := range archivedOrder {
		archived = append(archived, dbsqlc.ListAllMessagesForBackupRow{
			ID:             oldMessageIDs[i],
			BotID:          oldBotID,
			SessionID:      oldSessionID,
			Role:           roles[i],
			CreatedAt:      pgtype.Timestamptz{Time: createdAt[i], Valid: true},
			TurnID:         oldTurnID,
			TurnPosition:   pgtype.Int8{Int64: 1, Valid: true},
			TurnMessageSeq: pgtype.Int8{Int64: int64(i + 1), Valid: true},
			TurnVisible:    true,
		})
	}
	queries := postgresstore.NewQueries(dbsqlc.New(tx))
	if err := restoreHistoryTurnReadModelFromMessages(
		ctx,
		queries,
		botID,
		archived,
		map[string]pgtype.UUID{oldSessionID.String(): sessionID},
		messageMap,
		nil,
	); err != nil {
		t.Fatalf("restore decorated history turn: %v", err)
	}

	turn, err := queries.GetVisibleHistoryTurnByMessage(ctx, dbsqlc.GetVisibleHistoryTurnByMessageParams{
		SessionID: sessionID,
		MessageID: newMessageIDs[4],
	})
	if err != nil {
		t.Fatalf("get restored history turn: %v", err)
	}
	if turn.RequestMessageID != newMessageIDs[0] || turn.AssistantMessageID != newMessageIDs[2] {
		t.Fatalf(
			"restored turn anchors = %s/%s, want %s/%s",
			turn.RequestMessageID.String(),
			turn.AssistantMessageID.String(),
			newMessageIDs[0].String(),
			newMessageIDs[2].String(),
		)
	}
	visible, err := dbsqlc.New(tx).ListVisibleMessagesFromBySession(ctx, dbsqlc.ListVisibleMessagesFromBySessionParams{
		SessionID: sessionID,
		MessageID: newMessageIDs[0],
	})
	if err != nil {
		t.Fatalf("list restored visible messages: %v", err)
	}
	if len(visible) != len(newMessageIDs) {
		t.Fatalf("restored visible messages = %d, want %d", len(visible), len(newMessageIDs))
	}
	for i, row := range visible {
		if row.ID != newMessageIDs[i] {
			t.Fatalf("restored visible message %d = %s, want %s", i, row.ID.String(), newMessageIDs[i].String())
		}
	}
}

func beginPostgresBotBackupTestTx(t *testing.T, ctx context.Context) pgx.Tx {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip postgres integration test: TEST_POSTGRES_DSN is not set")
	}
	pool, err := dbpkg.OpenPostgresDSN(ctx, dsn)
	if err != nil {
		t.Fatalf("connect to configured postgres integration database: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping configured postgres integration database: %v", err)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	return tx
}

func setupPostgresBotBackupTestFixtures(
	t *testing.T,
	ctx context.Context,
	tx pgx.Tx,
	userID pgtype.UUID,
	botID pgtype.UUID,
	sessionID pgtype.UUID,
) {
	t.Helper()
	name := fmt.Sprintf("postgres-botbackup-test-%s", uuid.NewString())
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (id, username, role, is_active)
		VALUES ($1, $2, 'admin', true)
	`, userID, name); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bots (id, owner_user_id, name)
		VALUES ($1, $2, $3)
	`, botID, userID, name); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type)
		VALUES ($1, $2, 'local')
	`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
}

func newBotBackupTestUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: uuid.New(), Valid: true}
}
