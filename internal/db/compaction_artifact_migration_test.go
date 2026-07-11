package db

import (
	"strings"
	"testing"
)

func TestCompactionArtifactMigrationPreservesPublished0106Schema(t *testing.T) {
	t.Parallel()

	up := readEmbeddedMigration(t, "postgres/migrations/0106_compaction_artifacts.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0106_compaction_artifacts.down.sql")

	if !strings.HasPrefix(up, "-- 0106_compaction_artifacts\n-- Persist summary artifact") ||
		!strings.HasPrefix(down, "-- 0106_compaction_artifacts\n-- Remove summary artifact") {
		t.Fatal("0106 migration pair is missing the required name and description headers")
	}
	if !strings.Contains(up, "superseded_by UUID REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL") ||
		!strings.Contains(up, "CREATE INDEX IF NOT EXISTS idx_compacts_active_session") {
		t.Fatal("0106 up migration no longer matches its published schema")
	}
	for _, repair := range []string{
		"compacts_supersession_markers_check",
		"compacts_not_self_superseded_check",
		"idx_compacts_session_lineage",
		"DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey",
	} {
		if strings.Contains(up, repair) || strings.Contains(down, repair) {
			t.Fatalf("0106 migration pair contains post-publication repair %q", repair)
		}
	}
	for _, removed := range []string{
		"DROP INDEX IF EXISTS idx_compacts_active_session",
		"DROP COLUMN IF EXISTS superseded_at",
		"DROP COLUMN IF EXISTS superseded_by",
		"DROP COLUMN IF EXISTS parent_ids",
		"DROP COLUMN IF EXISTS coverage",
	} {
		if !strings.Contains(down, removed) {
			t.Fatalf("0106 down migration does not reverse published object %q", removed)
		}
	}
}

func TestCompactionArtifactParentEdgeMigrationPreservesDurableLineage(t *testing.T) {
	t.Parallel()

	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	baselineDown := readEmbeddedMigration(t, "postgres/migrations/0001_init.down.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0107_compaction_artifact_parent_edges.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0107_compaction_artifact_parent_edges.down.sql")

	if !strings.HasPrefix(up, "-- 0107_compaction_artifact_parent_edges\n-- Normalize compaction artifact parent edges") ||
		!strings.HasPrefix(down, "-- 0107_compaction_artifact_parent_edges\n-- Remove normalized compaction artifact parent edges") {
		t.Fatal("0106 migration pair is missing the required name and description headers")
	}
	for name, sql := range map[string]string{"baseline": baseline, "up": up} {
		for _, required := range []string{
			"bot_history_message_compacts_superseded_by_fkey",
			"FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id)",
			"compacts_supersession_markers_check",
			"(superseded_by IS NULL) = (superseded_at IS NULL)",
			"compacts_not_self_superseded_check",
			"superseded_by <> id",
			"idx_compacts_session_lineage",
			"session_id, status, superseded_by",
			"bot_history_message_compact_parent_edges",
			"PRIMARY KEY (artifact_id, parent_id)",
			"UNIQUE (artifact_id, ordinal)",
			"CHECK (ordinal >= 0)",
			"CHECK (artifact_id <> parent_id)",
			"FOREIGN KEY (artifact_id) REFERENCES bot_history_message_compacts(id) ON DELETE CASCADE",
			"FOREIGN KEY (parent_id) REFERENCES bot_history_message_compacts(id)",
			"sync_compaction_artifact_parent_edges",
			"AFTER INSERT OR UPDATE OF parent_ids",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s schema is missing normalized parent-edge contract %q", name, required)
			}
		}
	}
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS bot_history_message_compact_parent_edges",
		"DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey",
		"FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id)",
		"compacts_supersession_markers_check",
		"(superseded_by IS NULL) = (superseded_at IS NULL)",
		"compacts_not_self_superseded_check",
		"superseded_by <> id",
		"CREATE INDEX IF NOT EXISTS idx_compacts_session_lineage",
		"CREATE OR REPLACE FUNCTION sync_compaction_artifact_parent_edges()",
		"DROP TRIGGER IF EXISTS compaction_artifact_parent_edges_sync",
		"unnest(compact.parent_ids) WITH ORDINALITY",
	} {
		if !strings.Contains(up, required) {
			t.Fatalf("0106 up migration is missing idempotent sync/backfill contract %q", required)
		}
	}
	for _, required := range []string{
		"DROP TRIGGER IF EXISTS compaction_artifact_parent_edges_sync",
		"DROP FUNCTION IF EXISTS sync_compaction_artifact_parent_edges()",
		"DROP TABLE IF EXISTS bot_history_message_compact_parent_edges",
		"DROP INDEX IF EXISTS idx_compacts_session_lineage",
		"DROP CONSTRAINT IF EXISTS compacts_not_self_superseded_check",
		"DROP CONSTRAINT IF EXISTS compacts_supersession_markers_check",
		"FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL",
	} {
		if !strings.Contains(down, required) {
			t.Fatalf("0106 down migration does not reverse parent-edge object %q", required)
		}
	}
	for _, required := range []string{
		"DROP FUNCTION IF EXISTS sync_compaction_artifact_parent_edges()",
		"DROP TABLE IF EXISTS bot_history_message_compact_parent_edges",
	} {
		if !strings.Contains(baselineDown, required) {
			t.Fatalf("canonical 0001 down migration does not reverse parent-edge object %q", required)
		}
	}
}
