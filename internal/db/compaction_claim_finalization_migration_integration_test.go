package db

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCompactionClaimFinalizationMigrationContract(t *testing.T) {
	t.Parallel()
	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	baselineDown := readEmbeddedMigration(t, "postgres/migrations/0001_init.down.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0108_compaction_claim_finalization.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0108_compaction_claim_finalization.down.sql")

	if !strings.HasPrefix(up, "-- 0108_compaction_claim_finalization\n-- Finalize message ownership") ||
		!strings.HasPrefix(down, "-- 0108_compaction_claim_finalization\n-- Remove finalized message ownership") {
		t.Fatal("0108 migration pair is missing the required name and description headers")
	}
	for name, sql := range map[string]string{"baseline": baseline, "up": up} {
		for _, required := range []string{
			"compact_claim_finalized BOOLEAN NOT NULL DEFAULT false",
			"compact_claim_finalized_requires_owner",
			"guard_compaction_message_claim",
			"finalize_compaction_message_claims",
			"compaction_message_claim_guard",
			"compaction_message_claim_finalize",
			"BEFORE UPDATE OF compact_id, compact_claim_finalized",
			"AFTER UPDATE OF status ON bot_history_message_compacts",
			"OLD.status = 'pending' AND NEW.status = 'ok'",
			"FOR UPDATE",
			"NEW.message_count",
			"derived compaction artifact",
			"jsonb_typeof(NEW.coverage) IS DISTINCT FROM 'array'",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s schema is missing claim finalization contract %q", name, required)
			}
		}
	}
	for _, required := range []string{
		"existing successful compaction artifacts violate claim finalization shape",
		"existing successful direct compaction artifacts violate claim finalization coverage",
		"claimed.ids IS DISTINCT FROM covered.ids",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0108 migration is missing historical claim audit %q", required)
		}
	}
	for name, sql := range map[string]string{"baseline down": baselineDown, "0108 down": down} {
		for _, required := range []string{
			"DROP TRIGGER IF EXISTS compaction_message_claim_finalize",
			"DROP TRIGGER IF EXISTS compaction_message_claim_guard",
			"DROP FUNCTION IF EXISTS finalize_compaction_message_claims()",
			"DROP FUNCTION IF EXISTS guard_compaction_message_claim()",
			"DROP CONSTRAINT IF EXISTS compact_claim_finalized_requires_owner",
			"DROP COLUMN IF EXISTS compact_claim_finalized",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s does not reverse claim finalization object %q", name, required)
			}
		}
	}
}

func TestFinalizeCompactionArtifactClaimMigrationPostgresPath(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	schema := "compaction_claim_finalization_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+quotedSchema); err != nil {
		t.Fatalf("set search path: %v", err)
	}
	if _, err := tx.Exec(ctx, `
CREATE TABLE bot_history_message_compacts (
  id UUID PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ok', 'error')),
  message_count INTEGER NOT NULL DEFAULT 0,
  coverage JSONB NOT NULL DEFAULT '[]'::jsonb,
  artifact_level INTEGER NOT NULL DEFAULT 0,
  parent_ids UUID[] NOT NULL DEFAULT '{}'::uuid[]
);
CREATE TABLE bot_history_messages (
  id UUID PRIMARY KEY,
  compact_id UUID REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL
);
`); err != nil {
		t.Fatalf("create pre-0108 schema: %v", err)
	}

	up := readEmbeddedMigration(t, "postgres/migrations/0108_compaction_claim_finalization.up.sql")
	inconsistentLogID := testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id, status, message_count) VALUES ($1, 'ok', 1)`, inconsistentLogID); err != nil {
		t.Fatalf("insert inconsistent historical compact: %v", err)
	}
	assertPostgresOperationFails(t, ctx, tx, "reject_inconsistent_historical_claim", func() error {
		_, err := tx.Exec(ctx, up)
		return err
	})
	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_message_compacts WHERE id = $1`, inconsistentLogID); err != nil {
		t.Fatalf("remove inconsistent historical compact: %v", err)
	}

	mismatchedLogID, mismatchedClaimID, mismatchedCoverageID := testUUID(), testUUID(), testUUID()
	mismatchedCoverage := finalizeParams(
		mismatchedLogID,
		pgtype.UUID{},
		pgtype.UUID{},
		[]pgtype.UUID{mismatchedCoverageID},
		[]string{"1"},
		"historical summary",
	).Coverage
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id, status, message_count, coverage) VALUES ($1, 'ok', 1, $2)`, mismatchedLogID, mismatchedCoverage); err != nil {
		t.Fatalf("insert mismatched historical compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2), ($3, NULL)`, mismatchedClaimID, mismatchedLogID, mismatchedCoverageID); err != nil {
		t.Fatalf("insert mismatched historical messages: %v", err)
	}
	assertPostgresOperationFails(t, ctx, tx, "reject_mismatched_historical_coverage", func() error {
		_, err := tx.Exec(ctx, up)
		return err
	})
	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_message_compacts WHERE id = $1`, mismatchedLogID); err != nil {
		t.Fatalf("remove mismatched historical compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = ANY($1::uuid[])`, []pgtype.UUID{mismatchedClaimID, mismatchedCoverageID}); err != nil {
		t.Fatalf("remove mismatched historical messages: %v", err)
	}

	preexistingLogID, preexistingMessageID := testUUID(), testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id, status, message_count) VALUES ($1, 'ok', 1)`, preexistingLogID); err != nil {
		t.Fatalf("insert pre-0108 compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2)`, preexistingMessageID, preexistingLogID); err != nil {
		t.Fatalf("insert pre-0108 claim: %v", err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("apply 0108 up: %v", err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatalf("reapply 0108 up: %v", err)
	}
	assertClaimMarker(t, ctx, tx, preexistingMessageID, preexistingLogID, true)

	legacyLogID, legacyMessageID := testUUID(), testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1)`, legacyLogID); err != nil {
		t.Fatalf("insert legacy compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2)`, legacyMessageID, legacyLogID); err != nil {
		t.Fatalf("insert legacy claim: %v", err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE bot_history_message_compacts
SET status = 'ok', message_count = 1
WHERE id = $1
`, legacyLogID); err != nil {
		t.Fatalf("complete legacy claim: %v", err)
	}
	assertClaimMarker(t, ctx, tx, legacyMessageID, legacyLogID, true)

	nextLogID := testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1)`, nextLogID); err != nil {
		t.Fatalf("insert next attempt: %v", err)
	}
	assertPostgresStatementFails(t, ctx, tx, "overwrite_finalized_claim", `
UPDATE bot_history_messages SET compact_id = $1 WHERE id = $2
`, nextLogID, legacyMessageID)
	assertPostgresStatementFails(t, ctx, tx, "clear_finalized_marker", `
UPDATE bot_history_messages SET compact_claim_finalized = false WHERE id = $1
`, legacyMessageID)
	assertPostgresStatementFails(t, ctx, tx, "clear_finalized_owner", `
UPDATE bot_history_messages SET compact_id = NULL WHERE id = $1
`, legacyMessageID)

	markWinnerLogID, markLoserLogID, racedMessageID := testUUID(), testUUID(), testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1), ($2)`, markLoserLogID, markWinnerLogID); err != nil {
		t.Fatalf("insert mark-wins compacts: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2)`, racedMessageID, markLoserLogID); err != nil {
		t.Fatalf("insert mark-wins claim: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET compact_id = $1 WHERE id = $2`, markWinnerLogID, racedMessageID); err != nil {
		t.Fatalf("move pending claim before completion: %v", err)
	}
	assertPostgresStatementFails(t, ctx, tx, "complete_lost_claim", `
UPDATE bot_history_message_compacts SET status = 'ok', message_count = 1 WHERE id = $1
`, markLoserLogID)
	if _, err := tx.Exec(ctx, `UPDATE bot_history_message_compacts SET status = 'ok', message_count = 1 WHERE id = $1`, markWinnerLogID); err != nil {
		t.Fatalf("complete mark winner: %v", err)
	}
	assertClaimMarker(t, ctx, tx, racedMessageID, markWinnerLogID, true)

	wrongCoverageLogID, wrongCoverageMessageID, expectedMessageID := testUUID(), testUUID(), testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1)`, wrongCoverageLogID); err != nil {
		t.Fatalf("insert coverage mismatch compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2), ($3, NULL)`, wrongCoverageMessageID, wrongCoverageLogID, expectedMessageID); err != nil {
		t.Fatalf("insert coverage mismatch messages: %v", err)
	}
	coverage := finalizeParams(
		wrongCoverageLogID,
		pgtype.UUID{},
		pgtype.UUID{},
		[]pgtype.UUID{expectedMessageID},
		[]string{"1"},
		"summary",
	).Coverage
	assertPostgresStatementFails(t, ctx, tx, "complete_wrong_claim_set", `
UPDATE bot_history_message_compacts
SET status = 'ok', message_count = 1, coverage = $2
WHERE id = $1
`, wrongCoverageLogID, coverage)

	invalidCoverageLogID, invalidCoverageMessageID := testUUID(), testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id) VALUES ($1)`, invalidCoverageLogID); err != nil {
		t.Fatalf("insert invalid coverage compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2)`, invalidCoverageMessageID, invalidCoverageLogID); err != nil {
		t.Fatalf("insert invalid coverage claim: %v", err)
	}
	assertPostgresStatementFails(t, ctx, tx, "complete_non_array_coverage", `
UPDATE bot_history_message_compacts
SET status = 'ok', message_count = 1, coverage = '{}'::jsonb
WHERE id = $1
`, invalidCoverageLogID)
	assertClaimMarker(t, ctx, tx, invalidCoverageMessageID, invalidCoverageLogID, false)

	derivedLogID, derivedParentID, derivedMessageID := testUUID(), testUUID(), testUUID()
	derivedCoverage := finalizeParams(
		derivedLogID,
		pgtype.UUID{},
		pgtype.UUID{},
		[]pgtype.UUID{derivedMessageID},
		[]string{"1"},
		"derived summary",
	).Coverage
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id, artifact_level, parent_ids) VALUES ($1, 1, ARRAY[$2]::uuid[])`, derivedLogID, derivedParentID); err != nil {
		t.Fatalf("insert derived compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id) VALUES ($1, $2)`, derivedMessageID, derivedLogID); err != nil {
		t.Fatalf("insert invalid derived direct claim: %v", err)
	}
	assertPostgresStatementFails(t, ctx, tx, "complete_derived_with_direct_claim", `
UPDATE bot_history_message_compacts
SET status = 'ok', message_count = 1, coverage = $2
WHERE id = $1
`, derivedLogID, derivedCoverage)
	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_messages WHERE id = $1`, derivedMessageID); err != nil {
		t.Fatalf("remove invalid derived direct claim: %v", err)
	}
	assertPostgresStatementFails(t, ctx, tx, "complete_derived_with_invalid_coverage", `
UPDATE bot_history_message_compacts
SET status = 'ok', message_count = 1, coverage = '{}'::jsonb
WHERE id = $1
`, derivedLogID)
	if _, err := tx.Exec(ctx, `
UPDATE bot_history_message_compacts
SET status = 'ok', message_count = 1, coverage = $2
WHERE id = $1
`, derivedLogID, derivedCoverage); err != nil {
		t.Fatalf("complete valid derived artifact: %v", err)
	}

	deletableLogID, deletableMessageID := testUUID(), testUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_message_compacts (id, status, message_count) VALUES ($1, 'ok', 1)`, deletableLogID); err != nil {
		t.Fatalf("insert deletable compact: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO bot_history_messages (id, compact_id, compact_claim_finalized) VALUES ($1, $2, true)`, deletableMessageID, deletableLogID); err != nil {
		t.Fatalf("insert deletable finalized claim: %v", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM bot_history_message_compacts WHERE id = $1`, deletableLogID); err != nil {
		t.Fatalf("delete finalized claim owner: %v", err)
	}
	assertClaimMarker(t, ctx, tx, deletableMessageID, pgtype.UUID{}, false)

	down := readEmbeddedMigration(t, "postgres/migrations/0108_compaction_claim_finalization.down.sql")
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("apply 0108 down: %v", err)
	}
	if _, err := tx.Exec(ctx, down); err != nil {
		t.Fatalf("reapply 0108 down: %v", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE bot_history_messages SET compact_id = $1 WHERE id = $2`, nextLogID, legacyMessageID); err != nil {
		t.Fatalf("claim overwrite remained blocked after down: %v", err)
	}
}

func assertClaimMarker(t *testing.T, ctx context.Context, tx pgx.Tx, messageID, compactID pgtype.UUID, finalized bool) {
	t.Helper()
	var gotCompactID pgtype.UUID
	var gotFinalized bool
	if err := tx.QueryRow(ctx, `
SELECT compact_id, compact_claim_finalized
FROM bot_history_messages
WHERE id = $1
`, messageID).Scan(&gotCompactID, &gotFinalized); err != nil {
		t.Fatalf("read claim marker: %v", err)
	}
	if gotCompactID != compactID || gotFinalized != finalized {
		t.Fatalf(
			"message %s claim = (%s, %v), want (%s, %v)",
			messageID,
			gotCompactID,
			gotFinalized,
			compactID,
			finalized,
		)
	}
}
