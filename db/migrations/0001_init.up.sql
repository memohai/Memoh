CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
    CREATE TYPE user_role AS ENUM ('member', 'admin');
  END IF;
END
$$;

CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  username TEXT NOT NULL,
  email TEXT,
  password_hash TEXT NOT NULL,
  role user_role NOT NULL DEFAULT 'member',
  display_name TEXT,
  avatar_url TEXT,
  is_active BOOLEAN NOT NULL DEFAULT true,
  data_root TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ,
  CONSTRAINT users_email_unique UNIQUE (email),
  CONSTRAINT users_username_unique UNIQUE (username)
);

CREATE TABLE IF NOT EXISTS llm_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  client_type TEXT NOT NULL,
  base_url TEXT NOT NULL,
  api_key TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT llm_providers_name_unique UNIQUE (name),
  CONSTRAINT llm_providers_client_type_check CHECK (client_type IN ('openai', 'openai-compat', 'anthropic', 'google', 'ollama'))
);

CREATE TABLE IF NOT EXISTS models (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  model_id TEXT NOT NULL,
  name TEXT,
  llm_provider_id UUID NOT NULL REFERENCES llm_providers(id) ON DELETE CASCADE,
  dimensions INTEGER,
  is_multimodal BOOLEAN NOT NULL DEFAULT false,
  type TEXT NOT NULL DEFAULT 'chat',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT models_model_id_unique UNIQUE (model_id),
  CONSTRAINT models_type_check CHECK (type IN ('chat', 'embedding')),
  CONSTRAINT models_dimensions_check CHECK (type != 'embedding' OR dimensions IS NOT NULL)
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
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bots_type_check CHECK (type IN ('personal', 'public'))
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

CREATE TABLE IF NOT EXISTS bot_settings (
  bot_id UUID PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
  max_context_load_time INTEGER NOT NULL DEFAULT 1440,
  language TEXT NOT NULL DEFAULT 'auto',
  allow_guest BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS bot_model_configs (
  bot_id UUID PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
  chat_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  embedding_model_id UUID REFERENCES models(id) ON DELETE SET NULL,
  memory_model_id UUID REFERENCES models(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS conversations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL,
  channel_type TEXT NOT NULL,
  chat_id TEXT,
  sender_id TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT conversations_session_unique UNIQUE (bot_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_conversations_bot_id ON conversations(bot_id);

CREATE TABLE IF NOT EXISTS history (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL,
  messages JSONB NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  skills TEXT[] NOT NULL DEFAULT '{}'::text[],
  timestamp TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_history_bot ON history(bot_id);
CREATE INDEX IF NOT EXISTS idx_history_session ON history(session_id);
CREATE INDEX IF NOT EXISTS idx_history_timestamp ON history(timestamp);

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

CREATE TABLE IF NOT EXISTS bot_channel_configs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL,
  credentials JSONB NOT NULL DEFAULT '{}'::jsonb,
  external_identity TEXT,
  self_identity JSONB NOT NULL DEFAULT '{}'::jsonb,
  routing JSONB NOT NULL DEFAULT '{}'::jsonb,
  capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending',
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT bot_channel_status_check CHECK (status IN ('pending', 'verified', 'disabled')),
  CONSTRAINT bot_channel_unique UNIQUE (bot_id, channel_type)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_channel_external_identity
  ON bot_channel_configs(channel_type, external_identity);

CREATE INDEX IF NOT EXISTS idx_bot_channel_bot_id ON bot_channel_configs(bot_id);

CREATE TABLE IF NOT EXISTS contacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  user_id UUID REFERENCES users(id) ON DELETE SET NULL,
  display_name TEXT,
  alias TEXT,
  tags TEXT[] NOT NULL DEFAULT '{}'::text[],
  status TEXT NOT NULL DEFAULT 'active',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT contacts_status_check CHECK (status IN ('active', 'blocked', 'pending'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contacts_bot_user_unique
  ON contacts(bot_id, user_id)
  WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_contacts_bot_id ON contacts(bot_id);

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

CREATE TABLE IF NOT EXISTS contact_channels (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  contact_id UUID NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  platform TEXT NOT NULL,
  external_id TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT contact_channels_unique UNIQUE (bot_id, platform, external_id)
);

CREATE INDEX IF NOT EXISTS idx_contact_channels_contact_id ON contact_channels(contact_id);
CREATE INDEX IF NOT EXISTS idx_contact_channels_platform_external ON contact_channels(platform, external_id);

CREATE TABLE IF NOT EXISTS channel_sessions (
  session_id TEXT PRIMARY KEY,
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  channel_config_id UUID REFERENCES bot_channel_configs(id) ON DELETE SET NULL,
  user_id UUID REFERENCES users(id) ON DELETE CASCADE,
  contact_id UUID REFERENCES contacts(id) ON DELETE SET NULL,
  platform TEXT NOT NULL,
  reply_target TEXT,
  thread_id TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_channel_sessions_bot_id ON channel_sessions(bot_id);
CREATE INDEX IF NOT EXISTS idx_channel_sessions_user_id ON channel_sessions(user_id);

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
  id TEXT PRIMARY KEY,
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  parent_snapshot_id TEXT REFERENCES snapshots(id) ON DELETE SET NULL,
  snapshotter TEXT NOT NULL,
  digest TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_snapshots_container_id ON snapshots(container_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_parent_id ON snapshots(parent_snapshot_id);

CREATE TABLE IF NOT EXISTS container_versions (
  id TEXT PRIMARY KEY,
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE RESTRICT,
  version INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (container_id, version)
);

CREATE INDEX IF NOT EXISTS idx_container_versions_container_id ON container_versions(container_id);

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

CREATE TABLE IF NOT EXISTS user_settings (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  chat_model_id TEXT,
  memory_model_id TEXT,
  embedding_model_id TEXT,
  max_context_load_time INTEGER NOT NULL DEFAULT 1440,
  language TEXT NOT NULL DEFAULT 'auto'
);
