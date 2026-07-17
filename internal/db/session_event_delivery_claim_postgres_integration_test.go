package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestCompleteSessionEventDeliveryRejectsClaimExpiredWhileWaitingForEventLock(t *testing.T) {
	ctx := context.Background()
	pool := openSessionEventDeliveryClaimTestPool(t, ctx, "")
	eventID, claimToken := createSessionEventDeliveryClaimFixture(t, ctx, pool, true, 500*time.Millisecond)

	blocker := lockSessionEventDeliveryRow(t, ctx, pool, eventID)
	result := make(chan completeSessionEventDeliveryResult, 1)
	applicationName := "complete-delivery-" + uuid.NewString()
	queryPool := openSessionEventDeliveryClaimTestPool(t, ctx, applicationName)
	go func() {
		rows, err := sqlc.New(queryPool).CompleteSessionEventDelivery(ctx, sqlc.CompleteSessionEventDeliveryParams{
			EventID: eventID, ClaimToken: claimToken,
		})
		result <- completeSessionEventDeliveryResult{rows: rows, err: err}
	}()
	waitForSessionEventDeliveryLock(t, ctx, pool, applicationName)
	waitForSessionEventDeliveryClaimExpiry(t, ctx, pool, eventID)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release event blocker: %v", err)
	}

	select {
	case got := <-result:
		if got.err != nil || got.rows != 0 {
			t.Fatalf("complete expired claim after lock wait = %d, %v, want 0/nil", got.rows, got.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("complete expired claim remained blocked after event lock release")
	}
}

func TestRenewSessionEventDeliveryRejectsClaimExpiredWhileWaitingForEventLock(t *testing.T) {
	ctx := context.Background()
	pool := openSessionEventDeliveryClaimTestPool(t, ctx, "")
	eventID, claimToken := createSessionEventDeliveryClaimFixture(t, ctx, pool, false, 500*time.Millisecond)

	blocker := lockSessionEventDeliveryRow(t, ctx, pool, eventID)
	result := make(chan error, 1)
	applicationName := "renew-delivery-" + uuid.NewString()
	queryPool := openSessionEventDeliveryClaimTestPool(t, ctx, applicationName)
	go func() {
		_, err := sqlc.New(queryPool).RenewSessionEventDelivery(ctx, sqlc.RenewSessionEventDeliveryParams{
			EventID: eventID, ClaimToken: claimToken, LeaseMs: time.Minute.Milliseconds(),
		})
		result <- err
	}()
	waitForSessionEventDeliveryLock(t, ctx, pool, applicationName)
	waitForSessionEventDeliveryClaimExpiry(t, ctx, pool, eventID)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release event blocker: %v", err)
	}

	select {
	case err := <-result:
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("renew expired claim after lock wait error = %v, want no rows", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("renew expired claim remained blocked after event lock release")
	}
}

func TestRenewSessionEventDeliveryExtendsLeaseFromEventLockAcquisition(t *testing.T) {
	ctx := context.Background()
	pool := openSessionEventDeliveryClaimTestPool(t, ctx, "")
	eventID, claimToken := createSessionEventDeliveryClaimFixture(t, ctx, pool, false, 5*time.Second)

	blocker := lockSessionEventDeliveryRow(t, ctx, pool, eventID)
	result := make(chan renewSessionEventDeliveryResult, 1)
	applicationName := "renew-delivery-expiry-" + uuid.NewString()
	queryPool := openSessionEventDeliveryClaimTestPool(t, ctx, applicationName)
	go func() {
		claimedUntil, err := sqlc.New(queryPool).RenewSessionEventDelivery(ctx, sqlc.RenewSessionEventDeliveryParams{
			EventID: eventID, ClaimToken: claimToken, LeaseMs: time.Second.Milliseconds(),
		})
		result <- renewSessionEventDeliveryResult{claimedUntil: claimedUntil, err: err}
	}()
	waitForSessionEventDeliveryLock(t, ctx, pool, applicationName)
	time.Sleep(400 * time.Millisecond)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatalf("release event blocker: %v", err)
	}

	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("renew claim after lock wait: %v", got.err)
		}
		var databaseNow time.Time
		if err := pool.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&databaseNow); err != nil {
			t.Fatalf("read database clock: %v", err)
		}
		if remaining := got.claimedUntil.Time.Sub(databaseNow); remaining < 800*time.Millisecond {
			t.Fatalf("renewed lease remaining after lock release = %s, want at least 800ms", remaining)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("renew claim remained blocked after event lock release")
	}
}

type completeSessionEventDeliveryResult struct {
	rows int64
	err  error
}

type renewSessionEventDeliveryResult struct {
	claimedUntil pgtype.Timestamptz
	err          error
}

func openSessionEventDeliveryClaimTestPool(t *testing.T, ctx context.Context, applicationName string) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse postgres DSN: %v", err)
	}
	config.AfterConnect = SetDefaultTeamOnConnect
	if applicationName != "" {
		config.MaxConns = 1
		config.ConnConfig.RuntimeParams["application_name"] = applicationName
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return pool
}

func createSessionEventDeliveryClaimFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	withHistory bool,
	leaseDuration time.Duration,
) (pgtype.UUID, pgtype.UUID) {
	t.Helper()
	userID := uuid.New()
	botID := uuid.New()
	sessionID := uuid.New()
	eventID := uuid.New()
	claimToken := uuid.New()
	name := fmt.Sprintf("delivery-claim-%s", uuid.NewString()[:12])
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (id, username, role, is_active)
		VALUES ($1, $2, 'admin', true)
	`, userID, name); err != nil {
		t.Fatalf("insert delivery claim user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID) })
	if _, err := pool.Exec(ctx, `
		INSERT INTO bots (id, owner_user_id, name)
		VALUES ($1, $2, $3)
	`, botID, userID, name); err != nil {
		t.Fatalf("insert delivery claim bot: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bot_sessions (id, bot_id, channel_type)
		VALUES ($1, $2, 'local')
	`, sessionID, botID); err != nil {
		t.Fatalf("insert delivery claim session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bot_session_events (
			id, bot_id, session_id, event_kind, event_data, external_message_id, received_at_ms
		)
		VALUES ($1, $2, $3, 'message', '{}'::jsonb, $4, 1)
	`, eventID, botID, sessionID, "delivery-claim-"+eventID.String()); err != nil {
		t.Fatalf("insert delivery claim event: %v", err)
	}
	if withHistory {
		if _, err := pool.Exec(ctx, `
			INSERT INTO bot_history_messages (
				bot_id, session_id, event_id, role, content, turn_id, turn_message_seq
			)
			VALUES ($1, $2, $3, 'user', '{}'::jsonb, $4, 0)
		`, botID, sessionID, eventID, uuid.New()); err != nil {
			t.Fatalf("insert delivery claim history: %v", err)
		}
	}
	pgEventID := pgtype.UUID{Bytes: eventID, Valid: true}
	pgClaimToken := pgtype.UUID{Bytes: claimToken, Valid: true}
	if _, err := sqlc.New(pool).ClaimSessionEventDelivery(ctx, sqlc.ClaimSessionEventDeliveryParams{
		EventID: pgEventID, ClaimToken: pgClaimToken, LeaseMs: leaseDuration.Milliseconds(),
	}); err != nil {
		t.Fatalf("claim event delivery: %v", err)
	}
	return pgEventID, pgClaimToken
}

func lockSessionEventDeliveryRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID pgtype.UUID) pgx.Tx {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin event blocker: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	if _, err := tx.Exec(ctx, `SELECT id FROM bot_session_events WHERE id = $1 FOR UPDATE`, eventID); err != nil {
		t.Fatalf("lock event row: %v", err)
	}
	return tx
}

func waitForSessionEventDeliveryLock(t *testing.T, ctx context.Context, pool *pgxpool.Pool, applicationName string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM pg_stat_activity
				WHERE application_name = $1
				  AND wait_event_type = 'Lock'
			)
		`, applicationName).Scan(&waiting); err != nil {
			t.Fatalf("inspect delivery claim lock wait: %v", err)
		}
		if waiting {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("delivery claim query did not reach the event row lock")
}

func waitForSessionEventDeliveryClaimExpiry(t *testing.T, ctx context.Context, pool *pgxpool.Pool, eventID pgtype.UUID) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var expired bool
		if err := pool.QueryRow(ctx, `
			SELECT delivery_claimed_until <= clock_timestamp()
			FROM bot_session_events
			WHERE id = $1
		`, eventID).Scan(&expired); err != nil {
			t.Fatalf("read delivery claim expiry: %v", err)
		}
		if expired {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("delivery claim did not expire")
}
