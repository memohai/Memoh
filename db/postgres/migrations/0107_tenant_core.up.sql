-- 0107_tenant_core
-- Add the singleton tenant root, tenant-scoped schema, query context, RLS,
-- tenant-safe view, and tenant-prefixed indexes.

-- ---------------------------------------------------------------------------
-- Tenant root
-- ---------------------------------------------------------------------------
-- Introduce the tenants root table and seed the default singleton tenant.
--
-- This is the first migration of the tenant-core work. tenants is the unique
-- root special case: its own id IS the tenant id, so it must NOT carry a
-- redundant tenant_id column. Existing installs upgrade in place (no wipe): the
-- single default tenant is seeded idempotently and every existing business row
-- is later backfilled to DefaultTenantID by subsequent migrations.
--
-- DefaultTenantID is the fixed constant published in internal/tenant/id.go
-- (00000000-0000-0000-0000-000000000001). It must never be generated per install.

CREATE TABLE IF NOT EXISTS tenants (
    id         UUID        PRIMARY KEY,
    slug       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata   JSONB       NOT NULL DEFAULT '{}'::jsonb
);

-- slug is an optional directory field, not an identity/authorization boundary.
-- When present it must be unique cell-wide; NULL slugs are allowed and excluded.
CREATE UNIQUE INDEX IF NOT EXISTS tenants_slug_unique ON tenants (slug) WHERE slug IS NOT NULL;

-- Seed the singleton tenant idempotently. Existing self-hosted installations
-- continue to use this tenant without any configuration changes.
INSERT INTO tenants (id, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'default')
ON CONFLICT (id) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Tenant context
-- ---------------------------------------------------------------------------
-- Add the fail-closed tenant context helper used by tenant-scoped queries and
-- row-level security policies.

CREATE SCHEMA IF NOT EXISTS app;

CREATE OR REPLACE FUNCTION app.current_tenant_id()
RETURNS uuid
LANGUAGE plpgsql
STABLE
SECURITY INVOKER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
  raw text;
BEGIN
  raw := pg_catalog.current_setting('app.tenant_id', true);
  IF raw IS NULL OR pg_catalog.btrim(raw) = '' THEN
    RAISE EXCEPTION 'app.tenant_id is not set'
      USING ERRCODE = '42501';
  END IF;
  BEGIN
    RETURN raw::uuid;
  EXCEPTION
    WHEN invalid_text_representation THEN
      RAISE EXCEPTION 'app.tenant_id is invalid'
        USING ERRCODE = '22P02';
  END;
END;
$$;

-- ---------------------------------------------------------------------------
-- Tenant columns and singleton backfill
-- ---------------------------------------------------------------------------
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

-- ---------------------------------------------------------------------------
-- Tenant-scoped keys and foreign keys
-- ---------------------------------------------------------------------------
-- Add tenant-scoped unique keys and composite foreign keys while preserving the
-- existing primary keys and delete behavior.
--
-- This work is atomic because in PostgreSQL an FK binds to a specific unique
-- index: you cannot rebuild a referenced key without dropping and recreating its
-- dependent FKs in the same operation. Splitting across migrations would fight
-- that coupling; doing it atomically is both correct and simpler to reason about.
--
-- Preconditions (from earlier sections): every tenant table already has a
-- backfilled tenant_id and the tenants root exists. Tenant tables are identified
-- by the tenant default installed above, so user-managed
-- tables in the public schema are not modified.
--
-- Existing ON DELETE SET NULL constraints use PostgreSQL's column-list form,
-- SET NULL (child_column), so the reference is cleared without clearing the
-- non-null tenant_id column.

DO $$
DECLARE
    rec record;
    cols text;
    delete_action text;
    update_action text;
BEGIN
    CREATE TEMP TABLE _tenant_tables (table_name text PRIMARY KEY) ON COMMIT DROP;
    INSERT INTO _tenant_tables
    SELECT c.relname
      FROM pg_class c
      JOIN pg_namespace n ON n.oid = c.relnamespace
      JOIN pg_attribute a ON a.attrelid = c.oid
      JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
     WHERE n.nspname = 'public'
       AND c.relkind IN ('r', 'p')
       AND a.attname = 'tenant_id'
       AND NOT a.attisdropped
       AND pg_get_expr(d.adbin, d.adrelid) LIKE '%app.current_tenant_id()%';

    CREATE TEMP TABLE _fk_saved ON COMMIT DROP AS
    SELECT con.oid,
           c.relname            AS child_table,
           con.conname          AS fk_name,
           rt.relname           AS parent_table,
           con.confdeltype      AS del_type,
           con.confupdtype      AS upd_type,
           (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.conrelid AND a.attnum = con.conkey[1])  AS child_col,
           (SELECT a.attname FROM pg_attribute a
             WHERE a.attrelid = con.confrelid AND a.attnum = con.confkey[1]) AS parent_col,
           cardinality(con.conkey) AS ncols
      FROM pg_constraint con
      JOIN pg_class c  ON c.oid  = con.conrelid
      JOIN pg_class rt ON rt.oid = con.confrelid
      JOIN pg_namespace n ON n.oid = c.relnamespace
     WHERE con.contype = 'f'
       AND n.nspname = 'public'
       AND c.relname IN (SELECT table_name FROM _tenant_tables)
       AND rt.relname IN (SELECT table_name FROM _tenant_tables)
       AND cardinality(con.conkey) = 1;

    -- Safety: this algorithm only handles single-column business FKs.
    IF EXISTS (SELECT 1 FROM _fk_saved WHERE ncols <> 1) THEN
        RAISE EXCEPTION 'multi-column FK present; composite re-key algorithm needs revision';
    END IF;

    IF EXISTS (SELECT 1 FROM _fk_saved WHERE upd_type IN ('n', 'd')) THEN
        RAISE EXCEPTION 'ON UPDATE SET NULL/DEFAULT requires an explicit tenant-safe migration';
    END IF;

    FOR rec IN SELECT child_table, fk_name FROM _fk_saved LOOP
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.child_table, rec.fk_name);
    END LOOP;

    -- Keep existing primary keys stable. Add a tenant-prefixed unique key for
    -- each primary key so composite foreign keys have a valid target.
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE con.contype = 'p' AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
    LOOP
        SELECT 'tenant_id, ' || string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
          INTO cols
          FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = rec.conrelid AND a.attnum = k.attnum;
        EXECUTE format('ALTER TABLE public.%I ALTER COLUMN tenant_id SET NOT NULL', rec.table_name);
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint
             WHERE conrelid = rec.conrelid
               AND conname = 'memoh_tenant_key_' || substr(md5(rec.table_name || ':' || rec.conname), 1, 12)
        ) THEN
            EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I UNIQUE (%s)',
                rec.table_name,
                'memoh_tenant_key_' || substr(md5(rec.table_name || ':' || rec.conname), 1, 12),
                cols);
        END IF;
    END LOOP;

    -- ===== Phase 3: rebuild UNIQUE constraints with tenant_id prepended =====
    FOR rec IN
        SELECT c.relname AS table_name, con.conname, con.conrelid, con.conkey,
               i.indnullsnotdistinct AS nulls_not_distinct
          FROM pg_constraint con
          JOIN pg_class c ON c.oid = con.conrelid
          JOIN pg_namespace n ON n.oid = c.relnamespace
          JOIN pg_index i ON i.indexrelid = con.conindid
         WHERE con.contype = 'u' AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
           AND con.conname NOT LIKE 'memoh_tenant_key_%'
    LOOP
        IF (SELECT attname FROM pg_attribute
              WHERE attrelid = rec.conrelid AND attnum = rec.conkey[1]) = 'tenant_id' THEN
            CONTINUE;
        END IF;
        SELECT 'tenant_id, ' || string_agg(quote_ident(a.attname), ', ' ORDER BY k.ord)
          INTO cols
          FROM unnest(rec.conkey) WITH ORDINALITY AS k(attnum, ord)
          JOIN pg_attribute a ON a.attrelid = rec.conrelid AND a.attnum = k.attnum;
        EXECUTE format('ALTER TABLE public.%I DROP CONSTRAINT %I', rec.table_name, rec.conname);
        -- Preserve NULLS NOT DISTINCT: a bare UNIQUE would widen the semantics
        -- (NULLs become distinct), letting duplicate rows with NULL key columns
        -- through (e.g. bot_acl_rules_unique_target).
        EXECUTE format('ALTER TABLE public.%I ADD CONSTRAINT %I UNIQUE %s (%s)',
                       rec.table_name, rec.conname,
                       CASE WHEN rec.nulls_not_distinct THEN 'NULLS NOT DISTINCT' ELSE '' END,
                       cols);
    END LOOP;

    FOR rec IN SELECT * FROM _fk_saved LOOP
        update_action := CASE rec.upd_type WHEN 'c' THEN 'CASCADE' WHEN 'r' THEN 'RESTRICT'
                          ELSE 'NO ACTION' END;
        delete_action := CASE rec.del_type
            WHEN 'c' THEN 'CASCADE'
            WHEN 'r' THEN 'RESTRICT'
            WHEN 'n' THEN format('SET NULL (%I)', rec.child_col)
            WHEN 'd' THEN format('SET DEFAULT (%I)', rec.child_col)
            ELSE 'NO ACTION'
        END;
        EXECUTE format(
            'ALTER TABLE public.%I ADD CONSTRAINT %I FOREIGN KEY (tenant_id, %I) '
            || 'REFERENCES public.%I (tenant_id, %I) ON UPDATE %s ON DELETE %s',
            rec.child_table, rec.fk_name, rec.child_col,
            rec.parent_table, rec.parent_col,
            update_action, delete_action
        );
    END LOOP;

    -- ===== Phase 4b: add root FK (tenant_id) -> tenants(id) on every table =====
    FOR rec IN
        SELECT c.relname AS table_name
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind IN ('r', 'p') AND n.nspname = 'public'
           AND c.relname IN (SELECT table_name FROM _tenant_tables)
    LOOP
        IF NOT EXISTS (
            SELECT 1 FROM pg_constraint con
              JOIN pg_class rt ON rt.oid = con.confrelid
             WHERE con.contype = 'f'
               AND con.conrelid = ('public.'||quote_ident(rec.table_name))::regclass
               AND rt.relname = 'tenants'
        ) THEN
            EXECUTE format(
                'ALTER TABLE public.%I ADD CONSTRAINT %I FOREIGN KEY (tenant_id) '
                || 'REFERENCES public.tenants (id) ON DELETE RESTRICT',
                rec.table_name, rec.table_name || '_tenant_id_fkey');
        END IF;
    END LOOP;
END
$$;

-- ===== Phase 3b: partial / expression unique indexes with tenant_id prepended =====
DROP INDEX IF EXISTS idx_bot_channel_external_identity;
CREATE UNIQUE INDEX idx_bot_channel_external_identity
    ON public.bot_channel_configs (tenant_id, channel_type, external_identity);

DROP INDEX IF EXISTS idx_bot_channel_routes_unique;
CREATE UNIQUE INDEX idx_bot_channel_routes_unique
    ON public.bot_channel_routes
       (tenant_id, bot_id, channel_type, external_conversation_id, COALESCE(external_thread_id, ''::text));

DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
CREATE UNIQUE INDEX idx_bot_history_messages_turn_seq_unique
    ON public.bot_history_messages (tenant_id, turn_id, turn_message_seq)
    WHERE ((turn_id IS NOT NULL) AND (turn_message_seq IS NOT NULL));

DROP INDEX IF EXISTS idx_bot_user_grants_unique_everyone;
CREATE UNIQUE INDEX idx_bot_user_grants_unique_everyone
    ON public.bot_user_grants (tenant_id, bot_id)
    WHERE (subject_type = 'everyone'::text);

DROP INDEX IF EXISTS idx_bot_user_grants_unique_user;
CREATE UNIQUE INDEX idx_bot_user_grants_unique_user
    ON public.bot_user_grants (tenant_id, bot_id, user_id)
    WHERE (subject_type = 'user'::text);

DROP INDEX IF EXISTS idx_bots_name;
CREATE UNIQUE INDEX idx_bots_name
    ON public.bots (tenant_id, name);

DROP INDEX IF EXISTS idx_session_events_dedup;
CREATE UNIQUE INDEX idx_session_events_dedup
    ON public.bot_session_events (tenant_id, session_id, event_kind, external_message_id)
    WHERE ((external_message_id IS NOT NULL) AND (external_message_id <> ''::text));

DROP INDEX IF EXISTS idx_snapshots_container_runtime_name;
CREATE UNIQUE INDEX idx_snapshots_container_runtime_name
    ON public.snapshots (tenant_id, container_id, runtime_snapshot_name);

-- ---------------------------------------------------------------------------
-- Tenant row-level security
-- ---------------------------------------------------------------------------
-- Enable and force row-level security on Memoh tenant tables. Policies apply
-- to the connecting role, so installations do not need cluster-wide roles or
-- elevated CREATEROLE/BYPASSRLS privileges.

DO $$
DECLARE
    tbl text;
BEGIN
    FOR tbl IN
        SELECT c.relname
          FROM pg_class c
          JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind IN ('r', 'p')
           AND n.nspname = 'public'
           AND EXISTS (
               SELECT 1
                 FROM pg_constraint con
                 JOIN pg_class parent ON parent.oid = con.confrelid
                WHERE con.conrelid = c.oid
                  AND con.contype = 'f'
                  AND parent.relnamespace = n.oid
                  AND parent.relname = 'tenants'
           )
         ORDER BY c.relname
    LOOP
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_select', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR SELECT USING (tenant_id = app.current_tenant_id())',
            tbl || '_tenant_select', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_insert', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR INSERT WITH CHECK (tenant_id = app.current_tenant_id())',
            tbl || '_tenant_insert', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_update', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR UPDATE '
            || 'USING (tenant_id = app.current_tenant_id()) '
            || 'WITH CHECK (tenant_id = app.current_tenant_id())',
            tbl || '_tenant_update', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_delete', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR DELETE USING (tenant_id = app.current_tenant_id())',
            tbl || '_tenant_delete', tbl);
    END LOOP;
END
$$;

ALTER TABLE public.tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.tenants FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenants_self_select ON public.tenants;
CREATE POLICY tenants_self_select ON public.tenants
    FOR SELECT USING (id = app.current_tenant_id());

-- ---------------------------------------------------------------------------
-- Tenant-safe history view
-- ---------------------------------------------------------------------------
-- Fix the bot_visible_history_messages view so it cannot bypass tenant RLS.
--
-- The view was created without security_invoker, so it executed with its
-- owner's privileges and could bypass the caller's RLS policies. This migration:
--   1. recreates the view WITH (security_invoker = true) so it runs under the
--      caller's privileges — the base table's RLS then scopes it automatically;
--   2. projects tenant_id so consuming queries can carry explicit scope
--      (defense-in-depth) and so the schema guard can verify the view;

-- Adding tenant_id as the first projected column changes column order, which
-- CREATE OR REPLACE VIEW rejects, so drop and recreate. No other object depends
-- on this view (verified), so a plain DROP is safe.
DROP VIEW IF EXISTS bot_visible_history_messages;

CREATE VIEW bot_visible_history_messages
WITH (security_invoker = true) AS
SELECT
  m.tenant_id,
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

-- ---------------------------------------------------------------------------
-- Tenant-prefixed secondary indexes
-- ---------------------------------------------------------------------------
-- Prepend tenant_id to every non-unique btree secondary index on a tenant table
-- that does not already lead with it. Tenant queries filter by tenant_id (RLS +
-- explicit app.current_tenant_id()), so a tenant_id-leading index lets the
-- planner scan only the current tenant's slice. Purely a performance change.
--
-- New incremental (existing migrations untouched). Each index is dropped and
-- recreated with tenant_id prepended, preserving its column list, partial WHERE,
-- and (btree) access method. Non-btree (gin/gist) indexes are left untouched.

DO $$
DECLARE
    rec record;
    idxdef text;
BEGIN
    FOR rec IN
        SELECT ic.relname AS index_name, pg_get_indexdef(i.indexrelid) AS def
          FROM pg_index i
          JOIN pg_class ic ON ic.oid = i.indexrelid
          JOIN pg_class tc ON tc.oid = i.indrelid
          JOIN pg_namespace n ON n.oid = tc.relnamespace
          JOIN pg_am am ON am.oid = ic.relam
         WHERE n.nspname = 'public'
           AND NOT i.indisprimary AND NOT i.indisunique
           AND am.amname = 'btree'
           AND EXISTS (
               SELECT 1
                 FROM pg_constraint con
                 JOIN pg_class parent ON parent.oid = con.confrelid
                WHERE con.conrelid = tc.oid
                  AND con.contype = 'f'
                  AND parent.relnamespace = n.oid
                  AND parent.relname = 'tenants'
           )
           AND (SELECT attname FROM pg_attribute
                 WHERE attrelid = i.indrelid AND attnum = i.indkey[0]) <> 'tenant_id'
    LOOP
        idxdef := regexp_replace(rec.def, '(USING btree \()', '\1tenant_id, ');
        EXECUTE format('DROP INDEX IF EXISTS public.%I', rec.index_name);
        EXECUTE idxdef;
    END LOOP;
END
$$;
