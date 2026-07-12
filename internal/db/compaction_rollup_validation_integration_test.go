package db

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestFinalizeCompactionRollupBuildsSecondLevelArtifactPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParentSet(t, pool, 3)
	firstRollupID, secondRollupID := testUUID(), testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{firstRollupID, secondRollupID})
	queries := sqlc.New(pool)

	first, err := queries.FinalizeCompactionRollup(fixture.ctx, rollupParams(
		firstRollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs[:2],
		"first checkpoint",
	))
	if err != nil || !first.Finalized {
		t.Fatalf("first rollup = %+v, %v", first, err)
	}
	secondParents := []pgtype.UUID{firstRollupID, fixture.parentIDs[2]}
	second, err := queries.FinalizeCompactionRollup(fixture.ctx, rollupParams(
		secondRollupID,
		fixture.botID,
		fixture.sessionID,
		secondParents,
		"second checkpoint",
	))
	if err != nil || !second.Finalized {
		t.Fatalf("second rollup = %+v, %v", second, err)
	}

	var messageCount, level int32
	var coverage []byte
	var parentIDs []pgtype.UUID
	if err := pool.QueryRow(fixture.ctx, `
SELECT message_count, coverage, artifact_level, parent_ids
FROM bot_history_message_compacts
WHERE id = $1
`, secondRollupID).Scan(&messageCount, &coverage, &level, &parentIDs); err != nil {
		t.Fatalf("read second-level rollup: %v", err)
	}
	if messageCount != 3 || level != 2 || !equalPGUUIDs(parentIDs, secondParents) {
		t.Fatalf("second-level shape = count:%d level:%d parents:%#v", messageCount, level, parentIDs)
	}
	var covered []struct {
		Ref struct {
			ID string `json:"id"`
		} `json:"ref"`
	}
	if err := json.Unmarshal(coverage, &covered); err != nil {
		t.Fatalf("decode second-level coverage: %v", err)
	}
	if len(covered) != len(fixture.messageIDs) {
		t.Fatalf("second-level coverage count = %d, want %d", len(covered), len(fixture.messageIDs))
	}
	for index := range covered {
		if covered[index].Ref.ID != fixture.messageIDs[index].String() {
			t.Fatalf("second-level coverage[%d] = %q, want %q", index, covered[index].Ref.ID, fixture.messageIDs[index])
		}
	}
	var directClaims int
	if err := pool.QueryRow(fixture.ctx, `SELECT COUNT(*) FROM bot_history_messages WHERE compact_id = $1`, secondRollupID).Scan(&directClaims); err != nil {
		t.Fatalf("count second-level direct claims: %v", err)
	}
	if directClaims != 0 {
		t.Fatalf("second-level rollup owns %d direct message claims", directClaims)
	}
	for index, messageID := range fixture.messageIDs {
		var compactID pgtype.UUID
		if err := pool.QueryRow(fixture.ctx, `SELECT compact_id FROM bot_history_messages WHERE id = $1`, messageID).Scan(&compactID); err != nil {
			t.Fatalf("read source claim %d: %v", index, err)
		}
		if compactID != fixture.parentIDs[index] {
			t.Fatalf("source claim %d = %s, want leaf %s", index, compactID, fixture.parentIDs[index])
		}
	}
	assertRollupCommitted(t, rollupFixture{
		ctx:       fixture.ctx,
		botID:     fixture.botID,
		sessionID: fixture.sessionID,
		parentIDs: secondParents,
	}, pool, secondRollupID)
	for _, leafID := range fixture.parentIDs[:2] {
		var successor pgtype.UUID
		if err := pool.QueryRow(fixture.ctx, `SELECT superseded_by FROM bot_history_message_compacts WHERE id = $1`, leafID).Scan(&successor); err != nil {
			t.Fatalf("read first-level leaf successor: %v", err)
		}
		if successor != firstRollupID {
			t.Fatalf("first-level leaf %s successor = %s, want %s", leafID, successor, firstRollupID)
		}
	}
	if _, err := pool.Exec(fixture.ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq
) VALUES ($1, $2, $3, $4, 1, 2)
`, testUUID(), fixture.botID, fixture.sessionID, testUUID()); err != nil {
		t.Fatalf("insert gap across rollup generations: %v", err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, []pgtype.UUID{firstRollupID, secondRollupID})
}

func TestFinalizeCompactionRollupRejectsOverlappingParentCoveragePostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParentSet(t, pool, 1)
	var coverage []byte
	if err := pool.QueryRow(fixture.ctx, `SELECT coverage FROM bot_history_message_compacts WHERE id = $1`, fixture.parentIDs[0]).Scan(&coverage); err != nil {
		t.Fatalf("read leaf coverage: %v", err)
	}
	derivedIDs := []pgtype.UUID{testUUID(), testUUID()}
	for _, derivedID := range derivedIDs {
		if _, err := pool.Exec(fixture.ctx, `
INSERT INTO bot_history_message_compacts (
  id, bot_id, session_id, status, summary, message_count, coverage,
  anchor_start_ms, anchor_end_ms, artifact_level, parent_ids, completed_at
) VALUES ($1, $2, $3, 'ok', 'overlapping parent', 1, $4, 1, 1, 1, ARRAY[$5]::uuid[], now())
`, derivedID, fixture.botID, fixture.sessionID, coverage, fixture.parentIDs[0]); err != nil {
			t.Fatalf("insert overlapping derived parent: %v", err)
		}
	}
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})

	result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		derivedIDs,
		"invalid overlap",
	))
	if err != nil || result.Finalized {
		t.Fatalf("overlapping coverage rollup = %+v, %v; want rejection", result, err)
	}
	assertRollupNotCommitted(t, rollupFixture{
		ctx:       fixture.ctx,
		botID:     fixture.botID,
		sessionID: fixture.sessionID,
		parentIDs: derivedIDs,
	}, pool, rollupID)
}

func TestFinalizeCompactionRollupRejectsMissingOrTerminalTargetPostgresPath(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "terminal"}[terminal], func(t *testing.T) {
			pool := openCompactionFinalizeTestPool(t)
			fixture := createRollupParents(t, pool)
			rollupID := testUUID()
			if terminal {
				insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})
				if _, err := pool.Exec(fixture.ctx, `UPDATE bot_history_message_compacts SET status = 'error', completed_at = now() WHERE id = $1`, rollupID); err != nil {
					t.Fatalf("terminalize target: %v", err)
				}
			}
			result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
				rollupID,
				fixture.botID,
				fixture.sessionID,
				fixture.parentIDs,
				"checkpoint",
			))
			if err != nil || result.Finalized {
				t.Fatalf("invalid target rollup = %+v, %v; want rejection", result, err)
			}
			assertParentsActive(t, fixture, fixture.parentIDs, pool)
		})
	}
}

func TestRollupBecomesInvalidWhenHistoryGapBecomesVisiblePostgresPath(t *testing.T) {
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

	result, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"checkpoint before hidden gap",
	))
	if err != nil || !result.Finalized {
		t.Fatalf("FinalizeCompactionRollup() = %+v, %v; want finalized before gap activation", result, err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, nil)
	if _, err := pool.Exec(fixture.ctx, `
INSERT INTO bot_history_messages (
  id, bot_id, session_id, turn_id, turn_position, turn_message_seq
) VALUES ($1, $2, $3, $4, 4, 1)
`, testUUID(), fixture.botID, fixture.sessionID, testUUID()); err != nil {
		t.Fatalf("append history after rollup range: %v", err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, nil)

	if _, err := pool.Exec(fixture.ctx, `UPDATE bot_history_messages SET turn_visible = true WHERE id = $1`, gapID); err != nil {
		t.Fatalf("activate history gap: %v", err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, []pgtype.UUID{rollupID})
}

func TestDerivedArtifactWithoutTopologySnapshotIsInvalidPostgresPath(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	fixture := createRollupParentSet(t, pool, 3)
	rollupID := testUUID()
	if _, err := pool.Exec(fixture.ctx, `
INSERT INTO bot_history_message_compacts (
  id, bot_id, session_id, status, summary, message_count, coverage,
  anchor_start_ms, anchor_end_ms, artifact_level, parent_ids, completed_at
)
SELECT
  $1, $2, $3, 'ok', 'legacy checkpoint', 3,
  jsonb_build_array(first.coverage->0, second.coverage->0, third.coverage->0),
  1, 1, 1, $4::uuid[], now()
FROM bot_history_message_compacts first
JOIN bot_history_message_compacts second ON second.id = $6
JOIN bot_history_message_compacts third ON third.id = $7
WHERE first.id = $5
`, rollupID, fixture.botID, fixture.sessionID, fixture.parentIDs,
		fixture.parentIDs[0], fixture.parentIDs[1], fixture.parentIDs[2]); err != nil {
		t.Fatalf("insert derived artifact without topology: %v", err)
	}
	assertInvalidRollupSeeds(t, fixture, pool, []pgtype.UUID{rollupID})
}

func assertInvalidRollupSeeds(t *testing.T, fixture rollupFixture, pool *pgxpool.Pool, want []pgtype.UUID) {
	t.Helper()
	rows, err := sqlc.New(pool).ListInvalidCompactionArtifactSeedsBySession(
		fixture.ctx,
		sqlc.ListInvalidCompactionArtifactSeedsBySessionParams{
			BotID:     fixture.botID,
			SessionID: fixture.sessionID,
		},
	)
	if err != nil {
		t.Fatalf("ListInvalidCompactionArtifactSeedsBySession() error = %v", err)
	}
	got := make([]pgtype.UUID, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.ID)
	}
	if !samePGUUIDSet(got, want) {
		t.Fatalf("invalid rollup seeds = %#v, want %#v", got, want)
	}
}

func samePGUUIDSet(left, right []pgtype.UUID) bool {
	if len(left) != len(right) {
		return false
	}
	want := make(map[pgtype.UUID]int, len(right))
	for _, id := range right {
		want[id]++
	}
	for _, id := range left {
		if want[id] == 0 {
			return false
		}
		want[id]--
	}
	return true
}

func assertParentsActive(t *testing.T, fixture rollupFixture, parentIDs []pgtype.UUID, queries *pgxpool.Pool) {
	t.Helper()
	for _, parentID := range parentIDs {
		var successor pgtype.UUID
		if err := queries.QueryRow(fixture.ctx, `SELECT superseded_by FROM bot_history_message_compacts WHERE id = $1`, parentID).Scan(&successor); err != nil {
			t.Fatalf("read parent activity: %v", err)
		}
		if successor.Valid {
			t.Fatalf("parent %s unexpectedly superseded by %s", parentID, successor)
		}
	}
}
