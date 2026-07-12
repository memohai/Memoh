package db

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestFinalizeCompactionRollupDerivesArtifactShapePostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParents(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})

	result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"derived checkpoint",
	))
	if err != nil {
		t.Fatalf("FinalizeCompactionRollup() error = %v", err)
	}
	if !result.Finalized || result.RequestedCount != 2 || result.MatchedCount != 2 || result.SupersededCount != 2 {
		t.Fatalf("FinalizeCompactionRollup() = %+v", result)
	}

	var (
		status                     string
		messageCount, level        int32
		coverage                   []byte
		anchorStartMs, anchorEndMs int64
		parentIDs                  []pgtype.UUID
	)
	if err := pool.QueryRow(fixture.ctx, `
SELECT status, message_count, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids
FROM bot_history_message_compacts
WHERE id = $1
`, rollupID).Scan(&status, &messageCount, &coverage, &anchorStartMs, &anchorEndMs, &level, &parentIDs); err != nil {
		t.Fatalf("read derived artifact: %v", err)
	}
	if status != "ok" || messageCount != 2 || anchorStartMs != 1 || anchorEndMs != 1 || level != 1 {
		t.Fatalf("derived shape = status:%s count:%d anchors:%d-%d level:%d", status, messageCount, anchorStartMs, anchorEndMs, level)
	}
	if !equalPGUUIDs(parentIDs, fixture.parentIDs) {
		t.Fatalf("derived parents = %#v, want %#v", parentIDs, fixture.parentIDs)
	}
	var topologyRevision, currentRevision, rangeStart, rangeEnd int64
	if err := pool.QueryRow(fixture.ctx, `
SELECT
  topology.topology_revision,
  counter.revision,
  topology.range_start_turn_position,
  topology.range_end_turn_position
FROM bot_history_message_compact_topology topology
JOIN bot_history_topology_counters counter ON counter.session_id = topology.session_id
WHERE topology.compact_id = $1
`, rollupID).Scan(&topologyRevision, &currentRevision, &rangeStart, &rangeEnd); err != nil {
		t.Fatalf("read derived topology snapshot: %v", err)
	}
	if topologyRevision != currentRevision || rangeStart != 1 || rangeEnd != 2 {
		t.Fatalf("derived topology = revision:%d current:%d range:%d-%d", topologyRevision, currentRevision, rangeStart, rangeEnd)
	}
	var covered []struct {
		Ref struct {
			ID string `json:"id"`
		} `json:"ref"`
	}
	if err := json.Unmarshal(coverage, &covered); err != nil {
		t.Fatalf("decode derived coverage: %v", err)
	}
	if len(covered) != 2 || covered[0].Ref.ID != fixture.messageIDs[0].String() || covered[1].Ref.ID != fixture.messageIDs[1].String() {
		t.Fatalf("derived coverage = %#v", covered)
	}
	assertRollupCommitted(t, fixture, pool, rollupID)
}

func TestFinalizeCompactionRollupRejectsInvalidRequestWithoutMutationPostgresPath(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams
		wantErr bool
	}{
		{
			name: "duplicate parents",
			mutate: func(_ *testing.T, fixture rollupFixture, _ *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				params := rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
				params.ParentIds[1] = params.ParentIds[0]
				return params
			},
		},
		{
			name: "reversed parents",
			mutate: func(_ *testing.T, fixture rollupFixture, _ *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				params := rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
				params.ParentIds[0], params.ParentIds[1] = params.ParentIds[1], params.ParentIds[0]
				return params
			},
		},
		{
			name: "missing parent",
			mutate: func(_ *testing.T, fixture rollupFixture, _ *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				params := rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
				params.ParentIds[1] = testUUID()
				return params
			},
		},
		{
			name: "pending parent",
			mutate: func(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				pendingID := testUUID()
				insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{pendingID})
				params := rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
				params.ParentIds[1] = pendingID
				return params
			},
		},
		{
			name: "cross-session parent",
			mutate: func(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				if _, err := pool.Exec(fixture.ctx, `UPDATE bot_history_message_compacts SET session_id = $2 WHERE id = $1`, fixture.parentIDs[1], testUUID()); err != nil {
					t.Fatalf("move parent scope: %v", err)
				}
				return rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
			},
		},
		{
			name: "wrong target scope",
			mutate: func(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				if _, err := pool.Exec(fixture.ctx, `UPDATE bot_history_message_compacts SET session_id = $2 WHERE id = $1`, rollupID, testUUID()); err != nil {
					t.Fatalf("move rollup target: %v", err)
				}
				return rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
			},
		},
		{
			name: "invalid source ancestor",
			mutate: func(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				if _, err := pool.Exec(fixture.ctx, `UPDATE bot_history_messages SET content = '{"edited":true}' WHERE id = $1`, fixture.messageIDs[0]); err != nil {
					t.Fatalf("invalidate rollup source: %v", err)
				}
				return rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "checkpoint")
			},
		},
		{
			name: "target constraint failure rolls back parents",
			mutate: func(_ *testing.T, fixture rollupFixture, _ *pgxpool.Pool, rollupID pgtype.UUID) sqlc.FinalizeCompactionRollupParams {
				return rollupParams(rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "reject")
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := openCompactionFinalizeTestPool(t)
			fixture := createRollupParents(t, pool)
			rollupID := testUUID()
			insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})
			result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, test.mutate(t, fixture, pool, rollupID))
			if test.wantErr {
				if err == nil {
					t.Fatalf("FinalizeCompactionRollup() = %+v, want error", result)
				}
			} else if err != nil || result.Finalized {
				t.Fatalf("FinalizeCompactionRollup() = %+v, %v; want rejected without error", result, err)
			}
			assertRollupNotCommitted(t, fixture, pool, rollupID)
		})
	}
}

func TestFinalizeCompactionRollupRejectsRawHistoryGapPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParentsWithGap(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})

	result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"cross-gap checkpoint",
	))
	if err != nil || result.Finalized {
		t.Fatalf("FinalizeCompactionRollup() = %+v, %v; want gap rejection", result, err)
	}
	assertRollupNotCommitted(t, fixture, pool, rollupID)
}

type rollupFixture struct {
	ctx        context.Context
	botID      pgtype.UUID
	sessionID  pgtype.UUID
	messageIDs []pgtype.UUID
	parentIDs  []pgtype.UUID
}

func createRollupParents(t *testing.T, pool *pgxpool.Pool) rollupFixture {
	return createRollupParentSet(t, pool, 2)
}

func createRollupParentSet(t *testing.T, pool *pgxpool.Pool, count int) rollupFixture {
	t.Helper()
	installRollupParentEdgesFixture(t, pool)
	fixture := rollupFixture{
		ctx:       context.Background(),
		botID:     testUUID(),
		sessionID: testUUID(),
	}
	for range count {
		fixture.messageIDs = append(fixture.messageIDs, testUUID())
		fixture.parentIDs = append(fixture.parentIDs, testUUID())
	}
	if _, err := pool.Exec(fixture.ctx, `INSERT INTO bot_sessions (id) VALUES ($1)`, fixture.sessionID); err != nil {
		t.Fatalf("insert rollup session: %v", err)
	}
	insertFinalizeMessages(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, fixture.messageIDs)
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, fixture.parentIDs)
	queries := sqlc.New(pool)
	for index := range fixture.parentIDs {
		result, err := queries.FinalizeCompactionArtifact(fixture.ctx, finalizeParams(
			fixture.parentIDs[index],
			fixture.botID,
			fixture.sessionID,
			[]pgtype.UUID{fixture.messageIDs[index]},
			readMessageVersions(t, fixture.ctx, pool, []pgtype.UUID{fixture.messageIDs[index]}),
			"parent summary",
		))
		if err != nil || !result.Finalized {
			t.Fatalf("finalize rollup parent %d = %+v, %v", index, result, err)
		}
	}
	return fixture
}

func createRollupParentsWithGap(t *testing.T, pool *pgxpool.Pool) rollupFixture {
	t.Helper()
	installRollupParentEdgesFixture(t, pool)
	allMessageIDs := []pgtype.UUID{testUUID(), testUUID(), testUUID()}
	fixture := rollupFixture{
		ctx:        context.Background(),
		botID:      testUUID(),
		sessionID:  testUUID(),
		messageIDs: []pgtype.UUID{allMessageIDs[0], allMessageIDs[2]},
		parentIDs:  []pgtype.UUID{testUUID(), testUUID()},
	}
	if _, err := pool.Exec(fixture.ctx, `INSERT INTO bot_sessions (id) VALUES ($1)`, fixture.sessionID); err != nil {
		t.Fatalf("insert gapped rollup session: %v", err)
	}
	insertFinalizeMessages(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, allMessageIDs)
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, fixture.parentIDs)
	queries := sqlc.New(pool)
	for index := range fixture.parentIDs {
		result, err := queries.FinalizeCompactionArtifact(fixture.ctx, finalizeParams(
			fixture.parentIDs[index],
			fixture.botID,
			fixture.sessionID,
			[]pgtype.UUID{fixture.messageIDs[index]},
			readMessageVersions(t, fixture.ctx, pool, []pgtype.UUID{fixture.messageIDs[index]}),
			"parent summary",
		))
		if err != nil || !result.Finalized {
			t.Fatalf("finalize gapped rollup parent %d = %+v, %v", index, result, err)
		}
	}
	return fixture
}

func installRollupParentEdgesFixture(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS bot_sessions (id UUID PRIMARY KEY)`); err != nil {
		t.Fatalf("install rollup session fixture: %v", err)
	}
	if _, err := pool.Exec(context.Background(), readEmbeddedMigration(t, "postgres/migrations/0106_compaction_artifact_parent_edges.up.sql")); err != nil {
		t.Fatalf("install rollup parent edges fixture: %v", err)
	}
	if _, err := pool.Exec(context.Background(), readEmbeddedMigration(t, "postgres/migrations/0113_compaction_topology_positions.up.sql")); err != nil {
		t.Fatalf("install rollup topology fixture: %v", err)
	}
}

func rollupParams(
	rollupID, botID, sessionID pgtype.UUID,
	parentIDs []pgtype.UUID,
	summary string,
) sqlc.FinalizeCompactionRollupParams {
	return sqlc.FinalizeCompactionRollupParams{
		RollupID:       rollupID,
		ScopeBotID:     botID,
		ScopeSessionID: sessionID,
		ParentIds:      append([]pgtype.UUID(nil), parentIDs...),
		Summary:        summary,
		Usage:          []byte(`{"total_tokens":42}`),
	}
}

func assertRollupCommitted(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) {
	t.Helper()
	for _, parentID := range fixture.parentIDs {
		var successor pgtype.UUID
		if err := pool.QueryRow(fixture.ctx, `SELECT superseded_by FROM bot_history_message_compacts WHERE id = $1`, parentID).Scan(&successor); err != nil {
			t.Fatalf("read parent successor: %v", err)
		}
		if successor != rollupID {
			t.Fatalf("parent %s successor = %s, want %s", parentID, successor, rollupID)
		}
	}
	var edgeIDs []pgtype.UUID
	if err := pool.QueryRow(fixture.ctx, `
SELECT array_agg(parent_id ORDER BY ordinal)
FROM bot_history_message_compact_parent_edges
WHERE artifact_id = $1
`, rollupID).Scan(&edgeIDs); err != nil {
		t.Fatalf("read normalized parent edges: %v", err)
	}
	if !equalPGUUIDs(edgeIDs, fixture.parentIDs) {
		t.Fatalf("normalized edges = %#v, want %#v", edgeIDs, fixture.parentIDs)
	}
	var topologyCount int
	if err := pool.QueryRow(fixture.ctx, `SELECT COUNT(*) FROM bot_history_message_compact_topology WHERE compact_id = $1`, rollupID).Scan(&topologyCount); err != nil {
		t.Fatalf("count committed topology snapshot: %v", err)
	}
	if topologyCount != 1 {
		t.Fatalf("committed rollup has %d topology snapshots", topologyCount)
	}
}

func assertRollupNotCommitted(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, rollupID pgtype.UUID) {
	t.Helper()
	var status string
	var parentIDs []pgtype.UUID
	if err := pool.QueryRow(fixture.ctx, `SELECT status, parent_ids FROM bot_history_message_compacts WHERE id = $1`, rollupID).Scan(&status, &parentIDs); err != nil {
		t.Fatalf("read rejected rollup: %v", err)
	}
	if status != "pending" || len(parentIDs) != 0 {
		t.Fatalf("rejected rollup mutated target: status=%s parents=%#v", status, parentIDs)
	}
	var topologyCount int
	if err := pool.QueryRow(fixture.ctx, `SELECT COUNT(*) FROM bot_history_message_compact_topology WHERE compact_id = $1`, rollupID).Scan(&topologyCount); err != nil {
		t.Fatalf("count rejected topology snapshot: %v", err)
	}
	if topologyCount != 0 {
		t.Fatalf("rejected rollup retained %d topology snapshots", topologyCount)
	}
	for _, parentID := range fixture.parentIDs {
		var successor pgtype.UUID
		if err := pool.QueryRow(fixture.ctx, `SELECT superseded_by FROM bot_history_message_compacts WHERE id = $1`, parentID).Scan(&successor); err != nil {
			t.Fatalf("read rejected parent: %v", err)
		}
		if successor.Valid {
			t.Fatalf("rejected rollup superseded parent %s by %s", parentID, successor)
		}
	}
}

func equalPGUUIDs(left, right []pgtype.UUID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
