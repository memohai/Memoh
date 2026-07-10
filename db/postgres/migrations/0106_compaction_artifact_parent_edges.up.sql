-- 0106_compaction_artifact_parent_edges
-- Normalize compaction artifact parent edges and enforce durable lineage.

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

CREATE INDEX IF NOT EXISTS idx_compacts_session_lineage
  ON bot_history_message_compacts(session_id, status, superseded_by);

CREATE TABLE IF NOT EXISTS bot_history_message_compact_parent_edges (
  artifact_id UUID NOT NULL,
  parent_id UUID NOT NULL,
  ordinal INTEGER NOT NULL,
  CONSTRAINT compaction_artifact_parent_edges_pkey PRIMARY KEY (artifact_id, parent_id),
  CONSTRAINT compaction_artifact_parent_edges_ordinal_key UNIQUE (artifact_id, ordinal),
  CONSTRAINT compaction_artifact_parent_edges_ordinal_check CHECK (ordinal >= 0),
  CONSTRAINT compaction_artifact_parent_edges_not_self_check CHECK (artifact_id <> parent_id),
  CONSTRAINT compaction_artifact_parent_edges_artifact_fkey
    FOREIGN KEY (artifact_id) REFERENCES bot_history_message_compacts(id) ON DELETE CASCADE,
  CONSTRAINT compaction_artifact_parent_edges_parent_fkey
    FOREIGN KEY (parent_id) REFERENCES bot_history_message_compacts(id)
);

CREATE OR REPLACE FUNCTION sync_compaction_artifact_parent_edges()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM bot_history_message_compact_parent_edges
  WHERE artifact_id = NEW.id;

  INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
  SELECT NEW.id, parent.parent_id, (parent.ordinality - 1)::INTEGER
  FROM unnest(NEW.parent_ids) WITH ORDINALITY AS parent(parent_id, ordinality);

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS compaction_artifact_parent_edges_sync
  ON bot_history_message_compacts;

CREATE TRIGGER compaction_artifact_parent_edges_sync
AFTER INSERT OR UPDATE OF parent_ids ON bot_history_message_compacts
FOR EACH ROW
EXECUTE FUNCTION sync_compaction_artifact_parent_edges();

DELETE FROM bot_history_message_compact_parent_edges;

INSERT INTO bot_history_message_compact_parent_edges (artifact_id, parent_id, ordinal)
SELECT compact.id, parent.parent_id, (parent.ordinality - 1)::INTEGER
FROM bot_history_message_compacts AS compact
CROSS JOIN LATERAL unnest(compact.parent_ids) WITH ORDINALITY AS parent(parent_id, ordinality);
