// Package tenant holds the default tenant identity used by self-hosted Memoh.
package tenant

// DefaultTenantID is the stable, published identity of the singleton tenant used
// by self-hosted (single-tenant) installs. It is a fixed constant — NEVER
// generated per install — so that migrations, fixtures, and application code can
// reference the same value across environments.
//
// It is seeded by migration 0106_tenants_root and referenced as the backfill
// value when propagating tenant_id onto existing business rows.
const DefaultTenantID = "00000000-0000-0000-0000-000000000001"
