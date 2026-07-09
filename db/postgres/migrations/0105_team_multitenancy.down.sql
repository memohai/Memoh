-- 0105_team_multitenancy
-- Remove team tenant boundaries and restore pre-team uniqueness.

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

    IF EXISTS (
      SELECT 1
      FROM pg_policies
      WHERE schemaname = 'public'
        AND tablename = table_name
        AND policyname = 'team_isolation'
    ) THEN
      EXECUTE format('DROP POLICY team_isolation ON %I', table_name);
    END IF;
    EXECUTE format('ALTER TABLE %I DISABLE ROW LEVEL SECURITY', table_name);
  END LOOP;
END $$;

-- The team-scoped composite FKs depend on idx_containers_container_id_team_unique;
-- drop them before the index, then restore the single-column FKs after
-- containers_container_id_unique is recreated (below).
ALTER TABLE IF EXISTS snapshots DROP CONSTRAINT IF EXISTS snapshots_container_id_fkey;
ALTER TABLE IF EXISTS container_versions DROP CONSTRAINT IF EXISTS container_versions_container_id_fkey;
ALTER TABLE IF EXISTS lifecycle_events DROP CONSTRAINT IF EXISTS lifecycle_events_container_id_fkey;

-- The view exposes team_id, which blocks dropping the column; drop it here and
-- recreate the pre-team definition at the end of this migration.
DROP VIEW IF EXISTS bot_visible_history_messages;

DROP INDEX IF EXISTS idx_channel_identities_team_subject;
DROP INDEX IF EXISTS idx_user_channel_bindings_team_unique;
DROP INDEX IF EXISTS idx_providers_team_name;
DROP INDEX IF EXISTS idx_search_providers_team_name;
DROP INDEX IF EXISTS idx_fetch_providers_team_name;
DROP INDEX IF EXISTS idx_models_team_provider_model;
DROP INDEX IF EXISTS idx_memory_providers_team_name;
DROP INDEX IF EXISTS idx_bots_team_name;
DROP INDEX IF EXISTS idx_bot_channel_admins_team_unique;
DROP INDEX IF EXISTS idx_uci_bindings_team_unique;
DROP INDEX IF EXISTS idx_plugin_installs_team_unique;
DROP INDEX IF EXISTS idx_mcp_connections_team_unique;
DROP INDEX IF EXISTS idx_plugin_resources_team_unique;
DROP INDEX IF EXISTS idx_mcp_oauth_tokens_team_connection;
DROP INDEX IF EXISTS idx_bot_channel_configs_team_unique;
DROP INDEX IF EXISTS idx_session_discuss_cursors_team_unique;
DROP INDEX IF EXISTS idx_tool_approval_short_id_team_unique;
DROP INDEX IF EXISTS idx_tool_approval_tool_call_team_unique;
DROP INDEX IF EXISTS idx_user_input_short_id_team_unique;
DROP INDEX IF EXISTS idx_user_input_tool_call_team_unique;
DROP INDEX IF EXISTS idx_containers_container_id_team_unique;
DROP INDEX IF EXISTS idx_containers_container_name_team_unique;
DROP INDEX IF EXISTS idx_storage_bindings_team_unique;
DROP INDEX IF EXISTS idx_media_assets_team_bot_hash;
DROP INDEX IF EXISTS idx_message_assets_team_content_unique;
DROP INDEX IF EXISTS idx_email_providers_team_user_name;
DROP INDEX IF EXISTS idx_bot_email_bindings_team_unique;
DROP INDEX IF EXISTS idx_provider_oauth_team_unique;
DROP INDEX IF EXISTS idx_user_provider_oauth_team_unique;
DROP INDEX IF EXISTS idx_memory_edges_team_unique;
DROP INDEX IF EXISTS idx_memory_nodes_team_bot_id;
DROP INDEX IF EXISTS idx_bots_team_owner;
DROP INDEX IF EXISTS idx_bot_acl_rules_team_bot;
DROP INDEX IF EXISTS idx_bot_channel_routes_team_bot;
DROP INDEX IF EXISTS idx_bot_sessions_team_bot_updated;
DROP INDEX IF EXISTS idx_session_events_team_session;
DROP INDEX IF EXISTS idx_history_messages_team_session;
DROP INDEX IF EXISTS idx_tool_approvals_team_bot;
DROP INDEX IF EXISTS idx_user_input_team_session;
DROP INDEX IF EXISTS idx_containers_team_bot;
DROP INDEX IF EXISTS idx_resource_limits_team_bot;
DROP INDEX IF EXISTS idx_snapshots_team_container;
DROP INDEX IF EXISTS idx_container_versions_team_container;
DROP INDEX IF EXISTS idx_lifecycle_events_team_container;
DROP INDEX IF EXISTS idx_schedule_team_bot;
DROP INDEX IF EXISTS idx_message_assets_team_message;
DROP INDEX IF EXISTS idx_heartbeat_logs_team_bot;
DROP INDEX IF EXISTS idx_compacts_team_session;
DROP INDEX IF EXISTS idx_schedule_logs_team_bot;
DROP INDEX IF EXISTS idx_email_oauth_team_provider;
DROP INDEX IF EXISTS idx_email_outbox_team_bot;
DROP INDEX IF EXISTS idx_bot_user_grants_team_bot;
DROP INDEX IF EXISTS idx_memory_nodes_team_bot_layer;
DROP INDEX IF EXISTS idx_memory_edges_team_bot_src;
DROP INDEX IF EXISTS idx_browser_contexts_team_name;
DROP INDEX IF EXISTS idx_tasks_team_bot;

CREATE OR REPLACE FUNCTION memoh_add_unique_constraint(table_name TEXT, constraint_name TEXT, constraint_definition TEXT)
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
  IF to_regclass('public.' || quote_ident(table_name)) IS NULL THEN
    RETURN;
  END IF;
  IF EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conrelid = to_regclass('public.' || quote_ident(table_name))
      AND conname = constraint_name
  ) THEN
    RETURN;
  END IF;
  EXECUTE format('ALTER TABLE %I ADD CONSTRAINT %I %s', table_name, constraint_name, constraint_definition);
END $$;

SELECT memoh_add_unique_constraint(table_name, constraint_name, constraint_definition)
FROM (VALUES
  ('channel_identities', 'channel_identities_channel_type_subject_unique', 'UNIQUE (channel_type, channel_subject_id)'),
  ('user_channel_bindings', 'user_channel_bindings_unique', 'UNIQUE (user_id, channel_type)'),
  ('providers', 'providers_name_unique', 'UNIQUE (name)'),
  ('search_providers', 'search_providers_name_unique', 'UNIQUE (name)'),
  ('fetch_providers', 'fetch_providers_name_unique', 'UNIQUE (name)'),
  ('models', 'models_provider_id_model_id_unique', 'UNIQUE (provider_id, model_id)'),
  ('memory_providers', 'memory_providers_name_unique', 'UNIQUE (name)'),
  ('bot_channel_admins', 'bot_channel_admins_unique', 'UNIQUE (bot_id, channel_identity_id)'),
  ('user_channel_identity_bindings', 'user_channel_identity_bindings_unique', 'UNIQUE (user_id, channel_identity_id)'),
  ('bot_plugin_installations', 'bot_plugin_installations_unique', 'UNIQUE (bot_id, plugin_id)'),
  ('mcp_connections', 'mcp_connections_unique', 'UNIQUE (bot_id, name)'),
  ('bot_plugin_resources', 'bot_plugin_resources_unique', 'UNIQUE (installation_id, resource_type, resource_key)'),
  ('mcp_oauth_tokens', 'mcp_oauth_tokens_connection_id_key', 'UNIQUE (connection_id)'),
  ('bot_channel_configs', 'bot_channel_unique', 'UNIQUE (bot_id, channel_type)'),
  ('tool_approval_requests', 'tool_approval_short_id_unique', 'UNIQUE (session_id, short_id)'),
  ('tool_approval_requests', 'tool_approval_tool_call_unique', 'UNIQUE (session_id, tool_call_id)'),
  ('user_input_requests', 'user_input_short_id_unique', 'UNIQUE (session_id, short_id)'),
  ('user_input_requests', 'user_input_tool_call_unique', 'UNIQUE (session_id, tool_call_id)'),
  ('containers', 'containers_container_id_unique', 'UNIQUE (container_id)'),
  ('containers', 'containers_container_name_unique', 'UNIQUE (container_name)'),
  ('bot_storage_bindings', 'bot_storage_bindings_unique', 'UNIQUE (bot_id)'),
  ('media_assets', 'media_assets_bot_hash_unique', 'UNIQUE (bot_id, content_hash)'),
  ('bot_history_message_assets', 'message_asset_content_unique', 'UNIQUE (message_id, content_hash)'),
  ('email_providers', 'email_providers_user_name_unique', 'UNIQUE (user_id, name)'),
  ('bot_email_bindings', 'bot_email_bindings_unique', 'UNIQUE (bot_id, email_provider_id)'),
  ('provider_oauth_tokens', 'provider_oauth_tokens_provider_id_key', 'UNIQUE (provider_id)'),
  ('user_provider_oauth_tokens', 'user_provider_oauth_tokens_provider_user_unique', 'UNIQUE (provider_id, user_id)'),
  ('memory_edges', 'memory_edges_unique', 'UNIQUE (bot_id, src_node, dst_node, rel)')
) AS old_constraints(table_name, constraint_name, constraint_definition);

DROP FUNCTION memoh_add_unique_constraint(TEXT, TEXT, TEXT);

-- Restore the pre-team single-column container FKs now that
-- containers_container_id_unique exists again.
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
      'ALTER TABLE %I ADD CONSTRAINT %I FOREIGN KEY (container_id) REFERENCES containers(container_id) ON DELETE CASCADE',
      fk.table_name,
      fk.constraint_name
    );
  END LOOP;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_bots_name ON bots(name);

ALTER TABLE IF EXISTS channel_identities DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS user_channel_bindings DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS providers DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS search_providers DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS fetch_providers DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS models DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS model_variants DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS memory_providers DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bots DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_acl_rules DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_channel_admins DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS user_channel_identity_bindings DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS channel_link_codes DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_plugin_installations DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS mcp_connections DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_plugin_resources DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS mcp_oauth_tokens DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_channel_configs DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS channel_identity_bind_codes DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_channel_routes DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_sessions DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_session_events DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_history_messages DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_session_discuss_cursors DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS tool_approval_requests DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS user_input_requests DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS containers DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_workspace_resource_limits DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS snapshots DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS container_versions DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS lifecycle_events DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS schedule DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_storage_bindings DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS media_assets DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_history_message_assets DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_heartbeat_logs DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_history_message_compacts DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS schedule_logs DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS email_providers DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS email_oauth_tokens DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_email_bindings DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS email_outbox DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS provider_oauth_tokens DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS user_provider_oauth_tokens DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS bot_user_grants DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS memory_nodes DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS memory_edges DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS browser_contexts DROP COLUMN IF EXISTS team_id;
ALTER TABLE IF EXISTS tasks DROP COLUMN IF EXISTS team_id;

DROP TABLE IF EXISTS team_members CASCADE;
DROP TABLE IF EXISTS teams CASCADE;

-- Restore the pre-team view definition (from 0103_message_turn_read_model).
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
  m.created_at
FROM bot_history_messages m
WHERE m.turn_visible = true
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL;
