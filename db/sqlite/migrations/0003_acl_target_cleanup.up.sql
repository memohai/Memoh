-- 0003_acl_target_cleanup
-- Remove redundant ACL target kind and ordering fields.

PRAGMA foreign_keys = OFF;

ALTER TABLE bot_acl_rules RENAME TO bot_acl_rules_old;

CREATE TABLE bot_acl_rules (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  enabled INTEGER NOT NULL DEFAULT 1,
  description TEXT,
  action TEXT NOT NULL,
  effect TEXT NOT NULL,
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
  CONSTRAINT bot_acl_rules_target_check CHECK (
    subject_channel_type IS NULL OR length(trim(subject_channel_type)) > 0
  ),
  CONSTRAINT bot_acl_rules_source_conversation_type_check CHECK (
    source_conversation_type IS NULL OR source_conversation_type IN ('private', 'group', 'thread')
  ),
  CONSTRAINT bot_acl_rules_source_scope_check CHECK (
    (source_conversation_id IS NULL AND source_thread_id IS NULL)
    OR source_channel IS NOT NULL
  ),
  CONSTRAINT bot_acl_rules_source_thread_check CHECK (
    source_thread_id IS NULL OR source_conversation_id IS NOT NULL
  )
);

INSERT INTO bot_acl_rules (
  id, bot_id, enabled, description, action, effect,
  channel_identity_id, subject_channel_type,
  source_channel, source_conversation_type, source_conversation_id, source_thread_id,
  created_by_user_id, created_at, updated_at
)
SELECT
  id, bot_id, enabled, description, action, effect,
  channel_identity_id, subject_channel_type,
  source_channel, source_conversation_type, source_conversation_id, source_thread_id,
  created_by_user_id, created_at, updated_at
FROM bot_acl_rules_old;

DROP TABLE bot_acl_rules_old;

CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_bot_id ON bot_acl_rules(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_channel_identity_id ON bot_acl_rules(channel_identity_id);
CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_subject_channel_type ON bot_acl_rules(subject_channel_type);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_acl_rules_unique_target ON bot_acl_rules(
  bot_id,
  action,
  effect,
  COALESCE(channel_identity_id, ''),
  COALESCE(subject_channel_type, ''),
  COALESCE(source_conversation_type, ''),
  COALESCE(source_conversation_id, ''),
  COALESCE(source_thread_id, '')
);

PRAGMA foreign_keys = ON;
