-- 0005_sync_overlay_display_browser_gateway
-- Sync SQLite schema with PostgreSQL 0075 overlay providers, 0077 bot display,
-- and 0078 browser gateway removal.

PRAGMA foreign_keys = OFF;

-- SQLite does not support ADD COLUMN IF NOT EXISTS. Patch the schema cookie
-- first so older databases can expose missing columns before the table rebuild
-- below copies data into the final shape.
PRAGMA writable_schema = ON;

UPDATE sqlite_schema
SET sql = replace(
  sql,
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bots_type_check',
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  overlay_provider TEXT NOT NULL DEFAULT '''',
  overlay_enabled INTEGER NOT NULL DEFAULT 0,
  overlay_config TEXT NOT NULL DEFAULT ''{}'',
  display_enabled INTEGER NOT NULL DEFAULT 0,
  CONSTRAINT bots_type_check'
)
WHERE type = 'table'
  AND name = 'bots'
  AND sql NOT LIKE '%overlay_provider%'
  AND sql NOT LIKE '%display_enabled%';

UPDATE sqlite_schema
SET sql = replace(
  sql,
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bots_type_check',
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  overlay_provider TEXT NOT NULL DEFAULT '''',
  overlay_enabled INTEGER NOT NULL DEFAULT 0,
  overlay_config TEXT NOT NULL DEFAULT ''{}'',
  CONSTRAINT bots_type_check'
)
WHERE type = 'table'
  AND name = 'bots'
  AND sql NOT LIKE '%overlay_provider%'
  AND sql LIKE '%display_enabled%';

UPDATE sqlite_schema
SET sql = replace(
  sql,
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bots_type_check',
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  display_enabled INTEGER NOT NULL DEFAULT 0,
  CONSTRAINT bots_type_check'
)
WHERE type = 'table'
  AND name = 'bots'
  AND sql LIKE '%overlay_provider%'
  AND sql NOT LIKE '%display_enabled%';

PRAGMA writable_schema = OFF;
PRAGMA schema_version = 1000005;

ALTER TABLE bots RENAME TO bots_old;

CREATE TABLE bots (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  timezone TEXT,
  is_active INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'ready',
  acl_default_effect TEXT NOT NULL DEFAULT 'allow',
  language TEXT NOT NULL DEFAULT 'auto',
  reasoning_enabled INTEGER NOT NULL DEFAULT 0,
  reasoning_effort TEXT NOT NULL DEFAULT 'medium',
  chat_model_id TEXT REFERENCES models(id) ON DELETE SET NULL,
  search_provider_id TEXT REFERENCES search_providers(id) ON DELETE SET NULL,
  memory_provider_id TEXT REFERENCES memory_providers(id) ON DELETE SET NULL,
  heartbeat_enabled INTEGER NOT NULL DEFAULT 0,
  heartbeat_interval INTEGER NOT NULL DEFAULT 30,
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
  CONSTRAINT bots_reasoning_effort_check CHECK (reasoning_effort IN ('low', 'medium', 'high'))
);

INSERT INTO bots (
  id, owner_user_id, type, display_name, avatar_url, timezone, is_active, status,
  acl_default_effect, language, reasoning_enabled, reasoning_effort,
  chat_model_id, search_provider_id, memory_provider_id,
  heartbeat_enabled, heartbeat_interval, heartbeat_prompt, heartbeat_model_id,
  compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id,
  title_model_id, image_model_id, discuss_probe_model_id, tts_model_id,
  transcription_model_id, persist_full_tool_results, show_tool_calls_in_im,
  tool_approval_config, display_enabled, overlay_provider, overlay_enabled,
  overlay_config, metadata, created_at, updated_at
)
SELECT
  id, owner_user_id, type, display_name, avatar_url, timezone, is_active, status,
  acl_default_effect, language, reasoning_enabled, reasoning_effort,
  chat_model_id, search_provider_id, memory_provider_id,
  heartbeat_enabled, heartbeat_interval, heartbeat_prompt, heartbeat_model_id,
  compaction_enabled, compaction_threshold, compaction_ratio, compaction_model_id,
  title_model_id, image_model_id, discuss_probe_model_id, tts_model_id,
  transcription_model_id, persist_full_tool_results, show_tool_calls_in_im,
  tool_approval_config, display_enabled, overlay_provider, overlay_enabled,
  overlay_config, metadata, created_at, updated_at
FROM bots_old;

DROP TABLE bots_old;
DROP TABLE IF EXISTS browser_contexts;

CREATE INDEX IF NOT EXISTS idx_bots_owner_user_id ON bots(owner_user_id);

PRAGMA foreign_keys = ON;
