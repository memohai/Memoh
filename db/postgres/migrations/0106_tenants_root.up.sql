-- 0106_tenants_root
-- Introduce the tenants root table and seed the default singleton tenant.
--
-- This is the first migration of the tenant-core work. tenants is the unique
-- root special case: its own id IS the tenant id, so it must NOT carry a
-- redundant tenant_id column. Existing installs upgrade in place (no wipe): the
-- single default tenant is seeded idempotently and every existing business row
-- is later backfilled to DefaultTenantID by subsequent migrations.
--
-- DefaultTenantID is the fixed constant published in internal/tenant/id.go
-- (00000000-0000-0000-0000-000000000001). It must never be generated per install.

CREATE TABLE IF NOT EXISTS tenants (
    id         UUID        PRIMARY KEY,
    slug       TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata   JSONB       NOT NULL DEFAULT '{}'::jsonb
);

-- slug is an optional directory field, not an identity/authorization boundary.
-- When present it must be unique cell-wide; NULL slugs are allowed and excluded.
CREATE UNIQUE INDEX IF NOT EXISTS tenants_slug_unique ON tenants (slug) WHERE slug IS NOT NULL;

-- Seed the singleton tenant idempotently. Self-hosted installs run as exactly
-- this one tenant; Cloud creates one row per team (tenant_id = team_id) and
-- never uses this singleton as the multi-tenant source of truth.
INSERT INTO tenants (id, slug)
VALUES ('00000000-0000-0000-0000-000000000001', 'default')
ON CONFLICT (id) DO NOTHING;
