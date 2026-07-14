-- 0108_tenant_id_nullable_backfill
-- Add a nullable tenant_id column to every tenant business table (STATIC ALTERs
-- so sqlc can see the column) and backfill it to the default singleton tenant
-- before tenant-scoped constraints are installed.
--
-- New incremental (existing migrations untouched). tenant_id is added NULLABLE,
-- given a DEFAULT of app.current_tenant_id() (so INSERTs auto-fill the current
-- tenant), and backfilled to the default singleton; a later migration tightens
-- it to NOT NULL after composite keys/FKs land. Existing installs upgrade in
-- place (no wipe).
--
-- The ADD COLUMN statements are STATIC (one literal ALTER per table) rather than
-- a dynamic DO/EXECUTE loop, because sqlc parses the schema declaratively and
-- cannot see columns added inside DO/EXECUTE blocks. `ALTER TABLE IF EXISTS ...
-- ADD COLUMN IF NOT EXISTS` is a safe no-op on the legacy upgrade path where a
-- table (e.g. tts_providers/tts_models) does not exist. The table list is the
-- applied fresh-install tenant set (51 tables), excluding schema_migrations
-- tooling and the tenants root.

ALTER TABLE IF EXISTS public.bot_acl_rules ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_channel_admins ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_channel_configs ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_channel_routes ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_email_bindings ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_heartbeat_logs ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_history_message_assets ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_history_message_compacts ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_history_messages ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_plugin_installations ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_plugin_resources ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_session_discuss_cursors ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_session_events ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_sessions ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_storage_bindings ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_user_grants ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bot_workspace_resource_limits ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.bots ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.channel_identities ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.channel_link_codes ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.container_versions ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.containers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.email_oauth_tokens ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.email_outbox ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.email_providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.fetch_providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.lifecycle_events ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.mcp_connections ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.mcp_oauth_tokens ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.media_assets ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.memory_edges ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.memory_nodes ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.memory_providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.model_variants ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.models ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.provider_oauth_tokens ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.schedule ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.schedule_logs ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.search_providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.snapshots ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.storage_providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.tasks ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.tool_approval_requests ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.tts_models ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.tts_providers ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.user_channel_bindings ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.user_channel_identity_bindings ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.user_input_requests ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.user_provider_oauth_tokens ADD COLUMN IF NOT EXISTS tenant_id uuid;
ALTER TABLE IF EXISTS public.users ADD COLUMN IF NOT EXISTS tenant_id uuid;


-- Give tenant_id a DEFAULT so any INSERT (sqlc-generated or raw) auto-fills the
-- current tenant from the session/transaction GUC. Fail-closed: if the GUC is
-- unset, app.current_tenant_id() raises rather than inserting a NULL/guessed tenant.
ALTER TABLE IF EXISTS public.bot_acl_rules ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_channel_admins ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_channel_configs ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_channel_routes ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_email_bindings ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_heartbeat_logs ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_history_message_assets ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_history_message_compacts ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_history_messages ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_plugin_installations ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_plugin_resources ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_session_discuss_cursors ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_session_events ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_sessions ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_storage_bindings ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_user_grants ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bot_workspace_resource_limits ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.bots ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.channel_identities ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.channel_link_codes ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.container_versions ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.containers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.email_oauth_tokens ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.email_outbox ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.email_providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.fetch_providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.lifecycle_events ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.mcp_connections ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.mcp_oauth_tokens ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.media_assets ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.memory_edges ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.memory_nodes ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.memory_providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.model_variants ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.models ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.provider_oauth_tokens ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.schedule ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.schedule_logs ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.search_providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.snapshots ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.storage_providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.tasks ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.tool_approval_requests ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.tts_models ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.tts_providers ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.user_channel_bindings ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.user_channel_identity_bindings ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.user_input_requests ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.user_provider_oauth_tokens ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();
ALTER TABLE IF EXISTS public.users ALTER COLUMN tenant_id SET DEFAULT app.current_tenant_id();

-- Backfill every present tenant table to the default singleton. Dynamic because
-- sqlc ignores UPDATE statements; enumerating the applied schema keeps this
-- correct across the fresh/legacy path divergence.
DO $$
DECLARE
    tbl text;
    default_tenant constant uuid := '00000000-0000-0000-0000-000000000001';
BEGIN
    FOR tbl IN
        SELECT c.relname
          FROM pg_catalog.pg_class c
          JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
         WHERE n.nspname = 'public'
           AND c.relkind IN ('r', 'p')
           AND EXISTS (
               SELECT 1
                 FROM pg_attribute a
                 JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
                WHERE a.attrelid = c.oid
                  AND a.attname = 'tenant_id'
                  AND NOT a.attisdropped
                  AND pg_get_expr(d.adbin, d.adrelid) LIKE '%app.current_tenant_id()%'
           )
         ORDER BY c.relname
    LOOP
        EXECUTE format('UPDATE public.%I SET tenant_id = %L WHERE tenant_id IS NULL', tbl, default_tenant);
    END LOOP;
END
$$;
