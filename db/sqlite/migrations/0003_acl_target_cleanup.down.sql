-- 0003_acl_target_cleanup
-- Restore ACL target kind and ordering fields.

PRAGMA foreign_keys = OFF;

DROP INDEX IF EXISTS idx_bot_acl_rules_unique_target;

ALTER TABLE bot_acl_rules RENAME TO bot_acl_rules_old;

CREATE TABLE bot_acl_rules (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  priority INTEGER NOT NULL DEFAULT 0,
  enabled INTEGER NOT NULL DEFAULT 1,
  description TEXT,
  action TEXT NOT NULL,
  effect TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE CASCADE,
  subject_channel_type TEXT,
  source_channel TEXT,
  source_conversation_type TEXT,
  source_conversation_id TEXT,
  source_thread_id TEXT,
  created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_acl_rules_action_check CHECK (action IN ('chat.trigger')),
  CONSTRAINT bot_acl_rules_effect_check CHECK (effect IN ('allow', 'deny')),
  CONSTRAINT bot_acl_rules_subject_kind_check CHECK (subject_kind IN ('all', 'channel_identity', 'channel_type')),
  CONSTRAINT bot_acl_rules_source_conversation_type_check CHECK (
    source_conversation_type IS NULL OR source_conversation_type IN ('private', 'group', 'thread')
  ),
  CONSTRAINT bot_acl_rules_source_scope_check CHECK (
    (source_conversation_id IS NULL AND source_thread_id IS NULL)
    OR source_channel IS NOT NULL
  ),
  CONSTRAINT bot_acl_rules_source_thread_check CHECK (
    source_thread_id IS NULL OR source_conversation_id IS NOT NULL
  ),
  CONSTRAINT bot_acl_rules_subject_value_check CHECK (
    (subject_kind = 'all' AND channel_identity_id IS NULL AND subject_channel_type IS NULL) OR
    (subject_kind = 'channel_identity' AND channel_identity_id IS NOT NULL AND subject_channel_type IS NULL) OR
    (subject_kind = 'channel_type' AND channel_identity_id IS NULL AND subject_channel_type IS NOT NULL)
  )
);

INSERT INTO bot_acl_rules (
  id, bot_id, priority, enabled, description, action, effect, subject_kind,
  channel_identity_id, subject_channel_type,
  source_channel, source_conversation_type, source_conversation_id, source_thread_id,
  created_by_user_id, created_at, updated_at
)
SELECT
  id,
  bot_id,
  0,
  enabled,
  description,
  action,
  effect,
  CASE
    WHEN channel_identity_id IS NOT NULL THEN 'channel_identity'
    WHEN subject_channel_type IS NOT NULL THEN 'channel_type'
    ELSE 'all'
  END,
  channel_identity_id,
  CASE WHEN channel_identity_id IS NOT NULL THEN NULL ELSE subject_channel_type END,
  source_channel,
  source_conversation_type,
  source_conversation_id,
  source_thread_id,
  created_by_user_id,
  created_at,
  updated_at
FROM bot_acl_rules_old;

DROP TABLE bot_acl_rules_old;

CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_bot_id ON bot_acl_rules(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_channel_identity_id ON bot_acl_rules(channel_identity_id);
CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_subject_channel_type ON bot_acl_rules(subject_channel_type);

PRAGMA foreign_keys = ON;
