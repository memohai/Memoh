//go:build ignore
// +build ignore

package identities_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/db/sqlc"
)

func setupIntegrationTest(t *testing.T) (*identities.Service, *sqlc.Queries, func()) {
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
	svc := identities.NewService(logger, queries)

	return svc, queries, func() { pool.Close() }
}

func TestIntegrationResolveByChannelIdentityStability(t *testing.T) {
	svc, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	key := fmt.Sprintf("ext_%d", time.Now().UnixNano())

	first, err := svc.ResolveByChannelIdentity(ctx, "feishu", key, "first", nil)
	if err != nil {
		t.Fatalf("first resolve failed: %v", err)
	}
	second, err := svc.ResolveByChannelIdentity(ctx, "feishu", key, "second", nil)
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected stable channelIdentity id, got %s and %s", first.ID, second.ID)
	}
}

func TestIntegrationLinkChannelIdentityToUser(t *testing.T) {
	svc, queries, cleanup := setupIntegrationTest(t)
	defer cleanup()

	ctx := context.Background()
	key := fmt.Sprintf("bind_%d", time.Now().UnixNano())
	channelIdentity, err := svc.ResolveByChannelIdentity(ctx, "telegram", key, "tg-user", nil)
	if err != nil {
		t.Fatalf("resolve channelIdentity failed: %v", err)
	}

	user, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	userID := uuid.UUID(user.ID.Bytes).String()

	if err := svc.LinkChannelIdentityToUser(ctx, channelIdentity.ID, userID); err != nil {
		t.Fatalf("link channelIdentity to user failed: %v", err)
	}
	linkedUserID, err := svc.GetLinkedUserID(ctx, channelIdentity.ID)
	if err != nil {
		t.Fatalf("get linked user failed: %v", err)
	}
	if linkedUserID != userID {
		t.Fatalf("expected linked user=%s, got %s", userID, linkedUserID)
	}
}
