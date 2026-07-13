-- 0108_tenant_id_nullable_backfill
-- Add a nullable tenant_id column to every tenant business table (STATIC ALTERs
-- so sqlc can see the column) and backfill it to the default singleton tenant
-- (schema contract phases A/B).
--
-- New incremental (existing migrations untouched). tenant_id is added NULLABLE
-- and backfilled to DEFAULT_TENANT_ID; a later migration tightens it to NOT NULL
-- after composite keys/FKs land. Existing installs upgrade in place (no wipe).
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
           AND c.relkind = 'r'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
           AND EXISTS (SELECT 1 FROM information_schema.columns
                        WHERE table_schema = 'public' AND table_name = c.relname
                          AND column_name = 'tenant_id')
         ORDER BY c.relname
    LOOP
        EXECUTE format('UPDATE public.%I SET tenant_id = %L WHERE tenant_id IS NULL', tbl, default_tenant);
    END LOOP;
END
$$;
