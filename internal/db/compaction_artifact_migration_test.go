package db

import (
	"strings"
	"testing"
)

func TestCompactionArtifactMigrationConstrainsSupersessionLineage(t *testing.T) {
	t.Parallel()

	baselineSQL := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	baseline := compactionTableDefinition(baselineSQL)
	up := readEmbeddedMigration(t, "postgres/migrations/0105_compaction_artifacts.up.sql")
	down := readEmbeddedMigration(t, "postgres/migrations/0105_compaction_artifacts.down.sql")

	for name, sql := range map[string]string{"baseline": baseline, "up": up} {
		if !strings.Contains(sql, "compacts_supersession_markers_check") ||
			!strings.Contains(sql, "(superseded_by IS NULL) = (superseded_at IS NULL)") {
			t.Fatalf("%s schema does not enforce paired supersession markers", name)
		}
		if !strings.Contains(sql, "compacts_not_self_superseded_check") ||
			!strings.Contains(sql, "superseded_by <> id") {
			t.Fatalf("%s schema does not reject self-supersession", name)
		}
		if strings.Contains(sql, "superseded_by UUID REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL") {
			t.Fatalf("%s schema can silently sever durable lineage on delete", name)
		}
		if !strings.Contains(sql, "bot_history_message_compacts_superseded_by_fkey") {
			t.Fatalf("%s schema does not name the durable lineage foreign key", name)
		}
	}
	for name, sql := range map[string]string{"baseline": baselineSQL, "up": up} {
		if !strings.Contains(sql, "idx_compacts_session_lineage") ||
			!strings.Contains(sql, "session_id, status, superseded_by") {
			t.Fatalf("%s schema is missing the session-leading lineage index", name)
		}
	}
	if !strings.Contains(up, "DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey") ||
		!strings.Contains(up, "FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id)") {
		t.Fatal("0105 up migration does not idempotently replace the legacy SET NULL foreign key")
	}
	if !strings.HasPrefix(up, "-- 0105_compaction_artifacts\n-- Persist summary artifact") ||
		!strings.HasPrefix(down, "-- 0105_compaction_artifacts\n-- Remove summary artifact") {
		t.Fatal("0105 migration pair is missing the required name and description headers")
	}
	if !strings.Contains(down, "DROP CONSTRAINT IF EXISTS compacts_not_self_superseded_check") ||
		!strings.Contains(down, "DROP CONSTRAINT IF EXISTS compacts_supersession_markers_check") ||
		!strings.Contains(down, "DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey") ||
		!strings.Contains(down, "DROP INDEX IF EXISTS idx_compacts_session_lineage") {
		t.Fatal("0105 down migration does not reverse supersession constraints")
	}
}

func compactionTableDefinition(sql string) string {
	start := strings.Index(sql, "CREATE TABLE IF NOT EXISTS bot_history_message_compacts")
	if start < 0 {
		return ""
	}
	tail := sql[start:]
	end := strings.Index(tail, "CREATE INDEX")
	if end < 0 {
		return tail
	}
	return tail[:end]
}
