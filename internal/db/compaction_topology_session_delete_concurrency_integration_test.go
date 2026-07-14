package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCompactionTopologyRevisionDoesNotDeadlockSessionDeletePostgresPath(t *testing.T) {
	pool := openTopologyMigrationTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sessionID, messageID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO bot_sessions (id) VALUES ($1)`, sessionID); err != nil {
		t.Fatalf("insert deleted session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq, turn_visible
) VALUES ($1, $2, $3, $4, 1, 1, true)
`, messageID, uuid.New(), sessionID, uuid.New()); err != nil {
		t.Fatalf("insert concurrently detached message: %v", err)
	}

	mutation, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin topology mutation: %v", err)
	}
	defer func() { _ = mutation.Rollback(context.Background()) }()
	if _, err := mutation.Exec(ctx, `UPDATE bot_history_messages SET turn_position = 2 WHERE id = $1`, messageID); err != nil {
		t.Fatalf("mutate topology before session delete: %v", err)
	}

	deleter, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire session deleter: %v", err)
	}
	t.Cleanup(deleter.Release)
	var deleterPID int32
	if err := deleter.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&deleterPID); err != nil {
		t.Fatalf("read session deleter pid: %v", err)
	}
	deleteResult := make(chan error, 1)
	go func() {
		_, err := deleter.Exec(ctx, `DELETE FROM bot_sessions WHERE id = $1`, sessionID)
		deleteResult <- err
	}()
	waitForBackendLock(t, pool, deleterPID)

	if err := mutation.Commit(ctx); err != nil {
		t.Fatalf("commit topology mutation against session delete: %v", err)
	}
	select {
	case err := <-deleteResult:
		if err != nil {
			t.Fatalf("delete session after topology mutation: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("session delete did not complete: %v", ctx.Err())
	}

	var sessionCount, topologyCount int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM bot_sessions WHERE id = $1`, sessionID).Scan(&sessionCount); err != nil {
		t.Fatalf("count deleted session: %v", err)
	}
	if err := pool.QueryRow(context.Background(), `
SELECT
  (SELECT COUNT(*) FROM bot_history_topology_counters WHERE session_id = $1)
  + (SELECT COUNT(*) FROM bot_history_topology_positions WHERE session_id = $1)
`, sessionID).Scan(&topologyCount); err != nil {
		t.Fatalf("count deleted session topology: %v", err)
	}
	if sessionCount != 0 || topologyCount != 0 {
		t.Fatalf("session delete left session=%d topology=%d", sessionCount, topologyCount)
	}
}
