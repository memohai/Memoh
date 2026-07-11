-- 0107_compaction_terminal_status
-- Protect terminal compaction attempts from late status transitions.

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
