-- 0115_compaction_validation_provenance
-- Identify artifacts produced by strict atomic finalizers.

CREATE TABLE IF NOT EXISTS bot_history_message_compact_validations (
  compact_id UUID PRIMARY KEY REFERENCES bot_history_message_compacts(id) ON DELETE CASCADE
);
