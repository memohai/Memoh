package message

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestPostgresAssetLinkAndHistoryReplacementShareLockOrder(t *testing.T) {
	fixture := setupCommittedCompactionFixture(t)
	ctx := context.Background()
	svc := NewService(nil, postgresstore.NewQueries(dbsqlc.New(fixture.pool)))
	replacement, err := svc.Persist(ctx, PersistInput{
		BotID:           fixture.botID,
		SessionID:       fixture.sessionID,
		Role:            "assistant",
		Content:         []byte(`{"role":"assistant","content":"replacement"}`),
		SkipHistoryTurn: true,
	})
	if err != nil {
		t.Fatalf("persist replacement assistant: %v", err)
	}

	assetTx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin asset transaction: %v", err)
	}
	defer func() { _ = assetTx.Rollback(ctx) }()
	if _, err := assetTx.Exec(ctx, `
		SELECT id FROM bot_sessions WHERE id = $1 FOR UPDATE
	`, fixture.sessionID); err != nil {
		t.Fatalf("lock asset owner session: %v", err)
	}

	replaceTx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin replacement: %v", err)
	}
	defer func() { _ = replaceTx.Rollback(ctx) }()
	replacePID := postgresBackendPID(t, replaceTx)
	replaced := make(chan error, 1)
	go func() {
		_, err := dbsqlc.New(replaceTx).ReplaceHistoryTurn(ctx, dbsqlc.ReplaceHistoryTurnParams{
			OldTurnID:          mustTestUUID(t, fixture.turn.ID),
			SessionID:          mustTestUUID(t, fixture.sessionID),
			RequestMessageID:   mustTestUUID(t, fixture.user.ID),
			AssistantMessageID: mustTestUUID(t, replacement.ID),
			SupersededAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			SupersededReason:   pgtype.Text{String: "retry", Valid: true},
		})
		replaced <- err
	}()
	waitForPostgresLock(t, fixture.pool, replacePID)

	if _, err := dbsqlc.New(assetTx).CreateMessageAsset(ctx, dbsqlc.CreateMessageAssetParams{
		MessageID:   mustTestUUID(t, fixture.assistant.ID),
		Role:        "attachment",
		Ordinal:     0,
		ContentHash: "sha256:lock-order",
		Name:        "lock-order.png",
		Metadata:    []byte("{}"),
	}); err != nil {
		t.Fatalf("link asset while replacement waits for session: %v", err)
	}
	if err := assetTx.Commit(ctx); err != nil {
		t.Fatalf("commit asset link: %v", err)
	}
	select {
	case err := <-replaced:
		if err != nil {
			t.Fatalf("replace after asset commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("replacement did not resume after asset commit")
	}
}

func TestPostgresAssetLinkAndHistoryMutationsShareLockOrder(t *testing.T) {
	t.Run("supersede", func(t *testing.T) {
		fixture := setupCommittedCompactionFixture(t)
		params := dbsqlc.SupersedeHistoryTurnParams{
			SupersededByTurnID: mustTestUUID(t, uuid.NewString()),
			SupersededAt:       pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			SupersededReason:   pgtype.Text{String: "retry", Valid: true},
			OldTurnID:          mustTestUUID(t, fixture.turn.ID),
			SessionID:          mustTestUUID(t, fixture.sessionID),
		}
		assertAssetMutationLockOrder(t, fixture, func(ctx context.Context, queries *dbsqlc.Queries) error {
			_, err := queries.SupersedeHistoryTurn(ctx, params)
			return err
		})
	})
	t.Run("hide", func(t *testing.T) {
		fixture := setupCommittedCompactionFixture(t)
		turnID := mustTestUUID(t, fixture.turn.ID)
		assertAssetMutationLockOrder(t, fixture, func(ctx context.Context, queries *dbsqlc.Queries) error {
			return queries.HideMessagesByHistoryTurn(ctx, turnID)
		})
	})
	t.Run("delete", func(t *testing.T) {
		fixture := setupCommittedCompactionFixture(t)
		messageIDs := []pgtype.UUID{mustTestUUID(t, fixture.assistant.ID)}
		assertAssetMutationLockOrder(t, fixture, func(ctx context.Context, queries *dbsqlc.Queries) error {
			return queries.DeleteMessagesByIDs(ctx, messageIDs)
		})
	})
}

func assertAssetMutationLockOrder(
	t *testing.T,
	fixture committedCompactionFixture,
	mutate func(context.Context, *dbsqlc.Queries) error,
) {
	t.Helper()
	ctx := context.Background()
	assetTx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin asset transaction: %v", err)
	}
	defer func() { _ = assetTx.Rollback(ctx) }()
	if _, err := assetTx.Exec(ctx, `
		SELECT id FROM bot_sessions WHERE id = $1 FOR UPDATE
	`, fixture.sessionID); err != nil {
		t.Fatalf("lock asset owner session: %v", err)
	}

	mutationTx, err := fixture.pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin history mutation: %v", err)
	}
	defer func() { _ = mutationTx.Rollback(ctx) }()
	mutationPID := postgresBackendPID(t, mutationTx)
	mutated := make(chan error, 1)
	go func() {
		mutated <- mutate(ctx, dbsqlc.New(mutationTx))
	}()
	waitForPostgresLock(t, fixture.pool, mutationPID)
	if _, err := dbsqlc.New(assetTx).CreateMessageAsset(ctx, dbsqlc.CreateMessageAssetParams{
		MessageID:   mustTestUUID(t, fixture.assistant.ID),
		Role:        "attachment",
		Ordinal:     0,
		ContentHash: "sha256:lock-order",
		Name:        "lock-order.png",
		Metadata:    []byte("{}"),
	}); err != nil {
		t.Fatalf("link asset while history mutation waits for session: %v", err)
	}
	if err := assetTx.Commit(ctx); err != nil {
		t.Fatalf("commit asset link: %v", err)
	}
	select {
	case err := <-mutated:
		if err != nil {
			t.Fatalf("history mutation after asset commit: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("history mutation did not resume after asset commit")
	}
	if err := mutationTx.Commit(ctx); err != nil {
		t.Fatalf("commit history mutation: %v", err)
	}
}
