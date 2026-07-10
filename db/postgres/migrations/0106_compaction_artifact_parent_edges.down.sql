-- 0106_compaction_artifact_parent_edges
-- Remove normalized compaction artifact parent edges and synchronization.

DROP TRIGGER IF EXISTS compaction_artifact_parent_edges_sync
  ON bot_history_message_compacts;

DROP FUNCTION IF EXISTS sync_compaction_artifact_parent_edges();

DROP TABLE IF EXISTS bot_history_message_compact_parent_edges;

DROP INDEX IF EXISTS idx_compacts_session_lineage;

ALTER TABLE bot_history_message_compacts
  DROP CONSTRAINT IF EXISTS compacts_not_self_superseded_check,
  DROP CONSTRAINT IF EXISTS compacts_supersession_markers_check,
  DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey;

ALTER TABLE bot_history_message_compacts
  ADD CONSTRAINT bot_history_message_compacts_superseded_by_fkey
  FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL;
