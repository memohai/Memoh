-- 0106_tenants_root (down)
-- Reverse the tenants root introduction.
--
-- Down safety gate: only allow rollback when the database is still a clean
-- singleton — i.e. there is no tenant other than the default. If any
-- non-default tenant exists, dropping the root would orphan tenant data, so we
-- fail closed rather
-- than silently destroy data. (A wipe is never an acceptable rollback.)

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM tenants
        WHERE id <> '00000000-0000-0000-0000-000000000001'::uuid
    ) THEN
        RAISE EXCEPTION
            'refusing to drop tenants root: non-default tenant rows present (multi-tenant database)';
    END IF;
END
$$;

DROP INDEX IF EXISTS tenants_slug_unique;
DROP TABLE IF EXISTS tenants;
