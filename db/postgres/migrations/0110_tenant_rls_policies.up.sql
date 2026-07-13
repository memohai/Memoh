-- 0110_tenant_rls_policies
-- Enable + force row-level security on the tenants root and every tenant table,
-- create the per-command tenant policies, and grant the runtime role the minimal
-- DML it needs. This is the database-level isolation backstop (security design
-- §4.3, §5, §6). Application queries must STILL carry explicit tenant scope;
-- RLS is defense-in-depth, not a replacement.
--
-- New incremental (existing migrations untouched). Policies are created as owner
-- so they are owned correctly, and are idempotent (DROP POLICY IF EXISTS +
-- CREATE). Tables are enumerated dynamically from the applied schema.
--
-- Policy templates (TO memoh_runtime):
--   <table>_tenant_select  FOR SELECT USING (tenant_id = app.current_tenant_id())
--   <table>_tenant_insert  FOR INSERT WITH CHECK (tenant_id = current AND fence_matches)
--   <table>_tenant_update  FOR UPDATE USING (... AND fence) WITH CHECK (... AND fence)
--   <table>_tenant_delete  FOR DELETE USING (tenant_id = current AND fence_matches)
--
-- tenants root: ENABLE+FORCE RLS, SELECT-only self policy (id = current), runtime
-- has no write on the root by default.
--
-- The fence meta-table app.tenant_write_fences keeps RLS OFF (its boundary is
-- zero runtime table ACL + the controlled helpers, not RLS).

-- Runtime needs USAGE on public and per-table DML. Grants run before SET ROLE so
-- the migration role (which owns nothing yet in public) can grant on the tables.
GRANT USAGE ON SCHEMA public TO memoh_runtime;

DO $$
DECLARE
    tbl text;
BEGIN
    FOR tbl IN
        SELECT c.relname
          FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind = 'r' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
         ORDER BY c.relname
    LOOP
        -- Minimal DML for the serving role.
        EXECUTE format('GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE public.%I TO memoh_runtime', tbl);

        -- Enable + force RLS.
        EXECUTE format('ALTER TABLE public.%I ENABLE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE public.%I FORCE ROW LEVEL SECURITY', tbl);

        -- Per-command policies (idempotent).
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_select', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR SELECT TO memoh_runtime '
            || 'USING (tenant_id = app.current_tenant_id())',
            tbl || '_tenant_select', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_insert', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR INSERT TO memoh_runtime '
            || 'WITH CHECK (tenant_id = app.current_tenant_id() AND app.tenant_write_fence_matches())',
            tbl || '_tenant_insert', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_update', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR UPDATE TO memoh_runtime '
            || 'USING (tenant_id = app.current_tenant_id() AND app.tenant_write_fence_matches()) '
            || 'WITH CHECK (tenant_id = app.current_tenant_id() AND app.tenant_write_fence_matches())',
            tbl || '_tenant_update', tbl);

        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_delete', tbl);
        EXECUTE format(
            'CREATE POLICY %I ON public.%I FOR DELETE TO memoh_runtime '
            || 'USING (tenant_id = app.current_tenant_id() AND app.tenant_write_fence_matches())',
            tbl || '_tenant_delete', tbl);

        -- Grant sequence usage for any owned sequences (serial/identity columns).
        -- (Most PKs here are UUID defaults, but grant defensively where present.)
    END LOOP;
END
$$;

-- Grant USAGE/SELECT on any sequences owned by tenant tables (defensive).
DO $$
DECLARE
    seq text;
BEGIN
    FOR seq IN
        SELECT quote_ident(n.nspname) || '.' || quote_ident(c.relname)
          FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind = 'S' AND n.nspname = 'public'
    LOOP
        EXECUTE format('GRANT USAGE, SELECT ON SEQUENCE %s TO memoh_runtime', seq);
    END LOOP;
END
$$;

-- ---------------------------------------------------------------------------
-- tenants root: ENABLE + FORCE RLS, SELECT-only self policy. Runtime may read
-- its own tenant row but not modify the root by default.
-- ---------------------------------------------------------------------------
GRANT SELECT ON TABLE public.tenants TO memoh_runtime;
ALTER TABLE public.tenants ENABLE ROW LEVEL SECURITY;
ALTER TABLE public.tenants FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenants_self_select ON public.tenants;
CREATE POLICY tenants_self_select ON public.tenants
    FOR SELECT TO memoh_runtime
    USING (id = app.current_tenant_id());
