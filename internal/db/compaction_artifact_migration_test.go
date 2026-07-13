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

func TestCompactionArtifactCanonicalSchemaMatchesPublished0106(t *testing.T) {
	t.Parallel()

	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")

	for _, required := range []string{
		"superseded_by UUID REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL",
		"CREATE INDEX IF NOT EXISTS idx_compacts_active_session",
	} {
		if !strings.Contains(baseline, required) {
			t.Fatalf("canonical 0001 schema is missing published 0106 object %q", required)
		}
	}
	for _, deferred := range []string{
		"bot_history_message_compact_parent_edges",
		"sync_compaction_artifact_parent_edges",
		"compacts_supersession_markers_check",
		"compacts_not_self_superseded_check",
		"idx_compacts_session_lineage",
	} {
		if strings.Contains(baseline, deferred) {
			t.Fatalf("canonical 0001 schema contains %q, which belongs to the artifact-writer PR", deferred)
		}
	}
}

func TestCompactionTerminalStatusMigrationPreservesTerminalAttempts(t *testing.T) {
	t.Parallel()

	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	baselineDown := readEmbeddedMigration(t, "postgres/migrations/0001_init.down.sql")
	up := readEmbeddedMigration(t, "postgres/migrations/0108_compaction_terminal_status.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0108_compaction_terminal_status.down.sql")

	if !strings.HasPrefix(up, "-- 0108_compaction_terminal_status\n-- Protect terminal compaction attempts") ||
		!strings.HasPrefix(down, "-- 0108_compaction_terminal_status\n-- Remove the compaction terminal status guard") {
		t.Fatal("0107 migration pair is missing the required name and description headers")
	}
	for name, sql := range map[string]string{"baseline": baseline, "up": up} {
		for _, required := range []string{
			"guard_compaction_log_terminal_status",
			"OLD.status IN ('ok', 'error')",
			"NEW.status IS DISTINCT FROM OLD.status",
			"compaction_log_terminal_status_guard",
			"BEFORE UPDATE OF status ON bot_history_message_compacts",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s schema is missing compaction finalize guard %q", name, required)
			}
		}
	}
	for name, sql := range map[string]string{"baseline down": baselineDown, "0107 down": down} {
		for _, required := range []string{
			"DROP TRIGGER IF EXISTS compaction_log_terminal_status_guard",
			"DROP FUNCTION IF EXISTS guard_compaction_log_terminal_status()",
		} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s does not reverse compaction finalize guard %q", name, required)
			}
		}
	}
}
