-- 0105_compaction_artifacts
-- Remove summary artifact coverage, anchors, and frontier lineage.

DROP INDEX IF EXISTS idx_compacts_session_lineage;
DROP INDEX IF EXISTS idx_compacts_active_session;

ALTER TABLE bot_history_message_compacts
  DROP CONSTRAINT IF EXISTS compacts_not_self_superseded_check,
  DROP CONSTRAINT IF EXISTS compacts_supersession_markers_check,
  DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey,
  DROP COLUMN IF EXISTS superseded_at,
  DROP COLUMN IF EXISTS superseded_by,
  DROP COLUMN IF EXISTS parent_ids,
  DROP COLUMN IF EXISTS artifact_level,
  DROP COLUMN IF EXISTS anchor_end_ms,
  DROP COLUMN IF EXISTS anchor_start_ms,
  DROP COLUMN IF EXISTS coverage,
  DROP COLUMN IF EXISTS artifact_version;
