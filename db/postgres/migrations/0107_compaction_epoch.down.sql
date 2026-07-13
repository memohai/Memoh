DROP INDEX IF EXISTS idx_compacts_owner_epoch;

ALTER TABLE bot_history_message_compacts
  DROP COLUMN IF EXISTS compaction_epoch;

ALTER TABLE bot_sessions
  DROP COLUMN IF EXISTS compaction_epoch;
