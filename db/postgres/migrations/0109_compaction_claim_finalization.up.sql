-- 0109_compaction_claim_finalization
-- Finalize message ownership when a direct compaction artifact succeeds.

LOCK TABLE bot_history_message_compacts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS compact_claim_finalized BOOLEAN NOT NULL DEFAULT false;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'compact_claim_finalized_requires_owner'
      AND conrelid = 'bot_history_messages'::regclass
  ) THEN
    ALTER TABLE bot_history_messages
      ADD CONSTRAINT compact_claim_finalized_requires_owner
      CHECK (NOT compact_claim_finalized OR compact_id IS NOT NULL);
  END IF;
END $$;

DROP TRIGGER IF EXISTS compaction_message_claim_insert_guard
  ON bot_history_messages;
DROP TRIGGER IF EXISTS compaction_message_claim_guard
  ON bot_history_messages;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM bot_history_message_compacts compact
    WHERE compact.status = 'ok'
      AND (
        compact.artifact_level < 0
        OR jsonb_typeof(compact.coverage) IS DISTINCT FROM 'array'
        OR (
          compact.artifact_level = 0
          AND cardinality(compact.parent_ids) > 0
        )
        OR (
          compact.artifact_level > 0
          AND (
            cardinality(compact.parent_ids) = 0
            OR compact.message_count <= 0
            OR jsonb_array_length(compact.coverage) = 0
            OR EXISTS (
              SELECT 1
              FROM bot_history_messages message
              WHERE message.compact_id = compact.id
            )
          )
        )
      )
  ) THEN
    RAISE EXCEPTION 'existing successful compaction artifacts violate claim finalization shape'
      USING ERRCODE = '23514';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM bot_history_message_compacts compact
    CROSS JOIN LATERAL (
      SELECT COALESCE(array_agg(message.id ORDER BY message.id), ARRAY[]::UUID[]) AS ids
      FROM bot_history_messages message
      WHERE message.compact_id = compact.id
    ) claimed
    CROSS JOIN LATERAL (
      SELECT COALESCE(array_agg(expected.id ORDER BY expected.id), ARRAY[]::UUID[]) AS ids
      FROM (
        SELECT (covered.source->'ref'->>'id')::UUID AS id
        FROM jsonb_array_elements(compact.coverage) AS covered(source)
      ) expected
    ) covered
    WHERE compact.status = 'ok'
      AND compact.artifact_level = 0
      AND (
        compact.message_count <= 0
        OR cardinality(claimed.ids) <> compact.message_count
        OR (
          jsonb_array_length(compact.coverage) > 0
          AND (
            cardinality(covered.ids) <> compact.message_count
            OR claimed.ids IS DISTINCT FROM covered.ids
          )
        )
      )
  ) THEN
    RAISE EXCEPTION 'existing successful direct compaction artifacts violate claim finalization coverage'
      USING ERRCODE = '23514';
  END IF;
END $$;

UPDATE bot_history_messages message
SET compact_claim_finalized = true
FROM bot_history_message_compacts compact
WHERE message.compact_id = compact.id
  AND compact.status = 'ok'
  AND message.compact_claim_finalized = false;

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

CREATE OR REPLACE FUNCTION guard_compaction_log_terminal_status()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF OLD.status IN ('ok', 'error')
     AND (
       NEW.status IS DISTINCT FROM OLD.status
       OR NEW.summary IS DISTINCT FROM OLD.summary
       OR NEW.message_count IS DISTINCT FROM OLD.message_count
       OR NEW.error_message IS DISTINCT FROM OLD.error_message
       OR NEW.usage IS DISTINCT FROM OLD.usage
       OR (
         NEW.model_id IS DISTINCT FROM OLD.model_id
         AND NOT (
           NEW.model_id IS NULL
           AND OLD.model_id IS NOT NULL
           AND NOT EXISTS (
             SELECT 1
             FROM models model
             WHERE model.id = OLD.model_id
           )
         )
       )
       OR NEW.artifact_version IS DISTINCT FROM OLD.artifact_version
       OR NEW.coverage IS DISTINCT FROM OLD.coverage
       OR NEW.anchor_start_ms IS DISTINCT FROM OLD.anchor_start_ms
       OR NEW.anchor_end_ms IS DISTINCT FROM OLD.anchor_end_ms
       OR NEW.artifact_level IS DISTINCT FROM OLD.artifact_level
       OR NEW.parent_ids IS DISTINCT FROM OLD.parent_ids
       OR NEW.started_at IS DISTINCT FROM OLD.started_at
       OR NEW.completed_at IS DISTINCT FROM OLD.completed_at
     ) THEN
    RAISE EXCEPTION 'compaction attempt % has immutable terminal artifact state', OLD.id
      USING ERRCODE = '23514';
  END IF;

  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION finalize_compaction_message_claims()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  claimed_ids UUID[];
  expected_ids UUID[];
BEGIN
  IF NEW.artifact_level > 0 THEN
    IF cardinality(NEW.parent_ids) = 0
       OR NEW.message_count <= 0
       OR jsonb_typeof(NEW.coverage) IS DISTINCT FROM 'array'
       OR jsonb_array_length(NEW.coverage) = 0
       OR EXISTS (
         SELECT 1
         FROM bot_history_messages message
         WHERE message.compact_id = NEW.id
       ) THEN
      RAISE EXCEPTION 'derived compaction artifact % has invalid parents, coverage, or direct message claims', NEW.id
        USING ERRCODE = '23514';
    END IF;

    RETURN NULL;
  END IF;
  IF NEW.artifact_level < 0 THEN
    RAISE EXCEPTION 'compaction attempt % has invalid artifact level %', NEW.id, NEW.artifact_level
      USING ERRCODE = '23514';
  END IF;
  IF cardinality(NEW.parent_ids) > 0 THEN
    RAISE EXCEPTION 'direct compaction attempt % cannot have parent artifacts', NEW.id
      USING ERRCODE = '23514';
  END IF;
  IF NEW.message_count <= 0 THEN
    RAISE EXCEPTION 'direct compaction attempt % has no claimed messages', NEW.id
      USING ERRCODE = '23514';
  END IF;

  SELECT COALESCE(array_agg(claim.id ORDER BY claim.id), ARRAY[]::UUID[])
  INTO claimed_ids
  FROM (
    SELECT message.id
    FROM bot_history_messages message
    WHERE message.compact_id = NEW.id
    ORDER BY message.id
    FOR UPDATE
  ) claim;

  IF jsonb_typeof(NEW.coverage) IS DISTINCT FROM 'array' THEN
    RAISE EXCEPTION 'compaction attempt % has invalid coverage', NEW.id
      USING ERRCODE = '23514';
  ELSIF jsonb_array_length(NEW.coverage) > 0 THEN
    SELECT COALESCE(array_agg(expected.id ORDER BY expected.id), ARRAY[]::UUID[])
    INTO expected_ids
    FROM (
      SELECT (covered.source->'ref'->>'id')::UUID AS id
      FROM jsonb_array_elements(NEW.coverage) AS covered(source)
    ) expected;

    IF cardinality(expected_ids) <> NEW.message_count
       OR claimed_ids IS DISTINCT FROM expected_ids THEN
      RAISE EXCEPTION 'compaction attempt % claim set does not match coverage', NEW.id
        USING ERRCODE = '23514';
    END IF;
  ELSIF cardinality(claimed_ids) <> NEW.message_count THEN
    RAISE EXCEPTION 'legacy compaction attempt % claimed % messages, expected %',
      NEW.id, cardinality(claimed_ids), NEW.message_count
      USING ERRCODE = '23514';
  END IF;

  UPDATE bot_history_messages
  SET compact_claim_finalized = true
  WHERE compact_id = NEW.id
    AND compact_claim_finalized = false;

  RETURN NULL;
END;
$$;

CREATE TRIGGER compaction_message_claim_insert_guard
BEFORE INSERT ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_message_claim();

CREATE TRIGGER compaction_message_claim_guard
BEFORE UPDATE OF compact_id, compact_claim_finalized ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_message_claim();

DROP TRIGGER IF EXISTS compaction_log_terminal_status_guard
  ON bot_history_message_compacts;
CREATE TRIGGER compaction_log_terminal_status_guard
BEFORE UPDATE OF status ON bot_history_message_compacts
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_log_terminal_status();

DROP TRIGGER IF EXISTS compaction_log_terminal_artifact_guard
  ON bot_history_message_compacts;
CREATE TRIGGER compaction_log_terminal_artifact_guard
BEFORE UPDATE OF summary, message_count, error_message, usage, model_id,
  artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
  started_at, completed_at ON bot_history_message_compacts
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_log_terminal_status();

DROP TRIGGER IF EXISTS compaction_message_claim_finalize
  ON bot_history_message_compacts;
CREATE TRIGGER compaction_message_claim_finalize
AFTER UPDATE OF status ON bot_history_message_compacts
FOR EACH ROW
WHEN (OLD.status = 'pending' AND NEW.status = 'ok')
EXECUTE FUNCTION finalize_compaction_message_claims();
