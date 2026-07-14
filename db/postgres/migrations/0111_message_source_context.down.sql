-- 0111_message_source_context
-- Remove dormant message source-context storage and validation.

ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS history_message_source_context_valid,
  DROP COLUMN IF EXISTS source_context;
