-- 0108_tenant_id_nullable_backfill (down)
-- Drop the tenant_id column from every tenant business table.
--
-- Down safety gate: only allow rollback on a clean singleton database — no
-- row in any tenant table may carry a non-default tenant_id. If a non-default
-- tenant's data exists, dropping tenant_id would destroy the only isolation
-- marker, so we fail closed (never wipe as rollback).

DO $$
DECLARE
    tbl text;
    default_tenant constant uuid := '00000000-0000-0000-0000-000000000001';
    bad bigint;
BEGIN
    -- Fail-closed gate: refuse if any tenant table holds non-default tenant_id.
    FOR tbl IN
        SELECT c.relname
          FROM pg_catalog.pg_class c
          JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
         WHERE n.nspname = 'public'
           AND c.relkind = 'r'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
           AND EXISTS (
               SELECT 1 FROM information_schema.columns
                WHERE table_schema = 'public' AND table_name = c.relname
                  AND column_name = 'tenant_id'
           )
    LOOP
        EXECUTE format(
            'SELECT count(*) FROM public.%I WHERE tenant_id IS NOT NULL AND tenant_id <> %L',
            tbl, default_tenant
        ) INTO bad;
        IF bad > 0 THEN
            RAISE EXCEPTION
                'refusing to drop tenant_id from %: % rows carry a non-default tenant_id (multi-tenant database)',
                tbl, bad;
        END IF;
    END LOOP;

    -- Safe: drop the column everywhere it exists.
    FOR tbl IN
        SELECT c.relname
          FROM pg_catalog.pg_class c
          JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
         WHERE n.nspname = 'public'
           AND c.relkind = 'r'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
         ORDER BY c.relname
    LOOP
        EXECUTE format('ALTER TABLE public.%I DROP COLUMN IF EXISTS tenant_id', tbl);
    END LOOP;
END
$$;
