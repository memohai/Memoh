package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestFinalizeCompactionRollupPartialOverlapSerializesWithoutPartialMutationPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParentSet(t, pool, 3)
	firstRollupID, secondRollupID := testUUID(), testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{firstRollupID, secondRollupID})

	firstConn, err := pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatalf("acquire first connection: %v", err)
	}
	t.Cleanup(firstConn.Release)
	secondConn, err := pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatalf("acquire second connection: %v", err)
	}
	t.Cleanup(secondConn.Release)
	var secondPID int32
	if err := secondConn.QueryRow(fixture.ctx, `SELECT pg_backend_pid()`).Scan(&secondPID); err != nil {
		t.Fatalf("read second backend pid: %v", err)
	}

	firstTx, err := firstConn.Begin(fixture.ctx)
	if err != nil {
		t.Fatalf("begin first rollup: %v", err)
	}
	defer func() { _ = firstTx.Rollback(fixture.ctx) }()
	firstParents := fixture.parentIDs[:2]
	first, err := sqlc.New(firstTx).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		firstRollupID,
		fixture.botID,
		fixture.sessionID,
		firstParents,
		"first checkpoint",
	))
	if err != nil || !first.Finalized {
		t.Fatalf("first rollup = %+v, %v", first, err)
	}

	secondParents := fixture.parentIDs[1:]
	type finalizeResult struct {
		row sqlc.FinalizeCompactionRollupRow
		err error
	}
	resultCh := make(chan finalizeResult, 1)
	go func() {
		row, err := sqlc.New(secondConn).FinalizeCompactionRollup(fixture.ctx, rollupParams(
			secondRollupID,
			fixture.botID,
			fixture.sessionID,
			secondParents,
			"second checkpoint",
		))
		resultCh <- finalizeResult{row: row, err: err}
	}()
	waitForBackendLock(t, pool, secondPID)
	if err := firstTx.Commit(fixture.ctx); err != nil {
		t.Fatalf("commit first rollup: %v", err)
	}

	select {
	case second := <-resultCh:
		if second.err != nil || second.row.Finalized {
			t.Fatalf("stale overlapping rollup = %+v, %v; want rejection", second.row, second.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("overlapping rollup did not resume after first commit")
	}
	assertRollupCommitted(t, rollupFixture{
		ctx:       fixture.ctx,
		botID:     fixture.botID,
		sessionID: fixture.sessionID,
		parentIDs: firstParents,
	}, pool, firstRollupID)

	var secondStatus string
	if err := pool.QueryRow(fixture.ctx, `SELECT status FROM bot_history_message_compacts WHERE id = $1`, secondRollupID).Scan(&secondStatus); err != nil {
		t.Fatalf("read second target: %v", err)
	}
	if secondStatus != "pending" {
		t.Fatalf("second target status = %s, want pending", secondStatus)
	}
	var thirdSuccessor pgtype.UUID
	if err := pool.QueryRow(fixture.ctx, `SELECT superseded_by FROM bot_history_message_compacts WHERE id = $1`, fixture.parentIDs[2]).Scan(&thirdSuccessor); err != nil {
		t.Fatalf("read non-overlapping parent: %v", err)
	}
	if thirdSuccessor.Valid {
		t.Fatalf("failed overlapping rollup superseded non-overlapping parent by %s", thirdSuccessor)
	}
}

func TestFinalizeCompactionRollupRollsBackWhenParentSupersessionFailsPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParents(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})
	if _, err := pool.Exec(fixture.ctx, fmt.Sprintf(`
CREATE FUNCTION reject_rollup_parent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.id = '%s'::uuid AND NEW.superseded_by IS NOT NULL THEN
    RAISE EXCEPTION 'injected parent supersession failure';
  END IF;
  RETURN NEW;
END;
$$;
CREATE TRIGGER reject_rollup_parent_update
BEFORE UPDATE OF superseded_by ON bot_history_message_compacts
FOR EACH ROW EXECUTE FUNCTION reject_rollup_parent();
`, fixture.parentIDs[1].String())); err != nil {
		t.Fatalf("install parent failure trigger: %v", err)
	}

	_, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"checkpoint",
	))
	if err == nil {
		t.Fatal("FinalizeCompactionRollup() succeeded despite injected parent failure")
	}
	assertRollupNotCommitted(t, fixture, pool, rollupID)
	var edgeCount int
	if err := pool.QueryRow(fixture.ctx, `SELECT COUNT(*) FROM bot_history_message_compact_parent_edges WHERE artifact_id = $1`, rollupID).Scan(&edgeCount); err != nil {
		t.Fatalf("count rolled-back parent edges: %v", err)
	}
	if edgeCount != 0 {
		t.Fatalf("failed rollup retained %d normalized parent edges", edgeCount)
	}
}

func TestFinalizeCompactionRollupRejectsLockedSourceWithoutWaitingPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParents(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})

	mutator, err := pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatalf("acquire mutator: %v", err)
	}
	t.Cleanup(mutator.Release)

	mutationTx, err := mutator.Begin(fixture.ctx)
	if err != nil {
		t.Fatalf("begin source mutation: %v", err)
	}
	defer func() { _ = mutationTx.Rollback(fixture.ctx) }()
	if _, err := mutationTx.Exec(fixture.ctx, `UPDATE bot_history_messages SET content = '{"edited":true}' WHERE id = $1`, fixture.messageIDs[0]); err != nil {
		t.Fatalf("mutate rollup source: %v", err)
	}

	finalizeCtx, cancel := context.WithTimeout(fixture.ctx, time.Second)
	defer cancel()
	result, err := sqlc.New(pool).FinalizeCompactionRollup(finalizeCtx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"checkpoint",
	))
	if err != nil || result.Finalized {
		t.Fatalf("rollup against locked source = %+v, %v; want bounded rejection", result, err)
	}
	assertRollupNotCommitted(t, fixture, pool, rollupID)
	if err := mutationTx.Commit(fixture.ctx); err != nil {
		t.Fatalf("commit source mutation: %v", err)
	}
}

func TestRollupProjectionRejectsConcurrentHistoricalInsertPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParents(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})

	mutator, err := pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatalf("acquire historical writer: %v", err)
	}
	t.Cleanup(mutator.Release)
	mutationTx, err := mutator.Begin(fixture.ctx)
	if err != nil {
		t.Fatalf("begin historical insert: %v", err)
	}
	defer func() { _ = mutationTx.Rollback(fixture.ctx) }()
	if _, err := mutationTx.Exec(fixture.ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq
) VALUES ($1, $2, $3, $4, 1, 2)
`, testUUID(), fixture.botID, fixture.sessionID, testUUID()); err != nil {
		t.Fatalf("insert historical message: %v", err)
	}

	result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"checkpoint racing historical insert",
	))
	if err != nil || !result.Finalized {
		t.Fatalf("FinalizeCompactionRollup() = %+v, %v; want snapshot-valid finalize", result, err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, nil)
	if err := mutationTx.Commit(fixture.ctx); err != nil {
		t.Fatalf("commit historical insert: %v", err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, []pgtype.UUID{rollupID})
}

func TestRollupProjectionRejectsConcurrentHiddenGapActivationPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParentsWithGap(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})

	var gapID pgtype.UUID
	if err := pool.QueryRow(fixture.ctx, `
SELECT id
FROM bot_history_messages
WHERE session_id = $1
  AND id <> ALL($2::uuid[])
`, fixture.sessionID, fixture.messageIDs).Scan(&gapID); err != nil {
		t.Fatalf("read hidden gap: %v", err)
	}
	if _, err := pool.Exec(fixture.ctx, `UPDATE bot_history_messages SET turn_visible = false WHERE id = $1`, gapID); err != nil {
		t.Fatalf("hide history gap: %v", err)
	}

	mutator, err := pool.Acquire(fixture.ctx)
	if err != nil {
		t.Fatalf("acquire gap activator: %v", err)
	}
	t.Cleanup(mutator.Release)
	mutationTx, err := mutator.Begin(fixture.ctx)
	if err != nil {
		t.Fatalf("begin gap activation: %v", err)
	}
	defer func() { _ = mutationTx.Rollback(fixture.ctx) }()
	if _, err := mutationTx.Exec(fixture.ctx, `UPDATE bot_history_messages SET turn_visible = true WHERE id = $1`, gapID); err != nil {
		t.Fatalf("activate hidden gap: %v", err)
	}

	result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"checkpoint racing hidden activation",
	))
	if err != nil || !result.Finalized {
		t.Fatalf("FinalizeCompactionRollup() = %+v, %v; want snapshot-valid finalize", result, err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, nil)
	if err := mutationTx.Commit(fixture.ctx); err != nil {
		t.Fatalf("commit hidden gap activation: %v", err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, []pgtype.UUID{rollupID})
}

func waitForBackendLock(t *testing.T, pool *pgxpool.Pool, backendPID int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waitType pgtype.Text
		if err := pool.QueryRow(context.Background(), `SELECT wait_event_type FROM pg_stat_activity WHERE pid = $1`, backendPID).Scan(&waitType); err != nil {
			t.Fatalf("read backend wait state: %v", err)
		}
		if waitType.Valid && waitType.String == "Lock" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("backend %d did not wait on a source lock", backendPID)
}
