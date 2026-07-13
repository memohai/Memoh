-- 0110_tenant_rls_policies (down)
-- Reverse RLS: drop the per-command policies, disable/no-force RLS, and revoke
-- the runtime DML grants on tenant tables and the tenants root.

DO $$
DECLARE
    tbl text;
BEGIN
    FOR tbl IN
        SELECT c.relname
          FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind = 'r' AND n.nspname = 'public'
           AND c.relname NOT IN ('schema_migrations', 'tenants')
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_select', tbl);
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_insert', tbl);
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_update', tbl);
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_delete', tbl);
        EXECUTE format('ALTER TABLE public.%I NO FORCE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE public.%I DISABLE ROW LEVEL SECURITY', tbl);
        EXECUTE format('REVOKE SELECT, INSERT, UPDATE, DELETE ON TABLE public.%I FROM memoh_runtime', tbl);
    END LOOP;

    FOR tbl IN
        SELECT quote_ident(n.nspname) || '.' || quote_ident(c.relname)
          FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
         WHERE c.relkind = 'S' AND n.nspname = 'public'
    LOOP
        EXECUTE format('REVOKE USAGE, SELECT ON SEQUENCE %s FROM memoh_runtime', tbl);
    END LOOP;
END
$$;

DROP POLICY IF EXISTS tenants_self_select ON public.tenants;
ALTER TABLE public.tenants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE public.tenants DISABLE ROW LEVEL SECURITY;
REVOKE SELECT ON TABLE public.tenants FROM memoh_runtime;
