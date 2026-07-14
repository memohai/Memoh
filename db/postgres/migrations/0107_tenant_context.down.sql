-- 0107_tenant_context (down)
-- Remove the tenant context helper and its schema when empty.

DROP FUNCTION IF EXISTS app.current_tenant_id();
DROP SCHEMA IF EXISTS app;
