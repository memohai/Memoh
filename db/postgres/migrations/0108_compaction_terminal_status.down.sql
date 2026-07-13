-- 0108_compaction_terminal_status
-- Remove the compaction terminal status guard.

DROP TRIGGER IF EXISTS compaction_log_terminal_status_guard
  ON bot_history_message_compacts;
DROP FUNCTION IF EXISTS guard_compaction_log_terminal_status();
