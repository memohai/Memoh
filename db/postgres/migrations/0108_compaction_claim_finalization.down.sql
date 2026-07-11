-- 0108_compaction_claim_finalization
-- Remove finalized message ownership guards.

DROP TRIGGER IF EXISTS compaction_message_claim_finalize
  ON bot_history_message_compacts;
DROP TRIGGER IF EXISTS compaction_message_claim_guard
  ON bot_history_messages;

DROP FUNCTION IF EXISTS finalize_compaction_message_claims();
DROP FUNCTION IF EXISTS guard_compaction_message_claim();

ALTER TABLE bot_history_messages
  DROP CONSTRAINT IF EXISTS compact_claim_finalized_requires_owner,
  DROP COLUMN IF EXISTS compact_claim_finalized;
