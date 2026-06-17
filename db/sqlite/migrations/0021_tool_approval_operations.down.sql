-- 0021_tool_approval_operations (down)
-- Restore the previous tool-name constrained approval request shape.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS tool_approval_requests_old (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  tool_input TEXT NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at TEXT,
  CONSTRAINT tool_approval_tool_name_check CHECK (tool_name IN ('write', 'edit', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id)
);

INSERT INTO tool_approval_requests_old (
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id, tool_name, tool_input, short_id, status,
  decision_reason, requested_by_channel_identity_id, decided_by_channel_identity_id,
  requested_message_id, prompt_message_id, prompt_external_message_id,
  source_platform, reply_target, conversation_type, created_at, decided_at
)
SELECT
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id,
  CASE
    WHEN tool_name IN ('write', 'edit', 'exec') THEN tool_name
    WHEN operation = 'exec' THEN 'exec'
    ELSE 'write'
  END,
  tool_input, short_id, status, decision_reason,
  requested_by_channel_identity_id, decided_by_channel_identity_id,
  requested_message_id, prompt_message_id, prompt_external_message_id,
  source_platform, reply_target, conversation_type, created_at, decided_at
FROM tool_approval_requests;

DROP TABLE tool_approval_requests;
ALTER TABLE tool_approval_requests_old RENAME TO tool_approval_requests;

CREATE INDEX IF NOT EXISTS idx_tool_approval_bot_status_created
  ON tool_approval_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_session_status_created
  ON tool_approval_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_prompt_external
  ON tool_approval_requests(prompt_external_message_id)
  WHERE prompt_external_message_id != '';

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
  fetch_provider_id TEXT REFERENCES fetch_providers(id) ON DELETE SET NULL,
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
  CONSTRAINT bots_name_format_check CHECK (
    name GLOB '[a-z0-9]*'
    AND name NOT GLOB '*[^a-z0-9-]*'
    AND length(name) BETWEEN 2 AND 63
  )
);

INSERT INTO bots_new (
  id, owner_user_id, type, name, display_name, avatar_url, timezone, is_active, status,
  acl_default_effect, language, command_ui_language, reasoning_enabled, reasoning_effort,
  chat_model_id, search_provider_id, fetch_provider_id, memory_provider_id,
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
  chat_model_id, search_provider_id, fetch_provider_id, memory_provider_id,
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

UPDATE bots
SET tool_approval_config = CASE
  WHEN json_valid(tool_approval_config) THEN json_remove(
    json_set(
      tool_approval_config,
      '$.edit',
      json(CASE
        WHEN json_type(tool_approval_config, '$.write') = 'object'
          THEN json_extract(tool_approval_config, '$.write')
        ELSE '{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]}'
      END)
    ),
    '$.read'
  )
  ELSE '{"enabled":false,"write":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"edit":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"exec":{"require_approval":false,"bypass_commands":[],"force_review_commands":[]}}'
END;

PRAGMA foreign_keys = ON;
