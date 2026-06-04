-- 0018_relax_reasoning_effort (down)
-- Restore the previous fixed reasoning effort ladder by rebuilding the table.
-- Rows with newer effort tiers must be reconciled before rolling back.
-- Preserves columns added by earlier migrations, including command_ui_language,
-- and keeps heartbeat_interval DEFAULT 1440.
-- Use the bots_new/copy/drop/rename pattern (as in 0013) rather than renaming
-- bots to bots_old: on SQLite 3.26+, ALTER TABLE ... RENAME rewrites dependent
-- child-table foreign keys to the new name even with foreign_keys=OFF, so
-- renaming the parent first would leave child FKs pointing at the dropped table.

PRAGMA foreign_keys = OFF;

-- Map relaxed tiers back into the old enum before copying into the constrained
-- table, otherwise the INSERT into bots_new would violate bots_reasoning_effort_check:
--   minimal -> low, max -> xhigh, anything else out of range -> medium (default).
UPDATE bots SET reasoning_effort = CASE
  WHEN reasoning_effort = 'minimal' THEN 'low'
  WHEN reasoning_effort = 'max' THEN 'xhigh'
  ELSE 'medium'
END
WHERE reasoning_effort NOT IN ('none', 'low', 'medium', 'high', 'xhigh');

CREATE TABLE bots_new (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  name TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  timezone TEXT,
  is_active INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'ready',
  acl_default_effect TEXT NOT NULL DEFAULT 'allow',
  language TEXT NOT NULL DEFAULT 'auto',
  command_ui_language TEXT NOT NULL DEFAULT 'auto',
  reasoning_enabled INTEGER NOT NULL DEFAULT 0,
  reasoning_effort TEXT NOT NULL DEFAULT 'medium',
  chat_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  search_provider_id TEXT REFERENCES search_providers(id) ON DELETE SET NULL,
  memory_provider_id TEXT REFERENCES memory_providers(id) ON DELETE SET NULL,
  heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
  heartbeat_interval INTEGER NOT NULL DEFAULT 1440,
  heartbeat_prompt TEXT NOT NULL DEFAULT '',
  heartbeat_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  compaction_enabled INTEGER NOT NULL DEFAULT 0,
  compaction_threshold INTEGER NOT NULL DEFAULT 100000,
  compaction_ratio INTEGER NOT NULL DEFAULT 80,
  compaction_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  title_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  image_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  discuss_probe_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  tts_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  transcription_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  persist_full_tool_results INTEGER NOT NULL DEFAULT 0,
  show_tool_calls_in_im INTEGER NOT NULL DEFAULT 0,
  tool_approval_config TEXT NOT NULL DEFAULT '{"enabled":false,"write":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"edit":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"exec":{"require_approval":false,"bypass_commands":[],"force_review_commands":[]}}',
  display_enabled INTEGER NOT NULL DEFAULT 0,
  overlay_provider TEXT NOT NULL DEFAULT '',
  overlay_enabled INTEGER NOT NULL DEFAULT 0,
  overlay_config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bots_type_check CHECK (type IN ('personal', 'public')),
  CONSTRAINT bots_status_check CHECK (status IN ('creating', 'ready', 'deleting')),
  CONSTRAINT bots_acl_default_effect_check CHECK (acl_default_effect IN ('allow', 'deny')),
  CONSTRAINT bots_reasoning_effort_check CHECK (reasoning_effort IN ('none', 'low', 'medium', 'high', 'xhigh')),
  CONSTRAINT bots_name_format_check CHECK (
    name GLOB '[a-z0-9]*'
    AND name NOT GLOB '*[^a-z0-9-]*'
    AND length(name) BETWEEN 2 AND 63
  )
);

INSERT INTO bots_new (
  id, owner_user_id, type, name, display_name, avatar_url, timezone, is_active, status,
  acl_default_effect, language, command_ui_language, reasoning_enabled, reasoning_effort,
  chat_model_id, search_provider_id, memory_provider_id,
  heartbeat_enabled, heartbeat_interval, heartbeat_prompt, heartbeat_model_id,
  compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id,
  title_model_id, image_model_id, discuss_probe_model_id, tts_model_id,
  transcription_model_id, persist_full_tool_results, show_tool_calls_in_im,
  tool_approval_config, display_enabled, overlay_provider, overlay_enabled,
  overlay_config, metadata, created_at, updated_at
)
SELECT
  id, owner_user_id, type, name, display_name, avatar_url, timezone, is_active, status,
  acl_default_effect, language, command_ui_language, reasoning_enabled, reasoning_effort,
  chat_model_id, search_provider_id, memory_provider_id,
  heartbeat_enabled, heartbeat_interval, heartbeat_prompt, heartbeat_model_id,
  compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id,
  title_model_id, image_model_id, discuss_probe_model_id, tts_model_id,
  transcription_model_id, persist_full_tool_results, show_tool_calls_in_im,
  tool_approval_config, display_enabled, overlay_provider, overlay_enabled,
  overlay_config, metadata, created_at, updated_at
FROM bots;

DROP TABLE bots;
ALTER TABLE bots_new RENAME TO bots;

CREATE INDEX IF NOT EXISTS idx_bots_owner_user_id ON bots(owner_user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bots_name ON bots(name);

PRAGMA foreign_keys = ON;
