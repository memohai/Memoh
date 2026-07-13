-- 0108_tenant_id_nullable_backfill
-- Add a nullable tenant_id column to every applied tenant business table and
-- backfill it to the default singleton tenant (schema contract phases A/B).
--
-- New incremental (existing migrations untouched). We enumerate tenant tables
-- DYNAMICALLY from information_schema rather than hard-coding a list, because
-- the applied table set is path-dependent (e.g. tts_providers/tts_models
-- survive a fresh install but are dropped on a legacy upgrade, since
-- 0061_unify_providers early-returns on fresh DBs). Enumerating the live schema
-- makes this correct on whichever path the target database took.
--
-- Excluded from tenantization:
--   * schema_migrations  — golang-migrate tooling metadata (global)
--   * tenants            — the root: its own id IS the tenant id
--   * everything outside the public schema (app.tenant_write_fences etc.)
--
-- tenant_id is added NULLABLE here and backfilled to DEFAULT_TENANT_ID. A later
-- migration tightens it to NOT NULL after composite keys/FKs are in place. This
-- keeps existing installs upgradable in place (no wipe): existing rows are
-- assigned to the single default tenant.

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
         ORDER BY c.relname
    LOOP
        EXECUTE format(
            'ALTER TABLE public.%I ADD COLUMN IF NOT EXISTS tenant_id uuid',
            tbl
        );
        EXECUTE format(
            'UPDATE public.%I SET tenant_id = %L WHERE tenant_id IS NULL',
            tbl, default_tenant
        );
    END LOOP;
END
$$;
