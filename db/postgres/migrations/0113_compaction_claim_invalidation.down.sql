-- 0113_compaction_claim_invalidation
-- Remove compaction claim invalidation after retiring known-stale lineages.

LOCK TABLE bot_history_message_compacts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

CREATE TEMPORARY TABLE invalid_compaction_artifacts ON COMMIT DROP AS
WITH RECURSIVE invalid(id) AS (
  SELECT validity.compact_id
  FROM bot_history_message_compact_claim_validity validity
  WHERE NOT validity.sources_current
  UNION
  SELECT edge.artifact_id
  FROM bot_history_message_compact_parent_edges edge
  JOIN invalid parent ON parent.id = edge.parent_id
)
SELECT DISTINCT id FROM invalid;

UPDATE bot_history_message_compacts compact
SET superseded_by = NULL,
    superseded_at = NULL
WHERE compact.id NOT IN (SELECT id FROM invalid_compaction_artifacts)
  AND compact.superseded_by IN (SELECT id FROM invalid_compaction_artifacts);

UPDATE bot_history_messages message
SET compact_id = NULL,
    compact_claim_finalized = false,
    compact_claim_invalidated = false
WHERE message.compact_id IN (SELECT id FROM invalid_compaction_artifacts);

DELETE FROM bot_history_message_compact_parent_edges edge
WHERE edge.artifact_id IN (SELECT id FROM invalid_compaction_artifacts)
   OR edge.parent_id IN (SELECT id FROM invalid_compaction_artifacts);

DELETE FROM bot_history_message_compacts compact
WHERE compact.id IN (SELECT id FROM invalid_compaction_artifacts);

DROP VIEW IF EXISTS bot_history_message_compact_claim_validity;

DROP TRIGGER IF EXISTS history_message_source_revision_invalidation
  ON bot_history_messages;
DROP FUNCTION IF EXISTS invalidate_compaction_claim_on_source_revision();

CREATE OR REPLACE FUNCTION guard_compaction_message_claim()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  target_status TEXT;
  target_level INTEGER;
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.compact_id IS NOT NULL OR NEW.compact_claim_finalized THEN
      RAISE EXCEPTION 'new message % cannot start with a compaction claim', NEW.id
        USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
  END IF;

  IF NEW.compact_id IS NULL THEN
    IF OLD.compact_claim_finalized
       AND EXISTS (
         SELECT 1
         FROM bot_history_message_compacts compact
         WHERE compact.id = OLD.compact_id
       ) THEN
      RAISE EXCEPTION 'message % has a finalized compaction claim owned by %', OLD.id, OLD.compact_id
        USING ERRCODE = '23514';
    END IF;

    NEW.compact_claim_finalized := false;
    RETURN NEW;
  END IF;

  IF OLD.compact_claim_finalized
     AND (
       NEW.compact_id IS DISTINCT FROM OLD.compact_id
       OR NEW.compact_claim_finalized IS DISTINCT FROM true
     ) THEN
    RAISE EXCEPTION 'message % has a finalized compaction claim owned by %', OLD.id, OLD.compact_id
      USING ERRCODE = '23514';
  END IF;

  IF NOT OLD.compact_claim_finalized
     AND NEW.compact_claim_finalized THEN
    SELECT compact.status, compact.artifact_level
    INTO target_status, target_level
    FROM bot_history_message_compacts compact
    WHERE compact.id = NEW.compact_id;

    IF NOT FOUND OR target_status <> 'ok' OR target_level <> 0 THEN
      RAISE EXCEPTION 'message % compaction claim can only be finalized with its successful direct artifact', OLD.id
        USING ERRCODE = '23514';
    END IF;
  END IF;

  IF OLD.compact_id IS DISTINCT FROM NEW.compact_id THEN
    SELECT compact.status, compact.artifact_level
    INTO target_status, target_level
    FROM bot_history_message_compacts compact
    WHERE compact.id = NEW.compact_id
    FOR SHARE NOWAIT;

    IF NOT FOUND OR target_status <> 'pending' OR target_level <> 0 THEN
      RAISE EXCEPTION 'message % cannot claim terminal or derived compaction attempt %', OLD.id, NEW.compact_id
        USING ERRCODE = '23514';
    END IF;

    NEW.compact_claim_finalized := false;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS compaction_message_claim_guard
  ON bot_history_messages;
CREATE TRIGGER compaction_message_claim_guard
BEFORE UPDATE OF compact_id, compact_claim_finalized ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_message_claim();

ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS compact_claim_invalidation_requires_finalized,
  DROP COLUMN IF EXISTS compact_claim_invalidated;
