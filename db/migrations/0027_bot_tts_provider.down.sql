-- 0027_bot_tts_provider (rollback)
-- Drop tts_provider_id column from bots table

ALTER TABLE bots
  DROP COLUMN IF EXISTS tts_provider_id;
