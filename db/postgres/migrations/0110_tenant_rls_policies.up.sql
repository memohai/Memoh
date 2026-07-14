-- 0110_tenant_rls_policies
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
