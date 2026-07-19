package message

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

type claimPersistResult struct {
	handled bool
	err     error
}

func TestPostgresTurnResponseCursorCompletesIndependentTurnClaimsAtomically(t *testing.T) {
	ctx := context.Background()
	pool := openTurnResponseCursorTestPool(t, ctx)
	userID := uuid.NewString()
	botID := uuid.NewString()
	sessionID := uuid.NewString()
	name := "turn-response-claims-" + uuid.NewString()[:12]
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, role, is_active)
		VALUES ($1, $2, 'admin', true)
	`, userID, name); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, userID) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO bots (id, owner_user_id, name)
		VALUES ($1, $2, $3)
	`, botID, userID, name); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type)
		VALUES ($1, $2, 'local')
	`, sessionID, botID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	eventIDs := []string{uuid.NewString(), uuid.NewString()}
	sort.Strings(eventIDs)
	claimTokens := []string{uuid.NewString(), uuid.NewString()}
	queries := dbsqlc.New(pool)
	for i, eventID := range eventIDs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO bot_session_events (
				id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms
			)
			VALUES ($1, $2, $3, 'message', '{}'::jsonb, $4, $5)
		`, eventID, botID, sessionID, "claim-event-"+eventID, i+1); err != nil {
			t.Fatalf("insert event %d: %v", i, err)
		}
		if _, err := queries.ClaimSessionEventDelivery(ctx, dbsqlc.ClaimSessionEventDeliveryParams{
			EventID:    mustTestUUID(t, eventID),
			ClaimToken: mustTestUUID(t, claimTokens[i]),
			LeaseMs:    10 * 60 * 1000,
		}); err != nil {
			t.Fatalf("claim event %d: %v", i, err)
		}
	}

	seedService := NewService(nil, postgresstore.NewQueries(queries))
	firstRequest, err := seedService.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"first"}`),
		Metadata:  map[string]any{"pipeline_delivery_state": "pending"},
		EventID:   eventIDs[0],
	})
	if err != nil {
		t.Fatalf("persist request: %v", err)
	}
	secondRequest, err := seedService.Persist(ctx, PersistInput{
		BotID:     botID,
		SessionID: sessionID,
		Role:      "user",
		Content:   []byte(`{"role":"user","content":"second"}`),
		Metadata:  map[string]any{"pipeline_delivery_state": "pending"},
		EventID:   eventIDs[1],
	})
	if err != nil {
		t.Fatalf("persist second request: %v", err)
	}
	var sameTurn bool
	if err := pool.QueryRow(ctx, `
		SELECT first.turn_id = second.turn_id
		FROM bot_history_messages first, bot_history_messages second
		WHERE first.id = $1 AND second.id = $2
	`, firstRequest.ID, secondRequest.ID).Scan(&sameTurn); err != nil {
		t.Fatalf("read request turns: %v", err)
	}
	if sameTurn {
		t.Fatal("production-shaped merged requests unexpectedly share a turn")
	}

	service := NewService(nil, postgresstore.NewQueriesWithPool(pool, queries))
	input := PersistInput{
		BotID:                botID,
		SessionID:            sessionID,
		Role:                 "assistant",
		Content:              []byte(`{"role":"assistant","content":"done"}`),
		TurnRequestMessageID: secondRequest.ID,
		SessionMode:          "discuss",
		RuntimeType:          "model",
	}
	update := DiscussCursorUpdate{
		SessionID:           sessionID,
		ScopeKey:            "atomic-claims",
		Source:              "telegram",
		ConsumedCursor:      200,
		ConsumedEventCursor: 20,
		DeliveryClaims: []DeliveryClaim{
			{EventID: eventIDs[0], ClaimToken: claimTokens[0]},
			{EventID: eventIDs[1], ClaimToken: uuid.NewString()},
		},
	}
	if _, err := service.PersistTurnResponseWithCursor(ctx, []PersistInput{input}, update); err == nil {
		t.Fatal("PersistTurnResponseWithCursor() error = nil for stale second claim")
	}
	assertTurnResponseClaimState(
		t, ctx, pool,
		[]string{firstRequest.ID, secondRequest.ID},
		[]int{0, 0},
		sessionID, update.ScopeKey, eventIDs, claimTokens, false,
	)

	update.DeliveryClaims[1].ClaimToken = claimTokens[1]
	if _, err := service.PersistTurnResponseWithCursor(ctx, []PersistInput{input}, update); err != nil {
		t.Fatalf("PersistTurnResponseWithCursor() retry error = %v", err)
	}
	assertTurnResponseClaimState(
		t, ctx, pool,
		[]string{firstRequest.ID, secondRequest.ID},
		[]int{0, 1},
		sessionID, update.ScopeKey, eventIDs, claimTokens, true,
	)
}

func TestPostgresClaimFencedRoundRejectsLeaseThatExpiresWhileWaitingToPersist(t *testing.T) {
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
	`, eventID, botID, sessionID, "expiring-claim-"+eventID); err != nil {
		t.Fatalf("insert expiring claim event: %v", err)
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
		t.Fatalf("persist claim request: %v", err)
	}
	if _, err := queries.ClaimSessionEventDelivery(ctx, dbsqlc.ClaimSessionEventDeliveryParams{
		EventID:    mustTestUUID(t, eventID),
		ClaimToken: mustTestUUID(t, claimToken),
		LeaseMs:    500,
	}); err != nil {
		t.Fatalf("claim event delivery: %v", err)
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin session blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx, `SELECT id FROM bot_sessions WHERE id = $1 FOR UPDATE`, sessionID); err != nil {
		t.Fatalf("lock session row: %v", err)
	}

	persistPool := openNamedTurnResponseCursorTestPool(t, ctx, "claim-session-"+uuid.NewString())
	persistService := NewService(nil, postgresstore.NewQueriesWithPool(persistPool, dbsqlc.New(persistPool)))
	resultCh := make(chan claimPersistResult, 1)
	go func() {
		_, handled, persistErr := persistService.PersistRound(ctx, []PersistInput{{
			BotID:                botID.String(),
			SessionID:            sessionID.String(),
			Role:                 "assistant",
			Content:              []byte(`{"role":"assistant","content":"stale"}`),
			TurnRequestMessageID: request.ID,
		}}, RoundPersistenceOptions{DeliveryClaims: []DeliveryClaim{{EventID: eventID, ClaimToken: claimToken}}})
		resultCh <- claimPersistResult{handled: handled, err: persistErr}
	}()
	waitForPostgresLockWait(t, ctx, pool, persistPool.Config().ConnConfig.RuntimeParams["application_name"], resultCh)
	for {
		var expired bool
		if err := pool.QueryRow(ctx, `
			SELECT delivery_claimed_until <= clock_timestamp()
			FROM bot_session_events
			WHERE id = $1
		`, eventID).Scan(&expired); err != nil {
			t.Fatalf("read session-lock claim expiry: %v", err)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release session blocker: %v", err)
	}
	select {
	case result := <-resultCh:
		if result.err == nil || !result.handled {
			t.Fatalf("expired claim persistence = handled:%t err:%v, want true/error", result.handled, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("claim-fenced persistence remained blocked after releasing the session")
	}
	var responseCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM bot_history_messages
		WHERE turn_id = (SELECT turn_id FROM bot_history_messages WHERE id = $1)
		  AND role IN ('assistant', 'tool')
	`, request.ID).Scan(&responseCount); err != nil {
		t.Fatalf("count rolled-back responses: %v", err)
	}
	if responseCount != 0 {
		t.Fatalf("expired claim committed %d responses, want 0", responseCount)
	}
}

func TestPostgresClaimFencedRoundRejectsLeaseThatExpiresWhileWaitingForEventLock(t *testing.T) {
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
	`, eventID, botID, sessionID, "event-lock-claim-"+eventID); err != nil {
		t.Fatalf("insert event-lock claim event: %v", err)
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
		t.Fatalf("persist event-lock claim request: %v", err)
	}
	if _, err := queries.ClaimSessionEventDelivery(ctx, dbsqlc.ClaimSessionEventDeliveryParams{
		EventID:    mustTestUUID(t, eventID),
		ClaimToken: mustTestUUID(t, claimToken),
		LeaseMs:    1500,
	}); err != nil {
		t.Fatalf("claim event-lock delivery: %v", err)
	}

	blocker, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin event blocker: %v", err)
	}
	defer func() { _ = blocker.Rollback(ctx) }()
	if _, err := blocker.Exec(ctx, `SELECT id FROM bot_session_events WHERE id = $1 FOR UPDATE`, eventID); err != nil {
		t.Fatalf("lock event row: %v", err)
	}

	persistPool := openNamedTurnResponseCursorTestPool(t, ctx, "claim-lock-"+uuid.NewString())
	persistService := NewService(nil, postgresstore.NewQueriesWithPool(persistPool, dbsqlc.New(persistPool)))
	resultCh := make(chan claimPersistResult, 1)
	go func() {
		_, handled, persistErr := persistService.PersistRound(ctx, []PersistInput{{
			BotID:                botID.String(),
			SessionID:            sessionID.String(),
			Role:                 "assistant",
			Content:              []byte(`{"role":"assistant","content":"stale"}`),
			TurnRequestMessageID: request.ID,
		}}, RoundPersistenceOptions{DeliveryClaims: []DeliveryClaim{{EventID: eventID, ClaimToken: claimToken}}})
		resultCh <- claimPersistResult{handled: handled, err: persistErr}
	}()
	waitForPostgresLockWait(t, ctx, pool, persistPool.Config().ConnConfig.RuntimeParams["application_name"], resultCh)
	for {
		var expired bool
		if err := pool.QueryRow(ctx, `
			SELECT delivery_claimed_until <= clock_timestamp()
			FROM bot_session_events
			WHERE id = $1
		`, eventID).Scan(&expired); err != nil {
			t.Fatalf("read event claim expiry: %v", err)
		}
		if expired {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release event blocker: %v", err)
	}
	select {
	case result := <-resultCh:
		if result.err == nil || !result.handled {
			t.Fatalf("event-lock expired claim persistence = handled:%t err:%v, want true/error", result.handled, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("claim-fenced persistence remained blocked after releasing the event")
	}
	var responseCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM bot_history_messages
		WHERE turn_id = (SELECT turn_id FROM bot_history_messages WHERE id = $1)
		  AND role IN ('assistant', 'tool')
	`, request.ID).Scan(&responseCount); err != nil {
		t.Fatalf("count event-lock rolled-back responses: %v", err)
	}
	if responseCount != 0 {
		t.Fatalf("event-lock expired claim committed %d responses, want 0", responseCount)
	}
}

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

func TestPostgresListUncoveredTurnResponsesPreservesDurableOrder(t *testing.T) {
	ctx := context.Background()
	tx := beginPostgresMessageTestTx(t, ctx)
	setupPostgresMessageTestFixtures(t, ctx, tx)

	const (
		firstTurnID       = "00000000-0000-0000-0000-000000074301"
		firstUserID       = "00000000-0000-0000-0000-000000074302"
		firstAssistantID  = "00000000-0000-0000-0000-000000074303"
		secondTurnID      = "00000000-0000-0000-0000-000000074304"
		secondUserID      = "00000000-0000-0000-0000-000000074305"
		secondAssistantID = "00000000-0000-0000-0000-000000074306"
	)
	if _, err := tx.Exec(ctx, `
		INSERT INTO bot_history_messages (
			id, bot_id, session_id, role, content,
			turn_id, turn_position, turn_message_seq, turn_visible, created_at
		)
		VALUES
			($1, $7, $8, 'user', '{"role":"user","content":"first"}'::jsonb, $5, 1, 1, true, now() - interval '4 minutes'),
			($2, $7, $8, 'assistant', '{"role":"assistant","content":"first answer"}'::jsonb, $5, 1, 2, true, now() - interval '1 minute'),
			($3, $7, $8, 'user', '{"role":"user","content":"second"}'::jsonb, $6, 2, 1, true, now() - interval '3 minutes'),
			($4, $7, $8, 'assistant', '{"role":"assistant","content":"second answer"}'::jsonb, $6, 2, 2, true, now() - interval '2 minutes')
	`, firstUserID, firstAssistantID, secondUserID, secondAssistantID,
		firstTurnID, secondTurnID, postgresMessageTestBotID, postgresMessageTestSessionID); err != nil {
		t.Fatalf("insert positioned turn responses: %v", err)
	}

	service := NewService(nil, postgresstore.NewQueries(dbsqlc.New(tx)))
	responses, err := service.ListUncoveredTurnResponsesBySession(
		ctx,
		postgresMessageTestSessionID,
		time.Now().UTC().Add(-time.Hour),
		nil,
	)
	if err != nil {
		t.Fatalf("list uncovered turn responses: %v", err)
	}
	want := []struct {
		id       string
		position int64
		sequence int64
	}{
		{id: firstAssistantID, position: 1, sequence: 2},
		{id: secondAssistantID, position: 2, sequence: 2},
	}
	if len(responses) != len(want) {
		t.Fatalf("uncovered turn responses = %#v, want %d", responses, len(want))
	}
	for i, expected := range want {
		response := responses[i]
		if response.ID != expected.id || response.TurnPosition != expected.position || response.TurnMessageSequence != expected.sequence {
			t.Fatalf("response %d identity/position = %s %d/%d, want %s %d/%d", i,
				response.ID, response.TurnPosition, response.TurnMessageSequence,
				expected.id, expected.position, expected.sequence)
		}
	}
}

func openNamedTurnResponseCursorTestPool(t *testing.T, ctx context.Context, applicationName string) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(os.Getenv("TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	config.MaxConns = 1
	config.ConnConfig.RuntimeParams["application_name"] = applicationName
	config.AfterConnect = dbpkg.SetDefaultTeamOnConnect
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open named postgres pool: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping named postgres pool: %v", err)
	}
	return pool
}

func waitForPostgresLockWait(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	applicationName string,
	resultCh <-chan claimPersistResult,
) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case result := <-resultCh:
			t.Fatalf("claim-fenced persistence returned before event lock release: handled:%t err:%v", result.handled, result.err)
		default:
		}
		var waiting bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE application_name = $1
				  AND wait_event_type = 'Lock'
			)
		`, applicationName).Scan(&waiting); err != nil {
			t.Fatalf("inspect postgres lock wait: %v", err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("claim-fenced persistence did not reach the event row lock")
}

func openTurnResponseCursorTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	pool, err := dbpkg.OpenPostgresDSN(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return pool
}

func assertTurnResponseClaimState(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	requestIDs []string,
	wantResponses []int,
	sessionID, scopeKey string,
	eventIDs, claimTokens []string,
	wantCommitted bool,
) {
	t.Helper()
	if len(requestIDs) != len(wantResponses) {
		t.Fatalf("request/response expectations = %d/%d", len(requestIDs), len(wantResponses))
	}
	for i, requestID := range requestIDs {
		var responses int
		if err := pool.QueryRow(ctx, `
			SELECT count(*)
			FROM bot_history_messages
			WHERE turn_id = (SELECT turn_id FROM bot_history_messages WHERE id = $1)
			  AND role IN ('assistant', 'tool')
		`, requestID).Scan(&responses); err != nil {
			t.Fatalf("count responses for request %d: %v", i, err)
		}
		if responses != wantResponses[i] {
			t.Fatalf("response count for request %d = %d, want %d", i, responses, wantResponses[i])
		}
	}
	cursor, err := dbsqlc.New(pool).GetSessionDiscussCursor(ctx, dbsqlc.GetSessionDiscussCursorParams{
		SessionID: mustTestUUID(t, sessionID),
		ScopeKey:  scopeKey,
	})
	if !wantCommitted {
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("cursor after rollback = %#v, %v, want no rows", cursor, err)
		}
	} else if err != nil || cursor.ConsumedEventCursor != 20 {
		t.Fatalf("committed cursor = %#v, %v, want event cursor 20", cursor, err)
	}
	for i, eventID := range eventIDs {
		var completed bool
		var token pgtype.UUID
		var claimedUntil pgtype.Timestamptz
		if err := pool.QueryRow(ctx, `
			SELECT delivery_completed_at IS NOT NULL, delivery_claim_token, delivery_claimed_until
			FROM bot_session_events
			WHERE id = $1
		`, eventID).Scan(&completed, &token, &claimedUntil); err != nil {
			t.Fatalf("read event %d delivery: %v", i, err)
		}
		if completed != wantCommitted {
			t.Fatalf("event %d completed = %t, want %t", i, completed, wantCommitted)
		}
		if wantCommitted {
			if token.Valid || claimedUntil.Valid {
				t.Fatalf("event %d retained claim after commit: %v/%v", i, token, claimedUntil)
			}
		} else if !token.Valid || token.String() != claimTokens[i] || !claimedUntil.Valid {
			t.Fatalf("event %d claim after rollback = %v/%v, want %s/live", i, token, claimedUntil, claimTokens[i])
		}
	}
}
