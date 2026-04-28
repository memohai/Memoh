CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
    CREATE TYPE user_role AS ENUM ('member', 'admin');
  END IF;
END
$$;

-- users: Memoh user principal
CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username TEXT,
  email TEXT,
  password_hash TEXT,
  role user_role NOT NULL DEFAULT 'member',
  display_name TEXT,
  avatar_url TEXT,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  data_root TEXT,
  last_login_at TIMESTAMPTZ,
  is_active BOOLEAN NOT NULL DEFAULT true,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT users_email_unique UNIQUE (email),
  CONSTRAINT users_username_unique UNIQUE (username)
);

-- channel_identities: unified inbound identity subject
CREATE TABLE IF NOT EXISTS channel_identities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  channel_type TEXT NOT NULL,
  channel_subject_id TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT channel_identities_channel_type_subject_unique UNIQUE (channel_type, channel_subject_id)
);

CREATE INDEX IF NOT EXISTS idx_channel_identities_user_id ON channel_identities(user_id);

-- user_channel_bindings: outbound delivery config
CREATE TABLE IF NOT EXISTS user_channel_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_channel_bindings_unique UNIQUE (user_id, channel_type)
);

CREATE INDEX IF NOT EXISTS idx_user_channel_bindings_user_id ON user_channel_bindings(user_id);

CREATE TABLE IF NOT EXISTS providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  client_type TEXT NOT NULL DEFAULT 'openai-completions',
  icon TEXT,
  enable BOOLEAN NOT NULL DEFAULT true,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT providers_name_unique UNIQUE (name),
  CONSTRAINT providers_client_type_check CHECK (client_type IN (
    'openai-responses',
    'openai-completions',
    'anthropic-messages',
    'google-generative-ai',
    'openai-codex',
    'github-copilot',
    'edge-speech',
    'openai-speech',
    'openai-transcription',
    'openrouter-speech',
    'openrouter-transcription',
    'elevenlabs-speech',
    'elevenlabs-transcription',
    'deepgram-speech',
    'deepgram-transcription',
    'minimax-speech',
    'volcengine-speech',
    'alibabacloud-speech',
    'microsoft-speech',
    'google-speech',
    'google-transcription'
  ))
);

CREATE TABLE IF NOT EXISTS search_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  enable BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT search_providers_name_unique UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS models (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id TEXT NOT NULL,
  name TEXT,
  provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'chat',
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT models_provider_id_model_id_unique UNIQUE (provider_id, model_id),
  CONSTRAINT models_type_check CHECK (type IN ('chat', 'embedding', 'speech', 'transcription'))
);

CREATE TABLE IF NOT EXISTS model_variants (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_uuid UUID NOT NULL REFERENCES models(id) ON DELETE CASCADE,
  variant_id TEXT NOT NULL,
  weight INTEGER NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_model_variants_model_uuid ON model_variants(model_uuid);
CREATE INDEX IF NOT EXISTS idx_model_variants_variant_id ON model_variants(variant_id);

CREATE TABLE IF NOT EXISTS memory_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_providers_name_unique UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS browser_contexts (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name        TEXT NOT NULL DEFAULT '',
  config      JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  timezone TEXT,
  is_active BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'ready',
  language TEXT NOT NULL DEFAULT 'auto',
  reasoning_enabled BOOLEAN NOT NULL DEFAULT false,
  reasoning_effort TEXT NOT NULL DEFAULT 'medium',
  chat_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  search_provider_id UUID REFERENCES search_providers(id) ON DELETE SET NULL,
  memory_provider_id UUID REFERENCES memory_providers(id) ON DELETE SET NULL,
  heartbeat_enabled BOOLEAN NOT NULL DEFAULT false,
  heartbeat_interval INTEGER NOT NULL DEFAULT 30,
  heartbeat_prompt TEXT NOT NULL DEFAULT '',
  heartbeat_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  compaction_enabled BOOLEAN NOT NULL DEFAULT false,
  compaction_threshold INTEGER NOT NULL DEFAULT 100000,
  compaction_ratio INTEGER NOT NULL DEFAULT 80,
  compaction_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  title_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  image_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  discuss_probe_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  tts_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  transcription_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  browser_context_id UUID REFERENCES browser_contexts(id) ON DELETE SET NULL,
  persist_full_tool_results BOOLEAN NOT NULL DEFAULT false,
  show_tool_calls_in_im BOOLEAN NOT NULL DEFAULT false,
  tool_approval_config JSONB NOT NULL DEFAULT '{"enabled":false,"write":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"edit":{"require_approval":true,"bypass_globs":["/data/**","/tmp/**"],"force_review_globs":[]},"exec":{"require_approval":false,"bypass_commands":[],"force_review_commands":[]}}'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bots_type_check CHECK (type IN ('personal', 'public')),
  CONSTRAINT bots_status_check CHECK (status IN ('creating', 'ready', 'deleting')),
  CONSTRAINT bots_reasoning_effort_check CHECK (reasoning_effort IN ('low', 'medium', 'high'))
);

CREATE INDEX IF NOT EXISTS idx_bots_owner_user_id ON bots(owner_user_id);

CREATE TABLE IF NOT EXISTS bot_acl_rules (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  action TEXT NOT NULL,
  effect TEXT NOT NULL,
  subject_kind TEXT NOT NULL,
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  channel_identity_id UUID REFERENCES channel_identities(id) ON DELETE CASCADE,
  source_channel TEXT,
  source_conversation_type TEXT,
  source_conversation_id TEXT,
  source_thread_id TEXT,
  created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_acl_rules_action_check CHECK (action IN ('chat.trigger')),
  CONSTRAINT bot_acl_rules_effect_check CHECK (effect IN ('allow', 'deny')),
  CONSTRAINT bot_acl_rules_subject_kind_check CHECK (subject_kind IN ('guest_all', 'user', 'channel_identity')),
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
    (subject_kind = 'guest_all' AND user_id IS NULL AND channel_identity_id IS NULL) OR
    (subject_kind = 'user' AND user_id IS NOT NULL AND channel_identity_id IS NULL) OR
    (subject_kind = 'channel_identity' AND user_id IS NULL AND channel_identity_id IS NOT NULL)
  ),
  CONSTRAINT bot_acl_rules_unique_user UNIQUE NULLS NOT DISTINCT (
    bot_id, action, effect, subject_kind, user_id,
    source_channel, source_conversation_type, source_conversation_id, source_thread_id
  ),
  CONSTRAINT bot_acl_rules_unique_channel_identity UNIQUE NULLS NOT DISTINCT (
    bot_id, action, effect, subject_kind, channel_identity_id,
    source_channel, source_conversation_type, source_conversation_id, source_thread_id
  )
);

CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_bot_id ON bot_acl_rules(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_user_id ON bot_acl_rules(user_id);
CREATE INDEX IF NOT EXISTS idx_bot_acl_rules_channel_identity_id ON bot_acl_rules(channel_identity_id);

CREATE TABLE IF NOT EXISTS mcp_connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'unknown',
  tools_cache JSONB NOT NULL DEFAULT '[]'::jsonb,
  last_probed_at TIMESTAMPTZ,
  status_message TEXT NOT NULL DEFAULT '',
  auth_type TEXT NOT NULL DEFAULT 'none',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_connections_type_check CHECK (type IN ('stdio', 'http', 'sse')),
  CONSTRAINT mcp_connections_unique UNIQUE (bot_id, name)
);

CREATE INDEX IF NOT EXISTS idx_mcp_connections_bot_id ON mcp_connections(bot_id);

CREATE TABLE IF NOT EXISTS mcp_oauth_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  connection_id UUID NOT NULL UNIQUE REFERENCES mcp_connections(id) ON DELETE CASCADE,
  resource_metadata_url TEXT NOT NULL DEFAULT '',
  authorization_server_url TEXT NOT NULL DEFAULT '',
  authorization_endpoint TEXT NOT NULL DEFAULT '',
  token_endpoint TEXT NOT NULL DEFAULT '',
  registration_endpoint TEXT NOT NULL DEFAULT '',
  scopes_supported TEXT[] NOT NULL DEFAULT '{}',
  client_id TEXT NOT NULL DEFAULT '',
  client_secret TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT 'Bearer',
  expires_at TIMESTAMPTZ,
  scope TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  state_param TEXT NOT NULL DEFAULT '',
  resource_uri TEXT NOT NULL DEFAULT '',
  redirect_uri TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mcp_oauth_tokens_connection_id ON mcp_oauth_tokens(connection_id);

-- Bot history is bot-scoped (one history container per bot).

CREATE TABLE IF NOT EXISTS bot_channel_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL,
  credentials JSONB NOT NULL DEFAULT '{}'::jsonb,
  external_identity TEXT,
  self_identity JSONB NOT NULL DEFAULT '{}'::jsonb,
  routing JSONB NOT NULL DEFAULT '{}'::jsonb,
  capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
  disabled BOOLEAN NOT NULL DEFAULT false,
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_channel_unique UNIQUE (bot_id, channel_type)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_channel_external_identity
  ON bot_channel_configs(channel_type, external_identity);

CREATE INDEX IF NOT EXISTS idx_bot_channel_bot_id ON bot_channel_configs(bot_id);

-- channel_identity_bind_codes: one-time codes for channel identity->user linking
CREATE TABLE IF NOT EXISTS channel_identity_bind_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  token TEXT NOT NULL,
  issued_by_user_id UUID NOT NULL REFERENCES users(id),
  channel_type TEXT,
  expires_at TIMESTAMPTZ,
  used_at TIMESTAMPTZ,
  used_by_channel_identity_id UUID REFERENCES channel_identities(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT channel_identity_bind_codes_token_unique UNIQUE (token)
);

CREATE INDEX IF NOT EXISTS idx_channel_identity_bind_codes_channel_type ON channel_identity_bind_codes(channel_type);

-- bot_channel_routes: route mapping for inbound channel threads to bot history.
CREATE TABLE IF NOT EXISTS bot_channel_routes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL,
  channel_config_id UUID REFERENCES bot_channel_configs(id) ON DELETE SET NULL,
  external_conversation_id TEXT NOT NULL,
  external_thread_id TEXT,
  conversation_type TEXT,
  default_reply_target TEXT,
  active_session_id UUID,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_channel_routes_unique
  ON bot_channel_routes (bot_id, channel_type, external_conversation_id, COALESCE(external_thread_id, ''));
CREATE INDEX IF NOT EXISTS idx_bot_channel_routes_bot ON bot_channel_routes(bot_id);

-- bot_sessions: chat sessions within a bot, optionally linked to a channel route.
CREATE TABLE IF NOT EXISTS bot_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  route_id UUID REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_type TEXT,
  type TEXT NOT NULL DEFAULT 'chat' CHECK (type IN ('chat', 'heartbeat', 'schedule', 'subagent', 'discuss')),
  title TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  parent_session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_id ON bot_sessions(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_route_id ON bot_sessions(route_id);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active ON bot_sessions(bot_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_bot_sessions_parent ON bot_sessions(parent_session_id) WHERE parent_session_id IS NOT NULL;

-- Add FK from routes to sessions (deferred to avoid circular dependency during CREATE).
ALTER TABLE bot_channel_routes
  ADD CONSTRAINT fk_bot_channel_routes_active_session
  FOREIGN KEY (active_session_id) REFERENCES bot_sessions(id) ON DELETE SET NULL;

-- bot_session_events: DCP pipeline event store for cold-start replay.
CREATE TABLE IF NOT EXISTS bot_session_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  event_kind TEXT NOT NULL CHECK (event_kind IN ('message', 'edit', 'delete', 'service')),
  event_data JSONB NOT NULL,
  external_message_id TEXT,
  sender_channel_identity_id UUID,
  received_at_ms BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_session_events_session_received
  ON bot_session_events (session_id, received_at_ms);
CREATE UNIQUE INDEX IF NOT EXISTS idx_session_events_dedup
  ON bot_session_events (session_id, event_kind, external_message_id)
  WHERE external_message_id IS NOT NULL AND external_message_id != '';

-- bot_history_messages: unified message history under bot scope.
CREATE TABLE IF NOT EXISTS bot_history_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  sender_channel_identity_id UUID REFERENCES channel_identities(id),
  sender_account_user_id UUID REFERENCES users(id),
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content JSONB NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  usage JSONB,
  model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  compact_id UUID,
  event_id UUID REFERENCES bot_session_events(id) ON DELETE SET NULL,
  display_text TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_compact ON bot_history_messages(compact_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session
  ON bot_history_messages(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_source
  ON bot_history_messages(session_id, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_reply
  ON bot_history_messages(session_id, source_reply_to_message_id);

CREATE TABLE IF NOT EXISTS tool_approval_requests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id UUID REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id UUID REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  tool_input JSONB NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id UUID REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id UUID REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id UUID REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id UUID REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  decided_at TIMESTAMPTZ,
  CONSTRAINT tool_approval_tool_name_check CHECK (tool_name IN ('write', 'edit', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id)
);

CREATE INDEX IF NOT EXISTS idx_tool_approval_bot_status_created
  ON tool_approval_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_session_status_created
  ON tool_approval_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approval_prompt_external
  ON tool_approval_requests(prompt_external_message_id)
  WHERE prompt_external_message_id != '';

CREATE TABLE IF NOT EXISTS containers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  container_id TEXT NOT NULL,
  container_name TEXT NOT NULL,
  image TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'created',
  namespace TEXT NOT NULL DEFAULT 'default',
  auto_start BOOLEAN NOT NULL DEFAULT true,
  container_path TEXT NOT NULL DEFAULT '/data',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_started_at TIMESTAMPTZ,
  last_stopped_at TIMESTAMPTZ,
  CONSTRAINT containers_container_id_unique UNIQUE (container_id),
  CONSTRAINT containers_container_name_unique UNIQUE (container_name)
);

CREATE INDEX IF NOT EXISTS idx_containers_bot_id ON containers(bot_id);

CREATE TABLE IF NOT EXISTS snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  runtime_snapshot_name TEXT NOT NULL,
  display_name TEXT,
  parent_runtime_snapshot_name TEXT,
  snapshotter TEXT NOT NULL,
  source TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_container_runtime_name
  ON snapshots(container_id, runtime_snapshot_name);
CREATE INDEX IF NOT EXISTS idx_snapshots_container_created_at
  ON snapshots(container_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_snapshots_runtime_name
  ON snapshots(runtime_snapshot_name);

CREATE TABLE IF NOT EXISTS container_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE RESTRICT,
  version INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (container_id, version)
);

CREATE INDEX IF NOT EXISTS idx_container_versions_container_id ON container_versions(container_id);
CREATE INDEX IF NOT EXISTS idx_container_versions_snapshot_id ON container_versions(snapshot_id);

CREATE TABLE IF NOT EXISTS lifecycle_events (
  id TEXT PRIMARY KEY,
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  event_type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_container_id ON lifecycle_events(container_id);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_event_type ON lifecycle_events(event_type);

CREATE TABLE IF NOT EXISTS schedule (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  pattern TEXT NOT NULL,
  max_calls INTEGER,
  current_calls INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  enabled BOOLEAN NOT NULL DEFAULT true,
  command TEXT NOT NULL,
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_schedule_bot_id ON schedule(bot_id);
CREATE INDEX IF NOT EXISTS idx_schedule_enabled ON schedule(enabled);

-- storage_providers: pluggable object storage backends
CREATE TABLE IF NOT EXISTS storage_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT storage_providers_name_unique UNIQUE (name),
  CONSTRAINT storage_providers_provider_check CHECK (provider IN ('localfs', 's3', 'gcs'))
);

-- bot_storage_bindings: per-bot storage backend selection
CREATE TABLE IF NOT EXISTS bot_storage_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  storage_provider_id UUID NOT NULL REFERENCES storage_providers(id) ON DELETE CASCADE,
  base_path TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_storage_bindings_unique UNIQUE (bot_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_storage_bindings_bot_id ON bot_storage_bindings(bot_id);

-- bot_history_message_assets: soft link (message -> content_hash only).
-- MIME, size, storage_key are derived from storage at read time.
CREATE TABLE IF NOT EXISTS bot_history_message_assets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  message_id UUID NOT NULL REFERENCES bot_history_messages(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'attachment',
  ordinal INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT message_asset_content_unique UNIQUE (message_id, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_message_assets_message_id ON bot_history_message_assets(message_id);


-- bot_heartbeat_logs: structured execution records for periodic heartbeat checks.
CREATE TABLE IF NOT EXISTS bot_heartbeat_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'alert', 'error')),
  result_text TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  usage JSONB,
  model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_heartbeat_logs_bot_started ON bot_heartbeat_logs(bot_id, started_at DESC);

CREATE TABLE IF NOT EXISTS bot_history_message_compacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'ok', 'error')),
  summary TEXT NOT NULL DEFAULT '',
  message_count INTEGER NOT NULL DEFAULT 0,
  error_message TEXT NOT NULL DEFAULT '',
  usage JSONB,
  model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_compacts_bot_session ON bot_history_message_compacts(bot_id, session_id, started_at DESC);

ALTER TABLE bot_history_messages ADD CONSTRAINT fk_compact_id FOREIGN KEY (compact_id) REFERENCES bot_history_message_compacts(id) ON DELETE SET NULL;

-- schedule_logs: structured execution records for scheduled tasks.
CREATE TABLE IF NOT EXISTS schedule_logs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  schedule_id UUID NOT NULL REFERENCES schedule(id) ON DELETE CASCADE,
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'ok' CHECK (status IN ('ok', 'error')),
  result_text TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  usage JSONB,
  model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_schedule_logs_schedule ON schedule_logs(schedule_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_schedule_logs_bot ON schedule_logs(bot_id, started_at DESC);

-- email_providers: pluggable email service backends
CREATE TABLE IF NOT EXISTS email_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT email_providers_name_unique UNIQUE (name)
);

-- email_oauth_tokens: stored OAuth2 tokens for Gmail email providers
CREATE TABLE IF NOT EXISTS email_oauth_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email_provider_id UUID NOT NULL UNIQUE REFERENCES email_providers(id) ON DELETE CASCADE,
  email_address TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  scope TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_oauth_tokens_state ON email_oauth_tokens(state) WHERE state != '';

-- bot_email_bindings: per-bot email provider binding with read/write/delete permissions
CREATE TABLE IF NOT EXISTS bot_email_bindings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  email_provider_id UUID NOT NULL REFERENCES email_providers(id) ON DELETE CASCADE,
  email_address TEXT NOT NULL,
  can_read BOOLEAN NOT NULL DEFAULT TRUE,
  can_write BOOLEAN NOT NULL DEFAULT TRUE,
  can_delete BOOLEAN NOT NULL DEFAULT FALSE,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_email_bindings_unique UNIQUE (bot_id, email_provider_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_email_bindings_bot_id ON bot_email_bindings(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_email_bindings_provider_id ON bot_email_bindings(email_provider_id);

-- email_outbox: outbound email audit log
CREATE TABLE IF NOT EXISTS email_outbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL REFERENCES email_providers(id) ON DELETE CASCADE,
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  message_id TEXT NOT NULL DEFAULT '',
  from_address TEXT NOT NULL DEFAULT '',
  to_addresses JSONB NOT NULL DEFAULT '[]'::jsonb,
  subject TEXT NOT NULL DEFAULT '',
  body_text TEXT NOT NULL DEFAULT '',
  body_html TEXT NOT NULL DEFAULT '',
  attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
  error TEXT NOT NULL DEFAULT '',
  sent_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_outbox_provider_id ON email_outbox(provider_id);
CREATE INDEX IF NOT EXISTS idx_email_outbox_bot_id ON email_outbox(bot_id, created_at DESC);

-- provider_oauth_tokens: OAuth2 tokens for LLM providers (e.g. OpenAI Codex OAuth)
CREATE TABLE IF NOT EXISTS provider_oauth_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL UNIQUE REFERENCES providers(id) ON DELETE CASCADE,
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  scope TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_provider_oauth_tokens_state ON provider_oauth_tokens(state) WHERE state != '';

-- user_provider_oauth_tokens: per-user OAuth2 tokens for providers with user-scoped auth (e.g. GitHub Copilot)
CREATE TABLE IF NOT EXISTS user_provider_oauth_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  scope TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT user_provider_oauth_tokens_provider_user_unique UNIQUE (provider_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_provider_oauth_tokens_state ON user_provider_oauth_tokens(state) WHERE state != '';
