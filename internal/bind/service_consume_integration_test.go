package bind_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/bind"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

func setupBindConsumeIntegrationTest(t *testing.T) (*sqlc.Queries, *identities.Service, *bind.Service, func()) {
	t.Helper()

	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip integration test: TEST_POSTGRES_DSN is not set")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration test: cannot connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skip integration test: database ping failed: %v", err)
	}

	queries := sqlc.New(pool)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	channelIdentitySvc := identities.NewService(logger, queries)
	bindSvc := bind.NewService(logger, pool, queries)
	return queries, channelIdentitySvc, bindSvc, func() { pool.Close() }
}

func createUserForBind(ctx context.Context, queries *sqlc.Queries) (string, error) {
	row, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		return "", err
	}
	return row.ID.String(), nil
}

func createBotForBind(ctx context.Context, queries *sqlc.Queries, ownerUserID string) (string, error) {
	pgOwnerID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return "", err
	}
	meta, err := json.Marshal(map[string]any{"source": "bind-integration-test"})
	if err != nil {
		return "", err
	}
	row, err := queries.CreateBot(ctx, sqlc.CreateBotParams{
		OwnerUserID: pgOwnerID,
		Type:        "personal",
		DisplayName: pgtype.Text{String: "bind-test-bot", Valid: true},
		IsActive:    true,
		Metadata:    meta,
	})
	if err != nil {
		return "", err
	}
	return row.ID.String(), nil
}

func TestBindConsumeLinksChannelIdentityToIssuerUser(t *testing.T) {
	queries, channelIdentitySvc, bindSvc, cleanup := setupBindConsumeIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	ownerUserID, err := createUserForBind(ctx, queries)
	if err != nil {
		t.Fatalf("create owner user failed: %v", err)
	}
	sourceChannelIdentity, err := channelIdentitySvc.ResolveByChannelIdentity(ctx, "feishu", fmt.Sprintf("bind-src-%d", time.Now().UnixNano()), "source", nil)
	if err != nil {
		t.Fatalf("create source channelIdentity failed: %v", err)
	}
	code, err := bindSvc.Issue(ctx, ownerUserID, "feishu", 10*time.Minute)
	if err != nil {
		t.Fatalf("issue bind code failed: %v", err)
	}
	if err := bindSvc.Consume(ctx, code, sourceChannelIdentity.ID); err != nil {
		t.Fatalf("consume bind code failed: %v", err)
	}

	after, err := bindSvc.Get(ctx, code.Token)
	if err != nil {
		t.Fatalf("get bind code failed: %v", err)
	}
	if after.UsedAt.IsZero() {
		t.Fatal("expected code used_at set after consume")
	}
	if after.UsedByChannelIdentityID != sourceChannelIdentity.ID {
		t.Fatalf("expected used_by_channel_identity_id=%s, got %s", sourceChannelIdentity.ID, after.UsedByChannelIdentityID)
	}

	linkedUserID, err := channelIdentitySvc.GetLinkedUserID(ctx, sourceChannelIdentity.ID)
	if err != nil {
		t.Fatalf("get linked user failed: %v", err)
	}
	if linkedUserID != ownerUserID {
		t.Fatalf("expected linked user=%s, got %s", ownerUserID, linkedUserID)
	}
}

func TestBindConsumeConflictDoesNotMarkUsed(t *testing.T) {
	queries, channelIdentitySvc, bindSvc, cleanup := setupBindConsumeIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	issuerUserID, err := createUserForBind(ctx, queries)
	if err != nil {
		t.Fatalf("create issuer user failed: %v", err)
	}
	otherUserID, err := createUserForBind(ctx, queries)
	if err != nil {
		t.Fatalf("create other user failed: %v", err)
	}
	sourceChannelIdentity, err := channelIdentitySvc.ResolveByChannelIdentity(ctx, "feishu", fmt.Sprintf("bind-conflict-%d", time.Now().UnixNano()), "source", nil)
	if err != nil {
		t.Fatalf("create source channelIdentity failed: %v", err)
	}
	if err := channelIdentitySvc.LinkChannelIdentityToUser(ctx, sourceChannelIdentity.ID, otherUserID); err != nil {
		t.Fatalf("pre-link source channelIdentity failed: %v", err)
	}
	code, err := bindSvc.Issue(ctx, issuerUserID, "feishu", 10*time.Minute)
	if err != nil {
		t.Fatalf("issue bind code failed: %v", err)
	}
	if err := bindSvc.Consume(ctx, code, sourceChannelIdentity.ID); !errors.Is(err, bind.ErrLinkConflict) {
		t.Fatalf("expected ErrLinkConflict, got %v", err)
	}

	after, err := bindSvc.Get(ctx, code.Token)
	if err != nil {
		t.Fatalf("get bind code failed: %v", err)
	}
	if !after.UsedAt.IsZero() {
		t.Fatal("expected code to remain unused after conflict")
	}
}
