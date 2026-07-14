-- 0110_tenant_rls_policies (down)
-- Remove tenant policies and disable row-level security.

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
    LOOP
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_select', tbl);
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_insert', tbl);
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_update', tbl);
        EXECUTE format('DROP POLICY IF EXISTS %I ON public.%I', tbl || '_tenant_delete', tbl);
        EXECUTE format('ALTER TABLE public.%I NO FORCE ROW LEVEL SECURITY', tbl);
        EXECUTE format('ALTER TABLE public.%I DISABLE ROW LEVEL SECURITY', tbl);
    END LOOP;
END
$$;

DROP POLICY IF EXISTS tenants_self_select ON public.tenants;
ALTER TABLE public.tenants NO FORCE ROW LEVEL SECURITY;
ALTER TABLE public.tenants DISABLE ROW LEVEL SECURITY;
