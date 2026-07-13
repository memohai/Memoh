-- 0110_compaction_source_revision
-- Remove explicit history source revisions.

DROP TRIGGER IF EXISTS history_message_asset_source_revision_bump
  ON bot_history_message_assets;
DROP FUNCTION IF EXISTS bump_message_source_revision_for_asset();

DROP TRIGGER IF EXISTS history_message_source_revision_bump
  ON bot_history_messages;
DROP FUNCTION IF EXISTS bump_history_message_source_revision();

ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS history_message_source_revision_positive,
  DROP COLUMN IF EXISTS source_revision;
