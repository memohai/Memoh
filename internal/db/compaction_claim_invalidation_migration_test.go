package db

import (
	"strings"
	"testing"
)

func TestCompactionClaimInvalidationMigrationContract(t *testing.T) {
	t.Parallel()

	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	baselineDown := readEmbeddedMigration(t, "postgres/migrations/0001_init.down.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0113_compaction_claim_invalidation.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0113_compaction_claim_invalidation.down.sql")

	if !strings.HasPrefix(up, "-- 0113_compaction_claim_invalidation\n-- Invalidate durable compaction claims") ||
		!strings.HasPrefix(down, "-- 0113_compaction_claim_invalidation\n-- Remove compaction claim invalidation") {
		t.Fatal("0112 migration pair is missing the required name and description headers")
	}
	for name, sql := range map[string]string{"baseline": baseline, "up": up} {
		for _, required := range []string{
			"compact_claim_invalidated BOOLEAN NOT NULL DEFAULT false",
			"compact_claim_invalidation_requires_finalized",
			"bot_history_message_compact_claim_validity",
			"message.compact_claim_finalized = true",
			"message.compact_claim_invalidated = false",
			"message.turn_visible = true",
			"invalidate_compaction_claim_on_source_revision",
			"history_message_source_revision_invalidation",
			"NEW.compact_claim_finalized",
			"NEW.compact_id IS NOT DISTINCT FROM OLD.compact_id",
			"NEW.source_revision IS DISTINCT FROM OLD.source_revision",
			"NEW.compact_id IS DISTINCT FROM OLD.compact_id",
			"BEFORE UPDATE OF compact_id, compact_claim_finalized, compact_claim_invalidated",
			"old_claim_current",
			"NEW.compact_claim_invalidated := false",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s schema is missing claim invalidation contract %q", name, required)
			}
		}
	}
	if "history_message_source_revision_bump" >= "history_message_source_revision_invalidation" {
		t.Fatal("trigger names do not guarantee revision bump before claim invalidation")
	}
	for name, sql := range map[string]string{"baseline down": baselineDown, "0112 down": down} {
		for _, required := range []string{
			"DROP VIEW IF EXISTS bot_history_message_compact_claim_validity",
			"DROP TRIGGER IF EXISTS history_message_source_revision_invalidation",
			"DROP FUNCTION IF EXISTS invalidate_compaction_claim_on_source_revision()",
			"DROP CONSTRAINT IF EXISTS compact_claim_invalidation_requires_finalized",
			"DROP COLUMN IF EXISTS compact_claim_invalidated",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s does not reverse claim invalidation object %q", name, required)
			}
		}
	}
	for _, required := range []string{
		"WITH RECURSIVE invalid(id)",
		"invalid_compaction_artifacts",
		"SET compact_id = NULL",
		"DELETE FROM bot_history_message_compact_parent_edges",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0112 down migration does not retire invalid lineage via %q", required)
		}
	}
}
