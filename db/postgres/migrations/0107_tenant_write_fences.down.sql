-- 0107_tenant_write_fences (down)
-- Reverse the write-fence security core in dependency order.
--
-- Down safety gate: only allow rollback on a clean singleton database (no
-- non-default fence rows), matching 0106's gate. Fail closed for multi-tenant.
-- Roles are left in place if they still own objects created by later work; we
-- only DROP them when safe. We do not drop the app schema's future objects.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM app.tenant_write_fences
        WHERE tenant_id <> '00000000-0000-0000-0000-000000000001'::uuid
    ) THEN
        RAISE EXCEPTION
            'refusing to drop tenant_write_fences: non-default fence rows present (multi-tenant database)';
    END IF;
EXCEPTION
    WHEN undefined_table THEN
        NULL; -- table already gone; nothing to gate
END
$$;

DROP FUNCTION IF EXISTS app.cas_tenant_write_fence(uuid, bigint, bigint, boolean);
DROP FUNCTION IF EXISTS app.tenant_write_fence_matches();
DROP FUNCTION IF EXISTS app.assert_tenant_write_fence();
DROP FUNCTION IF EXISTS app.current_fencing_token();
DROP FUNCTION IF EXISTS app.current_tenant_id();

DROP TRIGGER IF EXISTS tenant_write_fences_guard ON app.tenant_write_fences;
DROP FUNCTION IF EXISTS app.tenant_write_fences_guard();
DROP TABLE IF EXISTS app.tenant_write_fences;

DROP SCHEMA IF EXISTS app;

-- Drop roles only if they own nothing else. Reassign is unsafe here, so guard.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_break_glass') THEN
        DROP ROLE memoh_break_glass;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_runtime') THEN
        DROP ROLE memoh_runtime;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_migrator') THEN
        DROP ROLE memoh_migrator;
    END IF;
    IF EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_owner') THEN
        DROP ROLE memoh_owner;
    END IF;
EXCEPTION
    WHEN dependent_objects_still_exist THEN
        RAISE NOTICE 'tenant roles still own objects; leaving roles in place';
END
$$;
