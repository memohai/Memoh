// Package tenant holds the upstream tenant-core primitives: the default
// singleton tenant identity and (in later work) the request-scoped tenant
// context used to bind PostgreSQL row-level security.
//
// Upstream Memoh is single-tenant by default: every self-hosted install runs as
// exactly one tenant, DefaultTenantID. Cloud layers multi-tenant placement on
// top of these primitives; upstream must never depend on a client-supplied
// tenant id.
package tenant

// DefaultTenantID is the stable, published identity of the singleton tenant used
// by self-hosted (single-tenant) installs. It is a fixed constant — NEVER
// generated per install — so that migrations, fixtures, and application code can
// reference the same value across environments.
//
// It is seeded by migration 0106_tenants_root and referenced as the backfill
// value when propagating tenant_id onto existing business rows.
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"
