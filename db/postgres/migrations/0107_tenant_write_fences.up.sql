-- 0107_tenant_write_fences
-- Introduce the tenant write-fence security meta-table, the DB roles, the app
-- schema, the fail-closed context helpers, the write-fence assert/matches
-- helpers, and the controlled CAS management function.
--
-- New incremental (existing migrations untouched). This is the security core of
-- the tenant-isolation contract (tenant-isolation-security-design.md §4, §6).
-- Token semantics are STRICT EQUALITY: only supplied_token = stored_token AND
-- write_enabled = true may write. No minimum/>=/window semantics.
--
-- DDL runs as the migration role. To keep object ownership aligned with the
-- security contract (owner = memoh_owner, NOLOGIN), we SET ROLE memoh_owner for
-- object creation where ownership matters and RESET at the end. Roles are
-- created idempotently and without a login password here; deployment/tests
-- assign credentials out of band. Wiring the *serving* connection to
-- memoh_runtime is a Phase 2 concern (handed off), not done here.

-- ---------------------------------------------------------------------------
-- Roles (idempotent). Attributes per security design §4.2/§4.3.
-- owner: NOLOGIN, non-superuser, owns objects.
-- migrator: LOGIN, BYPASSRLS (own attribute), NOINHERIT.
-- runtime: LOGIN, NOSUPERUSER, NOBYPASSRLS, NOINHERIT (serving role).
-- break_glass: NOLOGIN (manual, time-limited only).
-- ---------------------------------------------------------------------------
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_owner') THEN
        CREATE ROLE memoh_owner NOLOGIN NOSUPERUSER NOBYPASSRLS;
    ELSE
        ALTER ROLE memoh_owner NOLOGIN NOSUPERUSER NOBYPASSRLS;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_migrator') THEN
        CREATE ROLE memoh_migrator LOGIN NOSUPERUSER BYPASSRLS NOINHERIT;
    ELSE
        ALTER ROLE memoh_migrator NOSUPERUSER BYPASSRLS NOINHERIT;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_runtime') THEN
        CREATE ROLE memoh_runtime LOGIN NOSUPERUSER NOBYPASSRLS NOINHERIT;
    ELSE
        ALTER ROLE memoh_runtime NOSUPERUSER NOBYPASSRLS NOINHERIT;
    END IF;

    IF NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = 'memoh_break_glass') THEN
        CREATE ROLE memoh_break_glass NOLOGIN;
    ELSE
        ALTER ROLE memoh_break_glass NOLOGIN;
    END IF;
END
$$;

-- Grant owner membership to migrator for explicit SET ROLE (no auto-inherit).
GRANT memoh_owner TO memoh_migrator;
-- Allow the current bootstrap/migration role to assume owner for this migration.
GRANT memoh_owner TO CURRENT_USER;

-- memoh_owner needs access to the public schema to create the fence table's FK
-- to public.tenants (USAGE on schema, REFERENCES on the parent table).
GRANT USAGE ON SCHEMA public TO memoh_owner;
GRANT REFERENCES ON TABLE public.tenants TO memoh_owner;

-- ---------------------------------------------------------------------------
-- app schema (protected security-function/metadata schema), owned by owner.
-- ---------------------------------------------------------------------------
CREATE SCHEMA IF NOT EXISTS app;
ALTER SCHEMA app OWNER TO memoh_owner;
REVOKE CREATE ON SCHEMA app FROM PUBLIC, memoh_runtime;
GRANT USAGE ON SCHEMA app TO memoh_runtime, memoh_migrator;

-- Owner-created objects default to no PUBLIC access.
ALTER DEFAULT PRIVILEGES FOR ROLE memoh_owner IN SCHEMA public
  REVOKE ALL ON TABLES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE memoh_owner IN SCHEMA public
  REVOKE ALL ON SEQUENCES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE memoh_owner IN SCHEMA app
  REVOKE ALL ON TABLES FROM PUBLIC;
ALTER DEFAULT PRIVILEGES FOR ROLE memoh_owner IN SCHEMA app
  REVOKE ALL ON FUNCTIONS FROM PUBLIC;

-- Create objects as owner so ownership matches the contract.
SET ROLE memoh_owner;

-- ---------------------------------------------------------------------------
-- tenant_write_fences: global security meta-table. NOT a tenant table; no RLS.
-- Security boundary = zero runtime/PUBLIC table privileges + controlled helpers.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS app.tenant_write_fences (
    tenant_id     uuid        PRIMARY KEY REFERENCES public.tenants(id) ON DELETE RESTRICT,
    fencing_token bigint      NOT NULL CHECK (fencing_token > 0),
    write_enabled boolean     NOT NULL DEFAULT true,
    updated_at    timestamptz NOT NULL DEFAULT pg_catalog.now()
);
ALTER TABLE app.tenant_write_fences DISABLE ROW LEVEL SECURITY;

-- ---------------------------------------------------------------------------
-- DB-enforced monotonic contract (schema contract §5.3):
--  - fencing_token > 0 (also the table CHECK)
--  - tenant_id immutable on UPDATE
--  - fencing_token must never decrease
--  - when the token grows, the new state must be write_enabled = false
--  - false -> true only allowed when the token is unchanged
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION app.tenant_write_fences_guard()
RETURNS trigger
LANGUAGE plpgsql
SET search_path = pg_catalog, pg_temp
AS $$
BEGIN
    IF NEW.fencing_token <= 0 THEN
        RAISE EXCEPTION 'fencing_token must be positive' USING ERRCODE = '23514';
    END IF;

    IF TG_OP = 'UPDATE' THEN
        IF NEW.tenant_id <> OLD.tenant_id THEN
            RAISE EXCEPTION 'tenant_id is immutable' USING ERRCODE = '23514';
        END IF;
        IF NEW.fencing_token < OLD.fencing_token THEN
            RAISE EXCEPTION 'fencing_token must not decrease' USING ERRCODE = '23514';
        END IF;
        IF NEW.fencing_token > OLD.fencing_token AND NEW.write_enabled THEN
            RAISE EXCEPTION 'advancing fencing_token must land write_enabled = false'
                USING ERRCODE = '23514';
        END IF;
        -- Enabling (false -> true) is only reachable with an unchanged token,
        -- because the rule above forbids write_enabled = true whenever the token
        -- grows. No separate check is needed.
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE TRIGGER tenant_write_fences_guard
    BEFORE INSERT OR UPDATE ON app.tenant_write_fences
    FOR EACH ROW EXECUTE FUNCTION app.tenant_write_fences_guard();

-- ---------------------------------------------------------------------------
-- Fail-closed context helpers (SECURITY INVOKER). §6.2 / §6.3.
-- ---------------------------------------------------------------------------
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

CREATE OR REPLACE FUNCTION app.current_fencing_token()
RETURNS bigint
LANGUAGE plpgsql
STABLE
SECURITY INVOKER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
  raw_token text;
  supplied_token bigint;
BEGIN
  raw_token := pg_catalog.current_setting('app.fencing_token', true);
  IF raw_token IS NULL OR pg_catalog.btrim(raw_token) = '' THEN
    RAISE EXCEPTION 'FENCE_CONTEXT_MISSING'
      USING ERRCODE = '42501';
  END IF;

  BEGIN
    supplied_token := raw_token::bigint;
  EXCEPTION
    WHEN invalid_text_representation OR numeric_value_out_of_range THEN
      RAISE EXCEPTION 'FENCE_CONTEXT_MISSING'
        USING ERRCODE = '42501';
  END;

  IF supplied_token <= 0 THEN
    RAISE EXCEPTION 'FENCE_CONTEXT_MISSING'
      USING ERRCODE = '42501';
  END IF;

  RETURN supplied_token;
END;
$$;

-- ---------------------------------------------------------------------------
-- Write-transaction entry hard check (SECURITY DEFINER, holds FOR SHARE lock).
-- Does NOT catch/rewrite current-helper exceptions. §6.5.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION app.assert_tenant_write_fence()
RETURNS void
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
  current_tenant uuid := app.current_tenant_id();
  supplied_token bigint := app.current_fencing_token();
  expected_token bigint;
  enabled boolean;
BEGIN
  SELECT f.fencing_token, f.write_enabled
    INTO expected_token, enabled
    FROM app.tenant_write_fences AS f
   WHERE f.tenant_id = current_tenant
   FOR SHARE;

  IF NOT FOUND OR NOT enabled THEN
    RAISE EXCEPTION 'TENANT_WRITE_DISABLED'
      USING ERRCODE = '42501';
  END IF;

  IF expected_token <> supplied_token THEN
    RAISE EXCEPTION 'FENCE_TOKEN_STALE'
      USING ERRCODE = '42501';
  END IF;
END;
$$;

-- RLS defense-in-depth: resolve context (propagating errors) then lock-free,
-- side-effect-free boolean. §6.5.
CREATE OR REPLACE FUNCTION app.tenant_write_fence_matches()
RETURNS boolean
LANGUAGE plpgsql
STABLE
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
  current_tenant uuid := app.current_tenant_id();
  supplied_token bigint := app.current_fencing_token();
BEGIN
  RETURN COALESCE((
    SELECT f.fencing_token = supplied_token AND f.write_enabled
      FROM app.tenant_write_fences AS f
     WHERE f.tenant_id = current_tenant
  ), false);
END;
$$;

-- ---------------------------------------------------------------------------
-- Controlled CAS management (§6.5.1). Success = exactly one row affected.
-- advance: only old -> old+1 AND hard-disable. enable/disable: exact token only.
-- Granted to migrator only; runtime/PUBLIC/break-glass have NO execute.
-- ---------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION app.cas_tenant_write_fence(
    p_tenant_id   uuid,
    p_expected    bigint,
    p_next_token  bigint,
    p_next_enabled boolean
)
RETURNS boolean
LANGUAGE plpgsql
VOLATILE
SECURITY DEFINER
SET search_path = pg_catalog, pg_temp
AS $$
DECLARE
  affected bigint;
BEGIN
  IF p_next_token < p_expected THEN
    RAISE EXCEPTION 'cas: next token must not be lower than expected'
      USING ERRCODE = '23514';
  END IF;
  -- advance must be exactly +1 and land disabled; same-token change keeps token.
  IF p_next_token = p_expected THEN
    UPDATE app.tenant_write_fences
       SET write_enabled = p_next_enabled,
           updated_at = pg_catalog.now()
     WHERE tenant_id = p_tenant_id
       AND fencing_token = p_expected;
  ELSIF p_next_token = p_expected + 1 THEN
    IF p_next_enabled THEN
      RAISE EXCEPTION 'cas: advancing token must land write_enabled = false'
        USING ERRCODE = '23514';
    END IF;
    UPDATE app.tenant_write_fences
       SET fencing_token = p_expected + 1,
           write_enabled = false,
           updated_at = pg_catalog.now()
     WHERE tenant_id = p_tenant_id
       AND fencing_token = p_expected;
  ELSE
    RAISE EXCEPTION 'cas: token may only advance by exactly one (no skipping)'
      USING ERRCODE = '23514';
  END IF;

  GET DIAGNOSTICS affected = ROW_COUNT;
  RETURN affected = 1;
END;
$$;

RESET ROLE;

-- ---------------------------------------------------------------------------
-- Ownership + ACL closeout (§6.5 invariants 1-3).
-- ---------------------------------------------------------------------------
ALTER TABLE app.tenant_write_fences OWNER TO memoh_owner;
ALTER FUNCTION app.tenant_write_fences_guard() OWNER TO memoh_owner;
ALTER FUNCTION app.current_tenant_id() OWNER TO memoh_owner;
ALTER FUNCTION app.current_fencing_token() OWNER TO memoh_owner;
ALTER FUNCTION app.assert_tenant_write_fence() OWNER TO memoh_owner;
ALTER FUNCTION app.tenant_write_fence_matches() OWNER TO memoh_owner;
ALTER FUNCTION app.cas_tenant_write_fence(uuid, bigint, bigint, boolean) OWNER TO memoh_owner;

-- Fence table: zero runtime/PUBLIC table privileges; migrator gets full DML.
REVOKE ALL ON TABLE app.tenant_write_fences FROM PUBLIC, memoh_runtime;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE app.tenant_write_fences TO memoh_migrator;

-- Context helpers: owner, no PUBLIC; runtime EXECUTE on current_tenant_id;
-- current_fencing_token is only called by the DEFINER helpers, not runtime.
REVOKE ALL ON FUNCTION app.current_tenant_id() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION app.current_tenant_id() TO memoh_runtime;
REVOKE ALL ON FUNCTION app.current_fencing_token() FROM PUBLIC, memoh_runtime;

-- Write-fence helpers: runtime EXECUTE only; migrator/break-glass/PUBLIC none.
REVOKE ALL ON FUNCTION app.assert_tenant_write_fence() FROM PUBLIC, memoh_migrator, memoh_break_glass;
REVOKE ALL ON FUNCTION app.tenant_write_fence_matches() FROM PUBLIC, memoh_migrator, memoh_break_glass;
GRANT EXECUTE ON FUNCTION app.assert_tenant_write_fence() TO memoh_runtime;
GRANT EXECUTE ON FUNCTION app.tenant_write_fence_matches() TO memoh_runtime;

-- CAS management: migrator only. Runtime/PUBLIC/break-glass NO execute.
REVOKE ALL ON FUNCTION app.cas_tenant_write_fence(uuid, bigint, bigint, boolean)
  FROM PUBLIC, memoh_runtime, memoh_break_glass;
GRANT EXECUTE ON FUNCTION app.cas_tenant_write_fence(uuid, bigint, bigint, boolean) TO memoh_migrator;

-- ---------------------------------------------------------------------------
-- Seed the singleton tenant's fence (enabled, initial positive token).
-- Every undeleted tenants row must have exactly one fence; a writable tenant
-- with no fence is forbidden (fail-closed).
-- ---------------------------------------------------------------------------
INSERT INTO app.tenant_write_fences (tenant_id, fencing_token, write_enabled)
VALUES ('00000000-0000-0000-0000-000000000001', 1, true)
ON CONFLICT (tenant_id) DO NOTHING;
