-- 0105_team_multitenancy
-- Add team tenant boundaries, default team backfill, composite uniqueness, and RLS.

CREATE TABLE IF NOT EXISTS teams (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT false,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT teams_slug_unique UNIQUE (slug)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_teams_single_default ON teams(is_default) WHERE is_default;

-- Minimal FK anchor only: the backfill + FK below require this row to exist
-- during the migration. Canonical team identity (name/slug/is_default) and
-- membership are owned by teams.EnsureDefault at server startup, so control
-- planes (SaaS) can manage the teams table without fighting a migration seed.
INSERT INTO teams (id, slug, name, is_default)
VALUES ('00000000-0000-0000-0000-000000000001'::uuid, 'default', 'Default', true)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS team_members (
  team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'member',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (team_id, user_id),
  CONSTRAINT team_members_role_check CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE INDEX IF NOT EXISTS idx_team_members_user_id ON team_members(user_id);

CREATE OR REPLACE FUNCTION memoh_add_team_column(table_name TEXT)
RETURNS void
LANGUAGE plpgsql
AS $$
DECLARE
  constraint_name TEXT;
BEGIN
  IF to_regclass('public.' || quote_ident(table_name)) IS NULL THEN
    RETURN;
  END IF;

  EXECUTE format('ALTER TABLE %I ADD COLUMN IF NOT EXISTS team_id UUID', table_name);
  EXECUTE format('UPDATE %I SET team_id = %L::uuid WHERE team_id IS NULL', table_name, '00000000-0000-0000-0000-000000000001');
  EXECUTE format('ALTER TABLE %I ALTER COLUMN team_id SET DEFAULT %L::uuid', table_name, '00000000-0000-0000-0000-000000000001');
  EXECUTE format('ALTER TABLE %I ALTER COLUMN team_id SET NOT NULL', table_name);

  constraint_name := table_name || '_team_id_fkey';
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = to_regclass('public.' || quote_ident(table_name))
      AND conname = constraint_name
  ) THEN
    EXECUTE format(
      'ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (team_id) REFERENCES teams(id) ON DELETE CASCADE',
      table_name,
      constraint_name
    );
  END IF;
END $$;

SELECT memoh_add_team_column(table_name)
FROM unnest(ARRAY[
  'channel_identities',
  'user_channel_bindings',
  'providers',
  'search_providers',
  'fetch_providers',
  'models',
  'model_variants',
  'memory_providers',
  'bots',
  'bot_acl_rules',
  'bot_channel_admins',
  'user_channel_identity_bindings',
  'channel_link_codes',
  'bot_plugin_installations',
  'mcp_connections',
  'bot_plugin_resources',
  'mcp_oauth_tokens',
  'bot_channel_configs',
  'channel_identity_bind_codes',
  'bot_channel_routes',
  'bot_sessions',
  'bot_session_events',
  'bot_history_messages',
  'bot_session_discuss_cursors',
  'tool_approval_requests',
  'user_input_requests',
  'containers',
  'bot_workspace_resource_limits',
  'snapshots',
  'container_versions',
  'lifecycle_events',
  'schedule',
  'bot_storage_bindings',
  'media_assets',
  'bot_history_message_assets',
  'bot_heartbeat_logs',
  'bot_history_message_compacts',
  'schedule_logs',
  'email_providers',
  'email_oauth_tokens',
  'bot_email_bindings',
  'email_outbox',
  'provider_oauth_tokens',
  'user_provider_oauth_tokens',
  'bot_user_grants',
  'memory_nodes',
  'memory_edges',
  'browser_contexts',
  'tasks'
]) AS tenant_tables(table_name);

DROP FUNCTION memoh_add_team_column(TEXT);

-- Default-team membership is enrolled by teams.EnsureDefault at server startup,
-- not seeded here, so a control plane (SaaS) owns who belongs to which team.

ALTER TABLE IF EXISTS channel_identities DROP CONSTRAINT IF EXISTS channel_identities_channel_type_subject_unique;
ALTER TABLE IF EXISTS user_channel_bindings DROP CONSTRAINT IF EXISTS user_channel_bindings_unique;
ALTER TABLE IF EXISTS providers DROP CONSTRAINT IF EXISTS providers_name_unique;
ALTER TABLE IF EXISTS search_providers DROP CONSTRAINT IF EXISTS search_providers_name_unique;
ALTER TABLE IF EXISTS fetch_providers DROP CONSTRAINT IF EXISTS fetch_providers_name_unique;
ALTER TABLE IF EXISTS models DROP CONSTRAINT IF EXISTS models_provider_id_model_id_unique;
ALTER TABLE IF EXISTS memory_providers DROP CONSTRAINT IF EXISTS memory_providers_name_unique;
DROP INDEX IF EXISTS idx_bots_name;
ALTER TABLE IF EXISTS bot_channel_admins DROP CONSTRAINT IF EXISTS bot_channel_admins_unique;
ALTER TABLE IF EXISTS user_channel_identity_bindings DROP CONSTRAINT IF EXISTS user_channel_identity_bindings_unique;
ALTER TABLE IF EXISTS bot_plugin_installations DROP CONSTRAINT IF EXISTS bot_plugin_installations_unique;
ALTER TABLE IF EXISTS mcp_connections DROP CONSTRAINT IF EXISTS mcp_connections_unique;
ALTER TABLE IF EXISTS bot_plugin_resources DROP CONSTRAINT IF EXISTS bot_plugin_resources_unique;
ALTER TABLE IF EXISTS mcp_oauth_tokens DROP CONSTRAINT IF EXISTS mcp_oauth_tokens_connection_id_key;
ALTER TABLE IF EXISTS bot_channel_configs DROP CONSTRAINT IF EXISTS bot_channel_unique;
ALTER TABLE IF EXISTS bot_session_discuss_cursors DROP CONSTRAINT IF EXISTS bot_session_discuss_cursors_session_scope_key;
ALTER TABLE IF EXISTS tool_approval_requests DROP CONSTRAINT IF EXISTS tool_approval_short_id_unique;
ALTER TABLE IF EXISTS tool_approval_requests DROP CONSTRAINT IF EXISTS tool_approval_tool_call_unique;
ALTER TABLE IF EXISTS user_input_requests DROP CONSTRAINT IF EXISTS user_input_short_id_unique;
ALTER TABLE IF EXISTS user_input_requests DROP CONSTRAINT IF EXISTS user_input_tool_call_unique;
-- containers_container_id_unique is the FK target of snapshots/container_versions/
-- lifecycle_events; drop those FKs first, then recreate them as team-scoped
-- composites once idx_containers_container_id_team_unique exists (below).
ALTER TABLE IF EXISTS snapshots DROP CONSTRAINT IF EXISTS snapshots_container_id_fkey;
ALTER TABLE IF EXISTS container_versions DROP CONSTRAINT IF EXISTS container_versions_container_id_fkey;
ALTER TABLE IF EXISTS lifecycle_events DROP CONSTRAINT IF EXISTS lifecycle_events_container_id_fkey;
ALTER TABLE IF EXISTS containers DROP CONSTRAINT IF EXISTS containers_container_id_unique;
ALTER TABLE IF EXISTS containers DROP CONSTRAINT IF EXISTS containers_container_name_unique;
ALTER TABLE IF EXISTS bot_storage_bindings DROP CONSTRAINT IF EXISTS bot_storage_bindings_unique;
ALTER TABLE IF EXISTS media_assets DROP CONSTRAINT IF EXISTS media_assets_bot_hash_unique;
ALTER TABLE IF EXISTS bot_history_message_assets DROP CONSTRAINT IF EXISTS message_asset_content_unique;
ALTER TABLE IF EXISTS bot_history_message_assets DROP CONSTRAINT IF EXISTS message_asset_unique;
ALTER TABLE IF EXISTS email_providers DROP CONSTRAINT IF EXISTS email_providers_user_name_unique;
ALTER TABLE IF EXISTS bot_email_bindings DROP CONSTRAINT IF EXISTS bot_email_bindings_unique;
ALTER TABLE IF EXISTS provider_oauth_tokens DROP CONSTRAINT IF EXISTS provider_oauth_tokens_provider_id_key;
ALTER TABLE IF EXISTS user_provider_oauth_tokens DROP CONSTRAINT IF EXISTS user_provider_oauth_tokens_provider_user_unique;
ALTER TABLE IF EXISTS memory_edges DROP CONSTRAINT IF EXISTS memory_edges_unique;

CREATE OR REPLACE FUNCTION memoh_create_team_index(index_name TEXT, table_name TEXT, index_definition TEXT, is_unique BOOLEAN)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
  IF to_regclass('public.' || quote_ident(table_name)) IS NULL THEN
    RETURN;
  END IF;

  EXECUTE format(
    'CREATE %s INDEX IF NOT EXISTS %I ON %I %s',
    CASE WHEN is_unique THEN 'UNIQUE' ELSE '' END,
    index_name,
    table_name,
    index_definition
  );
END $$;

SELECT memoh_create_team_index(index_name, table_name, index_definition, is_unique)
FROM (VALUES
  ('idx_channel_identities_team_subject', 'channel_identities', '(team_id, channel_type, channel_subject_id)', true),
  ('idx_user_channel_bindings_team_unique', 'user_channel_bindings', '(team_id, user_id, channel_type)', true),
  ('idx_providers_team_name', 'providers', '(team_id, name)', true),
  ('idx_search_providers_team_name', 'search_providers', '(team_id, name)', true),
  ('idx_fetch_providers_team_name', 'fetch_providers', '(team_id, name)', true),
  ('idx_models_team_provider_model', 'models', '(team_id, provider_id, model_id)', true),
  ('idx_memory_providers_team_name', 'memory_providers', '(team_id, name)', true),
  ('idx_bots_team_name', 'bots', '(team_id, name)', true),
  ('idx_bot_channel_admins_team_unique', 'bot_channel_admins', '(team_id, bot_id, channel_identity_id)', true),
  ('idx_uci_bindings_team_unique', 'user_channel_identity_bindings', '(team_id, user_id, channel_identity_id)', true),
  ('idx_plugin_installs_team_unique', 'bot_plugin_installations', '(team_id, bot_id, plugin_id)', true),
  ('idx_mcp_connections_team_unique', 'mcp_connections', '(team_id, bot_id, name)', true),
  ('idx_plugin_resources_team_unique', 'bot_plugin_resources', '(team_id, installation_id, resource_type, resource_key)', true),
  ('idx_mcp_oauth_tokens_team_connection', 'mcp_oauth_tokens', '(team_id, connection_id)', true),
  ('idx_bot_channel_configs_team_unique', 'bot_channel_configs', '(team_id, bot_id, channel_type)', true),
  ('idx_session_discuss_cursors_team_unique', 'bot_session_discuss_cursors', '(team_id, session_id, scope_key)', true),
  ('idx_tool_approval_short_id_team_unique', 'tool_approval_requests', '(team_id, session_id, short_id)', true),
  ('idx_tool_approval_tool_call_team_unique', 'tool_approval_requests', '(team_id, session_id, tool_call_id)', true),
  ('idx_user_input_short_id_team_unique', 'user_input_requests', '(team_id, session_id, short_id)', true),
  ('idx_user_input_tool_call_team_unique', 'user_input_requests', '(team_id, session_id, tool_call_id)', true),
  ('idx_containers_container_id_team_unique', 'containers', '(team_id, container_id)', true),
  ('idx_containers_container_name_team_unique', 'containers', '(team_id, container_name)', true),
  ('idx_storage_bindings_team_unique', 'bot_storage_bindings', '(team_id, bot_id)', true),
  ('idx_media_assets_team_bot_hash', 'media_assets', '(team_id, bot_id, content_hash)', true),
  ('idx_message_assets_team_content_unique', 'bot_history_message_assets', '(team_id, message_id, content_hash)', true),
  ('idx_email_providers_team_user_name', 'email_providers', '(team_id, user_id, name)', true),
  ('idx_bot_email_bindings_team_unique', 'bot_email_bindings', '(team_id, bot_id, email_provider_id)', true),
  ('idx_provider_oauth_team_unique', 'provider_oauth_tokens', '(team_id, provider_id)', true),
  ('idx_user_provider_oauth_team_unique', 'user_provider_oauth_tokens', '(team_id, provider_id, user_id)', true),
  ('idx_memory_edges_team_unique', 'memory_edges', '(team_id, bot_id, src_node, dst_node, rel)', true),
  ('idx_memory_nodes_team_bot_id', 'memory_nodes', '(team_id, bot_id, id)', true),
  ('idx_bots_team_owner', 'bots', '(team_id, owner_user_id)', false),
  ('idx_bot_acl_rules_team_bot', 'bot_acl_rules', '(team_id, bot_id)', false),
  ('idx_bot_channel_routes_team_bot', 'bot_channel_routes', '(team_id, bot_id)', false),
  ('idx_bot_sessions_team_bot_updated', 'bot_sessions', '(team_id, bot_id, updated_at DESC)', false),
  ('idx_session_events_team_session', 'bot_session_events', '(team_id, session_id)', false),
  ('idx_history_messages_team_session', 'bot_history_messages', '(team_id, session_id, created_at DESC)', false),
  ('idx_tool_approvals_team_bot', 'tool_approval_requests', '(team_id, bot_id)', false),
  ('idx_user_input_team_session', 'user_input_requests', '(team_id, session_id)', false),
  ('idx_containers_team_bot', 'containers', '(team_id, bot_id)', false),
  ('idx_resource_limits_team_bot', 'bot_workspace_resource_limits', '(team_id, bot_id)', false),
  ('idx_snapshots_team_container', 'snapshots', '(team_id, container_id)', false),
  ('idx_container_versions_team_container', 'container_versions', '(team_id, container_id)', false),
  ('idx_lifecycle_events_team_container', 'lifecycle_events', '(team_id, container_id)', false),
  ('idx_schedule_team_bot', 'schedule', '(team_id, bot_id)', false),
  ('idx_message_assets_team_message', 'bot_history_message_assets', '(team_id, message_id)', false),
  ('idx_heartbeat_logs_team_bot', 'bot_heartbeat_logs', '(team_id, bot_id)', false),
  ('idx_compacts_team_session', 'bot_history_message_compacts', '(team_id, session_id)', false),
  ('idx_schedule_logs_team_bot', 'schedule_logs', '(team_id, bot_id)', false),
  ('idx_email_oauth_team_provider', 'email_oauth_tokens', '(team_id, email_provider_id)', false),
  ('idx_email_outbox_team_bot', 'email_outbox', '(team_id, bot_id, created_at DESC)', false),
  ('idx_bot_user_grants_team_bot', 'bot_user_grants', '(team_id, bot_id)', false),
  ('idx_memory_nodes_team_bot_layer', 'memory_nodes', '(team_id, bot_id, layer)', false),
  ('idx_memory_edges_team_bot_src', 'memory_edges', '(team_id, bot_id, src_node)', false),
  ('idx_browser_contexts_team_name', 'browser_contexts', '(team_id, name)', false),
  ('idx_tasks_team_bot', 'tasks', '(team_id, bot_id)', false)
) AS indexes(index_name, table_name, index_definition, is_unique);

DROP FUNCTION memoh_create_team_index(TEXT, TEXT, TEXT, BOOLEAN);

-- Recreate the container-scoped FKs as team-scoped composites backed by
-- idx_containers_container_id_team_unique.
DO $$
DECLARE
  fk RECORD;
BEGIN
  FOR fk IN
    SELECT *
    FROM (VALUES
      ('snapshots', 'snapshots_container_id_fkey'),
      ('container_versions', 'container_versions_container_id_fkey'),
      ('lifecycle_events', 'lifecycle_events_container_id_fkey')
    ) AS fks(table_name, constraint_name)
  LOOP
    IF to_regclass('public.' || quote_ident(fk.table_name)) IS NULL OR to_regclass('public.containers') IS NULL THEN
      CONTINUE;
    END IF;
    IF EXISTS (
      SELECT 1
      FROM pg_constraint
      WHERE conrelid = to_regclass('public.' || quote_ident(fk.table_name))
        AND conname = fk.constraint_name
    ) THEN
      CONTINUE;
    END IF;
    EXECUTE format(
      'ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (team_id, container_id) REFERENCES containers(team_id, container_id) ON DELETE CASCADE',
      fk.table_name,
      fk.constraint_name
    );
  END LOOP;
END $$;

-- bot_visible_history_messages predates team_id; recreate it so team-scoped
-- queries can filter on m.team_id (CREATE OR REPLACE may only append columns).
CREATE OR REPLACE VIEW bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.compact_id,
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.event_id,
  m.display_text,
  m.created_at,
  m.team_id
FROM bot_history_messages m
WHERE m.turn_visible = true
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL;

DO $$
DECLARE
  table_name TEXT;
BEGIN
  FOREACH table_name IN ARRAY ARRAY[
    'team_members',
    'channel_identities',
    'user_channel_bindings',
    'providers',
    'search_providers',
    'fetch_providers',
    'models',
    'model_variants',
    'memory_providers',
    'bots',
    'bot_acl_rules',
    'bot_channel_admins',
    'user_channel_identity_bindings',
    'channel_link_codes',
    'bot_plugin_installations',
    'mcp_connections',
    'bot_plugin_resources',
    'mcp_oauth_tokens',
    'bot_channel_configs',
    'channel_identity_bind_codes',
    'bot_channel_routes',
    'bot_sessions',
    'bot_session_events',
    'bot_history_messages',
    'bot_session_discuss_cursors',
    'tool_approval_requests',
    'user_input_requests',
    'containers',
    'bot_workspace_resource_limits',
    'snapshots',
    'container_versions',
    'lifecycle_events',
    'schedule',
    'bot_storage_bindings',
    'media_assets',
    'bot_history_message_assets',
    'bot_heartbeat_logs',
    'bot_history_message_compacts',
    'schedule_logs',
    'email_providers',
    'email_oauth_tokens',
    'bot_email_bindings',
    'email_outbox',
    'provider_oauth_tokens',
    'user_provider_oauth_tokens',
    'bot_user_grants',
    'memory_nodes',
    'memory_edges',
    'browser_contexts',
    'tasks'
  ]
  LOOP
    IF to_regclass('public.' || quote_ident(table_name)) IS NULL THEN
      CONTINUE;
    END IF;

    EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', table_name);
    IF NOT EXISTS (
      SELECT 1
      FROM pg_policies
      WHERE schemaname = 'public'
        AND tablename = table_name
        AND policyname = 'team_isolation'
    ) THEN
      EXECUTE format(
        'CREATE POLICY team_isolation ON %I USING (team_id = NULLIF(current_setting(''app.team_id'', true), '''')::uuid) WITH CHECK (team_id = NULLIF(current_setting(''app.team_id'', true), '''')::uuid)',
        table_name
      );
    END IF;
  END LOOP;
END $$;
