package message

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

func TestPostgresTurnResponseCursorRejectsClaimExpiredWhileWaitingForSessionLock(t *testing.T) {
	ctx := context.Background()
	pool := openRuntimeFencePostgresPool(t, ctx)
	botID, sessionID := createRuntimeFenceFixtures(t, ctx, pool)
	eventID := uuid.NewString()
	claimToken := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO bot_session_events (
			id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms
		)
		VALUES ($1, $2, $3, 'message', '{}'::jsonb, $4, 1)
	`, eventID, botID, sessionID, "cursor-expiry-claim-"+eventID); err != nil {
		t.Fatalf("insert cursor-expiry event: %v", err)
	}

	queries := dbsqlc.New(pool)
	seedService := NewService(nil, postgresstore.NewQueriesWithPool(pool, queries))
	request, err := seedService.Persist(ctx, PersistInput{
		BotID:     botID.String(),
		SessionID: sessionID.String(),
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"hello"}`),
		EventID:   eventID,
	})
	if err != nil {
		t.Fatalf("persist cursor-expiry request: %v", err)
	}
	if _, err := queries.ClaimSessionEventDelivery(ctx, dbsqlc.ClaimSessionEventDeliveryParams{
		EventID:    mustTestUUID(t, eventID),
		ClaimToken: mustTestUUID(t, claimToken),
		LeaseMs:    1500,
	}); err != nil {
		t.Fatalf("claim cursor-expiry event: %v", err)
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin cursor-expiry blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx, `SELECT id FROM bot_sessions WHERE id = $1 FOR UPDATE`, sessionID); err != nil {
		t.Fatalf("lock cursor-expiry session: %v", err)
	}

	persistPool := openNamedTurnResponseCursorTestPool(t, ctx, "cursor-expiry-"+uuid.NewString())
	persistService := NewService(nil, postgresstore.NewQueriesWithPool(persistPool, dbsqlc.New(persistPool)))
	resultCh := make(chan claimPersistResult, 1)
	go func() {
		_, persistErr := persistService.PersistTurnResponseWithCursor(ctx, []PersistInput{{
			BotID:                botID.String(),
			SessionID:            sessionID.String(),
			Role:                 "assistant",
			Content:              []byte(`{"role":"assistant","content":"stale"}`),
			TurnRequestMessageID: request.ID,
		}}, DiscussCursorUpdate{
			SessionID:           sessionID.String(),
			ScopeKey:            "cursor-expiry",
			Source:              "telegram",
			ConsumedCursor:      1,
			ConsumedEventCursor: 1,
			DeliveryClaims:      []DeliveryClaim{{EventID: eventID, ClaimToken: claimToken}},
		})
		resultCh <- claimPersistResult{handled: true, err: persistErr}
	}()
	waitForPostgresLockWait(t, ctx, pool, persistPool.Config().ConnConfig.RuntimeParams["application_name"], resultCh)

	for {
		var expired bool
		if err := pool.QueryRow(ctx, `
			SELECT delivery_claimed_until <= clock_timestamp()
			FROM bot_session_events
			WHERE id = $1
		`, eventID).Scan(&expired); err != nil {
			t.Fatalf("read cursor-expiry claim: %v", err)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release cursor-expiry blocker: %v", err)
	}

	select {
	case result := <-resultCh:
		if result.err == nil || !strings.Contains(result.err.Error(), "no durable completion evidence") {
			t.Fatalf("expired cursor claim persistence error = %v, want no durable completion evidence", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cursor claim persistence remained blocked after releasing the session")
	}

	var responseCount, cursorCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM bot_history_messages
		WHERE turn_id = (SELECT turn_id FROM bot_history_messages WHERE id = $1)
		  AND role IN ('assistant', 'tool')
	`, request.ID).Scan(&responseCount); err != nil {
		t.Fatalf("count cursor-expiry responses: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM bot_session_discuss_cursors
		WHERE session_id = $1 AND scope_key = 'cursor-expiry'
	`, sessionID).Scan(&cursorCount); err != nil {
		t.Fatalf("count cursor-expiry cursor rows: %v", err)
	}
	if responseCount != 0 || cursorCount != 0 {
		t.Fatalf("expired cursor claim committed response/cursor = %d/%d, want 0/0", responseCount, cursorCount)
	}

	var incomplete, tokenUnchanged bool
	if err := pool.QueryRow(ctx, `
		SELECT delivery_completed_at IS NULL, delivery_claim_token = $2
		FROM bot_session_events
		WHERE id = $1
	`, eventID, claimToken).Scan(&incomplete, &tokenUnchanged); err != nil {
		t.Fatalf("read cursor-expiry delivery state: %v", err)
	}
	if !incomplete || !tokenUnchanged {
		t.Fatalf("expired cursor claim state = incomplete:%t token_unchanged:%t, want true/true", incomplete, tokenUnchanged)
	}
}
