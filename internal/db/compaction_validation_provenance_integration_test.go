package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/memohai/memoh/internal/db/postgres/sqlc"
)

func TestStrictCompactionFinalizersRecordValidationProvenance(t *testing.T) {
	pool := openCompactionFinalizeTestPool(t)
	ctx := context.Background()

	botID, sessionID := testUUID(), testUUID()
	messageID, directID := testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{messageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{directID})
	direct, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
		directID,
		botID,
		sessionID,
		[]pgtype.UUID{messageID},
		readMessageVersions(t, ctx, pool, []pgtype.UUID{messageID}),
		"direct summary",
	))
	if err != nil || !direct.Finalized {
		t.Fatalf("FinalizeCompactionArtifact() = %+v, %v", direct, err)
	}
	assertCompactionValidationProvenance(t, ctx, pool, directID, true)
	staleMessageID, staleID := testUUID(), testUUID()
	insertFinalizeMessages(t, ctx, pool, botID, sessionID, []pgtype.UUID{staleMessageID})
	insertFinalizeLogs(t, ctx, pool, botID, sessionID, []pgtype.UUID{staleID})
	stale, err := sqlc.New(pool).FinalizeCompactionArtifact(ctx, finalizeParams(
		staleID,
		botID,
		sessionID,
		[]pgtype.UUID{staleMessageID},
		[]string{"stale"},
		"stale summary",
	))
	if err != nil || stale.Finalized {
		t.Fatalf("stale FinalizeCompactionArtifact() = %+v, %v", stale, err)
	}
	assertCompactionValidationProvenance(t, ctx, pool, staleID, false)

	fixture := createRollupParents(t, pool)
	rollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rollupID})
	rollup, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rollupParams(
		rollupID,
		fixture.botID,
		fixture.sessionID,
		fixture.parentIDs,
		"rollup summary",
	))
	if err != nil || !rollup.Finalized {
		t.Fatalf("FinalizeCompactionRollup() = %+v, %v", rollup, err)
	}
	assertCompactionValidationProvenance(t, fixture.ctx, pool, rollupID, true)
	rejectedRollupID := testUUID()
	insertFinalizeLogs(t, fixture.ctx, pool, fixture.botID, fixture.sessionID, []pgtype.UUID{rejectedRollupID})
	rejectedParams := rollupParams(rejectedRollupID, fixture.botID, fixture.sessionID, fixture.parentIDs, "rejected")
	rejectedParams.ParentIds[1] = rejectedParams.ParentIds[0]
	rejected, err := sqlc.New(pool).FinalizeCompactionRollup(fixture.ctx, rejectedParams)
	if err != nil || rejected.Finalized {
		t.Fatalf("rejected FinalizeCompactionRollup() = %+v, %v", rejected, err)
	}
	assertCompactionValidationProvenance(t, fixture.ctx, pool, rejectedRollupID, false)

	legacyID := testUUID()
	if _, err := pool.Exec(ctx, `
INSERT INTO bot_history_message_compacts (id, bot_id, session_id, status, summary)
VALUES ($1, $2, $3, 'ok', 'legacy')
`, legacyID, botID, sessionID); err != nil {
		t.Fatalf("create legacy artifact: %v", err)
	}
	assertCompactionValidationProvenance(t, ctx, pool, legacyID, false)
	metadata, err := sqlc.New(pool).ListCompactionArtifactLineageMetadataBySession(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListCompactionArtifactLineageMetadataBySession() error = %v", err)
	}
	validated := make(map[pgtype.UUID]bool, len(metadata))
	for _, artifact := range metadata {
		validated[artifact.ID] = artifact.LineageValidated
	}
	if !validated[directID] || validated[legacyID] {
		t.Fatalf("metadata validation provenance = direct:%v legacy:%v", validated[directID], validated[legacyID])
	}
}

func assertCompactionValidationProvenance(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	compactID pgtype.UUID,
	want bool,
) {
	t.Helper()
	var got bool
	if err := pool.QueryRow(ctx, `
SELECT EXISTS (
  SELECT 1
  FROM bot_history_message_compact_validations
  WHERE compact_id = $1
)
`, compactID).Scan(&got); err != nil {
		t.Fatalf("read validation provenance for %s: %v", compactID, err)
	}
	if got != want {
		t.Fatalf("validation provenance for %s = %v, want %v", compactID, got, want)
	}
}
