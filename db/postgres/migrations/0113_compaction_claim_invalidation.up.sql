-- 0113_compaction_claim_invalidation
-- Invalidate durable compaction claims when their source snapshot changes.

LOCK TABLE bot_history_message_compacts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS compact_claim_invalidated BOOLEAN NOT NULL DEFAULT false;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'compact_claim_invalidation_requires_finalized'
      AND conrelid = 'bot_history_messages'::regclass
  ) THEN
    ALTER TABLE bot_history_messages
      ADD CONSTRAINT compact_claim_invalidation_requires_finalized
      CHECK (NOT compact_claim_invalidated OR compact_claim_finalized);
  END IF;
END $$;

CREATE OR REPLACE VIEW bot_history_message_compact_claim_validity AS
SELECT
  compact.id AS compact_id,
  (
    compact.message_count > 0
    AND claims.claimed_count = compact.message_count
    AND claims.current_count = compact.message_count
    AND jsonb_typeof(compact.coverage) = 'array'
    AND (
      coverage.coverage_count = 0
      OR (
        coverage.coverage_count = compact.message_count
        AND coverage.distinct_source_count = compact.message_count
        AND coverage.matched_source_count = compact.message_count
      )
    )
  ) AS sources_current
FROM bot_history_message_compacts compact
CROSS JOIN LATERAL (
  SELECT
    COUNT(*)::integer AS claimed_count,
    COUNT(*) FILTER (WHERE
      message.bot_id = compact.bot_id
      AND message.session_id IS NOT DISTINCT FROM compact.session_id
      AND message.turn_visible = true
      AND message.turn_id IS NOT NULL
      AND message.turn_position IS NOT NULL
      AND message.turn_message_seq IS NOT NULL
      AND (message.metadata->>'trigger_mode' IS NULL OR message.metadata->>'trigger_mode' <> 'passive_sync')
      AND message.compact_claim_finalized = true
      AND message.compact_claim_invalidated = false
    )::integer AS current_count
  FROM bot_history_messages message
  WHERE message.compact_id = compact.id
) claims
CROSS JOIN LATERAL (
  SELECT
    COUNT(*)::integer AS coverage_count,
    COUNT(DISTINCT covered.source->'ref'->>'id')::integer AS distinct_source_count,
    COUNT(*) FILTER (WHERE
      jsonb_typeof(covered.source) = 'object'
      AND jsonb_typeof(covered.source->'ref') = 'object'
      AND jsonb_typeof(covered.source->'ref'->'id') = 'string'
      AND EXISTS (
        SELECT 1
        FROM bot_history_messages message
        WHERE message.compact_id = compact.id
          AND message.id::text = covered.source->'ref'->>'id'
      )
    )::integer AS matched_source_count
  FROM jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(compact.coverage) = 'array' THEN compact.coverage
      ELSE '[]'::jsonb
    END
  ) covered(source)
) coverage
WHERE compact.status = 'ok'
  AND compact.artifact_level = 0;

CREATE OR REPLACE FUNCTION invalidate_compaction_claim_on_source_revision()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF OLD.compact_claim_finalized
     AND NEW.compact_claim_finalized
     AND NEW.compact_id IS NOT DISTINCT FROM OLD.compact_id
     AND NEW.source_revision IS DISTINCT FROM OLD.source_revision THEN
    NEW.compact_claim_invalidated := true;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS history_message_source_revision_invalidation
  ON bot_history_messages;
CREATE TRIGGER history_message_source_revision_invalidation
BEFORE UPDATE ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION invalidate_compaction_claim_on_source_revision();

CREATE OR REPLACE FUNCTION guard_compaction_message_claim()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  target_status TEXT;
  target_level INTEGER;
  old_claim_current BOOLEAN;
BEGIN
  IF TG_OP = 'INSERT' THEN
    IF NEW.compact_id IS NOT NULL
       OR NEW.compact_claim_finalized
       OR NEW.compact_claim_invalidated THEN
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
      SELECT validity.sources_current
      INTO old_claim_current
      FROM bot_history_message_compact_claim_validity validity
      WHERE validity.compact_id = OLD.compact_id;

      IF COALESCE(old_claim_current, true) THEN
        RAISE EXCEPTION 'message % has a current finalized compaction claim owned by %', OLD.id, OLD.compact_id
          USING ERRCODE = '23514';
      END IF;
    END IF;

    NEW.compact_claim_finalized := false;
    NEW.compact_claim_invalidated := false;
    RETURN NEW;
  END IF;

  IF OLD.compact_claim_finalized
     AND NEW.compact_id IS DISTINCT FROM OLD.compact_id THEN
    SELECT validity.sources_current
    INTO old_claim_current
    FROM bot_history_message_compact_claim_validity validity
    WHERE validity.compact_id = OLD.compact_id;

    IF COALESCE(old_claim_current, true) THEN
      RAISE EXCEPTION 'message % has a current finalized compaction claim owned by %', OLD.id, OLD.compact_id
        USING ERRCODE = '23514';
    END IF;
  END IF;

  IF OLD.compact_claim_finalized
     AND NEW.compact_id IS NOT DISTINCT FROM OLD.compact_id
     AND NEW.compact_claim_finalized IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'message % has a finalized compaction claim owned by %', OLD.id, OLD.compact_id
      USING ERRCODE = '23514';
  END IF;

  IF OLD.compact_claim_invalidated
     AND NEW.compact_id IS NOT DISTINCT FROM OLD.compact_id
     AND NEW.compact_claim_invalidated IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'message % has an invalidated compaction claim owned by %', OLD.id, OLD.compact_id
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

    NEW.compact_claim_invalidated := false;
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
    NEW.compact_claim_invalidated := false;
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS compaction_message_claim_guard
  ON bot_history_messages;
CREATE TRIGGER compaction_message_claim_guard
BEFORE UPDATE OF compact_id, compact_claim_finalized, compact_claim_invalidated ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_message_claim();
