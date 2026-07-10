-- 0105_compaction_artifacts
-- Persist summary artifact coverage, anchors, and frontier lineage.

ALTER TABLE bot_history_message_compacts
  ADD COLUMN IF NOT EXISTS artifact_version INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS coverage JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS anchor_start_ms BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS anchor_end_ms BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS artifact_level INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS parent_ids UUID[] NOT NULL DEFAULT '{}'::uuid[],
  ADD COLUMN IF NOT EXISTS superseded_by UUID,
  ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ;

ALTER TABLE bot_history_message_compacts
  DROP CONSTRAINT IF EXISTS bot_history_message_compacts_superseded_by_fkey;

ALTER TABLE bot_history_message_compacts
  ADD CONSTRAINT bot_history_message_compacts_superseded_by_fkey
  FOREIGN KEY (superseded_by) REFERENCES bot_history_message_compacts(id);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'compacts_supersession_markers_check'
      AND conrelid = 'bot_history_message_compacts'::regclass
  ) THEN
    ALTER TABLE bot_history_message_compacts
      ADD CONSTRAINT compacts_supersession_markers_check
      CHECK ((superseded_by IS NULL) = (superseded_at IS NULL));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'compacts_not_self_superseded_check'
      AND conrelid = 'bot_history_message_compacts'::regclass
  ) THEN
    ALTER TABLE bot_history_message_compacts
      ADD CONSTRAINT compacts_not_self_superseded_check
      CHECK (superseded_by IS NULL OR superseded_by <> id);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_compacts_active_session
  ON bot_history_message_compacts(session_id, anchor_start_ms, started_at)
  WHERE status = 'ok' AND superseded_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_compacts_session_lineage
  ON bot_history_message_compacts(session_id, status, superseded_by);
