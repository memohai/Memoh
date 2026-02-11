package chat_test

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

	"github.com/memohai/memoh/internal/channelidentities"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/db/sqlc"
)

type chatPresenceFixture struct {
	chatSvc            *chat.Service
	channelIdentitySvc *channelidentities.Service
	queries            *sqlc.Queries
	cleanup            func()
}

func setupChatPresenceIntegrationTest(t *testing.T) chatPresenceFixture {
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

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	queries := sqlc.New(pool)

	return chatPresenceFixture{
		chatSvc:            chat.NewService(logger, queries),
		channelIdentitySvc: channelidentities.NewService(logger, queries),
		queries:            queries,
		cleanup:            func() { pool.Close() },
	}
}

func createUserForChatPresence(ctx context.Context, queries *sqlc.Queries) (string, error) {
	row, err := queries.CreateUser(ctx, sqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte("{}"),
	})
	if err != nil {
		return "", err
	}
	return db.UUIDToString(row.ID), nil
}

func createBotForChatPresence(ctx context.Context, queries *sqlc.Queries, ownerUserID string) (string, error) {
	pgOwnerID, err := db.ParseUUID(ownerUserID)
	if err != nil {
		return "", err
	}
	meta, err := json.Marshal(map[string]any{"source": "chat-presence-integration-test"})
	if err != nil {
		return "", err
	}
	row, err := queries.CreateBot(ctx, sqlc.CreateBotParams{
		OwnerUserID: pgOwnerID,
		Type:        "personal",
		DisplayName: pgtype.Text{String: "presence-test-bot", Valid: true},
		IsActive:    true,
		Metadata:    meta,
	})
	if err != nil {
		return "", err
	}
	return db.UUIDToString(row.ID), nil
}

func setupObservedChatScenario(t *testing.T) (chatPresenceFixture, string, string, string, string) {
	t.Helper()

	fixture := setupChatPresenceIntegrationTest(t)
	ctx := context.Background()

	ownerUserID, err := createUserForChatPresence(ctx, fixture.queries)
	if err != nil {
		fixture.cleanup()
		t.Fatalf("create owner user failed: %v", err)
	}
	observerUserID, err := createUserForChatPresence(ctx, fixture.queries)
	if err != nil {
		fixture.cleanup()
		t.Fatalf("create observer user failed: %v", err)
	}
	botID, err := createBotForChatPresence(ctx, fixture.queries, ownerUserID)
	if err != nil {
		fixture.cleanup()
		t.Fatalf("create bot failed: %v", err)
	}

	createdChat, err := fixture.chatSvc.Create(ctx, botID, ownerUserID, chat.CreateChatRequest{
		Kind:  chat.KindGroup,
		Title: "presence-observed",
	})
	if err != nil {
		fixture.cleanup()
		t.Fatalf("create chat failed: %v", err)
	}

	observedChannelIdentity, err := fixture.channelIdentitySvc.ResolveByChannelIdentity(
		ctx,
		"feishu",
		fmt.Sprintf("presence-channelIdentity-%d", time.Now().UnixNano()),
		"presence-observer",
	)
	if err != nil {
		fixture.cleanup()
		t.Fatalf("resolve channelIdentity failed: %v", err)
	}

	_, err = fixture.chatSvc.PersistMessage(
		ctx,
		createdChat.ID,
		botID,
		"",
		observedChannelIdentity.ID,
		"",
		"feishu",
		fmt.Sprintf("ext-msg-%d", time.Now().UnixNano()),
		"user",
		[]byte(`{"content":"hello from observed channelIdentity"}`),
		nil,
	)
	if err != nil {
		fixture.cleanup()
		t.Fatalf("persist message failed: %v", err)
	}

	return fixture, botID, createdChat.ID, observerUserID, observedChannelIdentity.ID
}

func TestObservedChatVisibleAfterBindWithoutBackfill(t *testing.T) {
	fixture, botID, chatID, observerUserID, observedChannelIdentityID := setupObservedChatScenario(t)
	defer fixture.cleanup()

	ctx := context.Background()
	beforeBind, err := fixture.chatSvc.ListByBotAndChannelIdentity(ctx, botID, observerUserID)
	if err != nil {
		t.Fatalf("list chats before bind failed: %v", err)
	}
	if len(beforeBind) != 0 {
		t.Fatalf("expected no visible chats before bind, got %d", len(beforeBind))
	}

	if err := fixture.channelIdentitySvc.LinkChannelIdentityToUser(ctx, observedChannelIdentityID, observerUserID); err != nil {
		t.Fatalf("link channelIdentity to user failed: %v", err)
	}

	afterBind, err := fixture.chatSvc.ListByBotAndChannelIdentity(ctx, botID, observerUserID)
	if err != nil {
		t.Fatalf("list chats after bind failed: %v", err)
	}
	if len(afterBind) == 0 {
		t.Fatalf("expected observed chat visible after bind, got %d chats", len(afterBind))
	}

	var target *chat.ChatListItem
	for i := range afterBind {
		if afterBind[i].ID == chatID {
			target = &afterBind[i]
			break
		}
	}
	if target == nil {
		t.Fatalf("expected chat %s in visible list after bind", chatID)
	}
	if target.AccessMode != chat.AccessModeChannelIdentityObserved {
		t.Fatalf("expected access_mode=%s, got %s", chat.AccessModeChannelIdentityObserved, target.AccessMode)
	}
	if target.ParticipantRole != "" {
		t.Fatalf("expected empty participant_role for observed chat, got %s", target.ParticipantRole)
	}
	if target.LastObservedAt == nil {
		t.Fatal("expected last_observed_at to be set for observed chat")
	}
}

func TestObservedAccessReadableButNotParticipant(t *testing.T) {
	fixture, botID, chatID, observerUserID, observedChannelIdentityID := setupObservedChatScenario(t)
	defer fixture.cleanup()

	ctx := context.Background()
	if err := fixture.channelIdentitySvc.LinkChannelIdentityToUser(ctx, observedChannelIdentityID, observerUserID); err != nil {
		t.Fatalf("link channelIdentity to user failed: %v", err)
	}

	access, err := fixture.chatSvc.GetReadAccess(ctx, chatID, observerUserID)
	if err != nil {
		t.Fatalf("get read access failed: %v", err)
	}
	if access.AccessMode != chat.AccessModeChannelIdentityObserved {
		t.Fatalf("expected read access %s, got %s", chat.AccessModeChannelIdentityObserved, access.AccessMode)
	}

	messages, err := fixture.chatSvc.ListMessages(ctx, chatID)
	if err != nil {
		t.Fatalf("list messages failed: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("expected observed user can read chat messages")
	}

	_, err = fixture.chatSvc.GetParticipant(ctx, chatID, observerUserID)
	if !errors.Is(err, chat.ErrNotParticipant) {
		t.Fatalf("expected ErrNotParticipant for observed user, got %v", err)
	}
	ok, err := fixture.chatSvc.IsParticipant(ctx, chatID, observerUserID)
	if err != nil {
		t.Fatalf("check participant failed: %v", err)
	}
	if ok {
		t.Fatal("expected observed user to remain non-participant")
	}

	visibleChats, err := fixture.chatSvc.ListByBotAndChannelIdentity(ctx, botID, observerUserID)
	if err != nil {
		t.Fatalf("list visible chats failed: %v", err)
	}
	if len(visibleChats) == 0 || visibleChats[0].AccessMode != chat.AccessModeChannelIdentityObserved {
		t.Fatal("expected observed list entry with channel_identity_observed access mode")
	}
}
