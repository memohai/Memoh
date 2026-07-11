-- 0108_compaction_claim_finalization
-- Remove finalized message ownership guards.

DROP TRIGGER IF EXISTS compaction_message_claim_finalize
  ON bot_history_message_compacts;
DROP TRIGGER IF EXISTS compaction_message_claim_insert_guard
  ON bot_history_messages;
DROP TRIGGER IF EXISTS compaction_message_claim_guard
  ON bot_history_messages;

DROP FUNCTION IF EXISTS finalize_compaction_message_claims();
DROP FUNCTION IF EXISTS guard_compaction_message_claim();

DROP TRIGGER IF EXISTS compaction_log_terminal_artifact_guard
  ON bot_history_message_compacts;

ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS compact_claim_finalized_requires_owner,
  DROP COLUMN IF EXISTS compact_claim_finalized;

CREATE OR REPLACE FUNCTION guard_compaction_log_terminal_status()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF OLD.status IN ('ok', 'error')
     AND NEW.status IS DISTINCT FROM OLD.status THEN
    RAISE EXCEPTION 'compaction attempt % is already terminal with status %', OLD.id, OLD.status
      USING ERRCODE = '23514';
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS compaction_log_terminal_status_guard
  ON bot_history_message_compacts;
CREATE TRIGGER compaction_log_terminal_status_guard
BEFORE UPDATE OF status ON bot_history_message_compacts
FOR EACH ROW
EXECUTE FUNCTION guard_compaction_log_terminal_status();
