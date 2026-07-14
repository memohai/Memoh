-- 0107_tenant_context
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
