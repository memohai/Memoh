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

CREATE TABLE IF NOT EXISTS llm_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT llm_providers_name_unique UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS search_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT search_providers_name_unique UNIQUE (name),
  CONSTRAINT search_providers_provider_check CHECK (provider IN ('brave'))
);

CREATE TABLE IF NOT EXISTS models (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id TEXT NOT NULL,
  name TEXT,
  llm_provider_id UUID NOT NULL REFERENCES llm_providers(id) ON DELETE CASCADE,
  client_type TEXT,
  dimensions INTEGER,
  input_modalities TEXT[] NOT NULL DEFAULT ARRAY['text']::TEXT[],
  type TEXT NOT NULL DEFAULT 'chat',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT models_model_id_unique UNIQUE (model_id),
  CONSTRAINT models_type_check CHECK (type IN ('chat', 'embedding')),
  CONSTRAINT models_dimensions_check CHECK (type != 'embedding' OR dimensions IS NOT NULL),
  CONSTRAINT models_client_type_check CHECK (client_type IS NULL OR client_type IN ('openai-responses', 'openai-completions', 'anthropic-messages', 'google-generative-ai')),
  CONSTRAINT models_chat_client_type_check CHECK (type != 'chat' OR client_type IS NOT NULL)
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

CREATE TABLE IF NOT EXISTS bots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  is_active BOOLEAN NOT NULL DEFAULT true,
  status TEXT NOT NULL DEFAULT 'ready',
  max_context_load_time INTEGER NOT NULL DEFAULT 1440,
  max_context_tokens INTEGER NOT NULL DEFAULT 0,
  language TEXT NOT NULL DEFAULT 'auto',
  allow_guest BOOLEAN NOT NULL DEFAULT false,
  max_inbox_items INTEGER NOT NULL DEFAULT 50,
  chat_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  memory_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  embedding_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  search_provider_id UUID REFERENCES search_providers(id) ON DELETE SET NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bots_type_check CHECK (type IN ('personal', 'public')),
  CONSTRAINT bots_status_check CHECK (status IN ('creating', 'ready', 'deleting'))
);

CREATE INDEX IF NOT EXISTS idx_bots_owner_user_id ON bots(owner_user_id);

CREATE TABLE IF NOT EXISTS bot_members (
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_members_role_check CHECK (role IN ('owner', 'admin', 'member')),
  CONSTRAINT bot_members_unique UNIQUE (bot_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_members_user_id ON bot_members(user_id);

CREATE TABLE IF NOT EXISTS mcp_connections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT mcp_connections_type_check CHECK (type IN ('stdio', 'http', 'sse')),
  CONSTRAINT mcp_connections_unique UNIQUE (bot_id, name)
);

CREATE INDEX IF NOT EXISTS idx_mcp_connections_bot_id ON mcp_connections(bot_id);

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

CREATE TABLE IF NOT EXISTS bot_preauth_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  token TEXT NOT NULL,
  issued_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  expires_at TIMESTAMPTZ,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_preauth_keys_unique UNIQUE (token)
);

CREATE INDEX IF NOT EXISTS idx_bot_preauth_keys_bot_id ON bot_preauth_keys(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_preauth_keys_expires ON bot_preauth_keys(expires_at);

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
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_channel_routes_unique
  ON bot_channel_routes (bot_id, channel_type, external_conversation_id, COALESCE(external_thread_id, ''));
CREATE INDEX IF NOT EXISTS idx_bot_channel_routes_bot ON bot_channel_routes(bot_id);

-- bot_history_messages: unified message history under bot scope.
CREATE TABLE IF NOT EXISTS bot_history_messages (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  route_id UUID REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  sender_channel_identity_id UUID REFERENCES channel_identities(id),
  sender_account_user_id UUID REFERENCES users(id),
  channel_type TEXT,
  source_message_id TEXT,
  source_reply_to_message_id TEXT,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'tool')),
  content JSONB NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  usage JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_bot_created ON bot_history_messages(bot_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_route ON bot_history_messages(route_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_source_lookup
  ON bot_history_messages(channel_type, source_message_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_reply_lookup
  ON bot_history_messages(channel_type, source_reply_to_message_id);

CREATE TABLE IF NOT EXISTS containers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  container_id TEXT NOT NULL,
  container_name TEXT NOT NULL,
  image TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'created',
  namespace TEXT NOT NULL DEFAULT 'default',
  auto_start BOOLEAN NOT NULL DEFAULT true,
  host_path TEXT,
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

CREATE TABLE IF NOT EXISTS subagents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  description TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  messages JSONB NOT NULL DEFAULT '[]'::jsonb,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  skills JSONB NOT NULL DEFAULT '[]'::jsonb,
  CONSTRAINT subagents_name_unique UNIQUE (bot_id, name)
);

CREATE INDEX IF NOT EXISTS idx_subagents_bot_id ON subagents(bot_id);
CREATE INDEX IF NOT EXISTS idx_subagents_deleted ON subagents(deleted);

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
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT message_asset_content_unique UNIQUE (message_id, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_message_assets_message_id ON bot_history_message_assets(message_id);

-- bot_inbox: per-bot message inbox for non-mentioned group messages, emails, etc.
CREATE TABLE IF NOT EXISTS bot_inbox (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT '',
  content JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_read BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  read_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_bot_inbox_bot_unread ON bot_inbox(bot_id, created_at DESC) WHERE is_read = FALSE;
CREATE INDEX IF NOT EXISTS idx_bot_inbox_bot_created ON bot_inbox(bot_id, created_at DESC);

