//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTenantCompositeKeys verifies the atomic composite-key migration:
//   - every tenant table PK leads with tenant_id
//   - every tenant table has a root FK (tenant_id) -> tenants(id)
//   - every business FK is composite and carries tenant_id
//   - no ON DELETE SET NULL remains on any tenant-table FK (all -> RESTRICT/etc.)
//   - a cross-tenant child insert is rejected; same-tenant self-reference works
func TestTenantCompositeKeys(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	// (1) Every tenant table PK must lead with tenant_id.
	rows, err := pool.Query(ctx, `
		SELECT c.relname,
		       (SELECT a.attname FROM pg_attribute a
		         WHERE a.attrelid = con.conrelid AND a.attnum = con.conkey[1])
		  FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype = 'p' AND n.nspname = 'public'
		   AND c.relname NOT IN ('schema_migrations', 'tenants')`)
	if err != nil {
		t.Fatalf("enumerate PKs: %v", err)
	}
	for rows.Next() {
		var tbl, firstCol string
		if err := rows.Scan(&tbl, &firstCol); err != nil {
			t.Fatalf("scan pk: %v", err)
		}
		if firstCol != "tenant_id" {
			t.Errorf("PK of %s must lead with tenant_id, leads with %q", tbl, firstCol)
		}
	}
	rows.Close()

	// (2) No ON DELETE SET NULL may remain on any public tenant-table FK.
	var setNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype = 'f' AND con.confdeltype = 'n'
		   AND n.nspname = 'public'`).Scan(&setNull); err != nil {
		t.Fatalf("count SET NULL: %v", err)
	}
	if setNull != 0 {
		t.Errorf("expected 0 ON DELETE SET NULL FKs after migration, got %d", setNull)
	}

	// (3) Every tenant table must have a root FK (tenant_id) -> tenants(id).
	assertRootFKs(ctx, t, pool)

	// (4) Every non-root business FK must include tenant_id.
	assertBusinessFKsCarryTenantID(ctx, t, pool)

	// (5) Cross-tenant child insert rejected; same-tenant self-ref works.
	assertCrossTenantFKRejected(ctx, t, pool)
}

func assertRootFKs(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT c.relname FROM pg_class c
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE c.relkind = 'r' AND n.nspname = 'public'
		   AND c.relname NOT IN ('schema_migrations', 'tenants')`)
	if err != nil {
		t.Fatalf("enumerate tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var n string
		_ = rows.Scan(&n)
		tables = append(tables, n)
	}
	rows.Close()

	for _, tbl := range tables {
		var hasRootFK bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_constraint con
				  JOIN pg_class rt ON rt.oid = con.confrelid
				  JOIN pg_attribute a ON a.attrelid = con.conrelid AND a.attnum = ANY(con.conkey)
				 WHERE con.contype = 'f'
				   AND con.conrelid = ('public.'||quote_ident($1))::regclass
				   AND rt.relname = 'tenants'
				   AND a.attname = 'tenant_id'
			)`, tbl).Scan(&hasRootFK); err != nil {
			t.Fatalf("check root FK on %s: %v", tbl, err)
		}
		if !hasRootFK {
			t.Errorf("tenant table %s is missing a root FK (tenant_id) -> tenants(id)", tbl)
		}
	}
}

func assertBusinessFKsCarryTenantID(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	// Every FK from a tenant table to another tenant table (parent != tenants)
	// must include the tenant_id column in its key.
	rows, err := pool.Query(ctx, `
		SELECT c.relname, con.conname, rt.relname AS parent
		  FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_class rt ON rt.oid = con.confrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype = 'f' AND n.nspname = 'public'
		   AND rt.relname NOT IN ('tenants')
		   AND NOT EXISTS (
		       SELECT 1 FROM pg_attribute a
		        WHERE a.attrelid = con.conrelid AND a.attnum = ANY(con.conkey)
		          AND a.attname = 'tenant_id')`)
	if err != nil {
		t.Fatalf("enumerate business FKs: %v", err)
	}
	for rows.Next() {
		var tbl, fk, parent string
		_ = rows.Scan(&tbl, &fk, &parent)
		t.Errorf("business FK %s on %s -> %s does not include tenant_id", fk, tbl, parent)
	}
	rows.Close()
}

func assertCrossTenantFKRejected(ctx context.Context, t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	const t1 = "00000000-0000-0000-0000-000000000001"
	const t2 = "00000000-0000-0000-0000-0000000000f2"
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 'ct2')`, t2); err != nil {
		t.Fatalf("seed tenant2: %v", err)
	}
	// Create a provider in t1, then try to create a model in t2 that references
	// it — must fail, because the composite FK (tenant_id, provider_id) ->
	// providers(tenant_id, id) cannot match across tenants.
	var providerID string
	if err := pool.QueryRow(ctx,
		`INSERT INTO providers (tenant_id, name, client_type) VALUES ($1, 'p', 'openai-completions') RETURNING id`, t1,
	).Scan(&providerID); err != nil {
		t.Fatalf("insert provider t1: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO models (tenant_id, provider_id, model_id, type) VALUES ($1, $2, 'm', 'chat')`, t2, providerID,
	); sqlState(err) != "23503" {
		t.Fatalf("cross-tenant model->provider FK must be rejected (23503), got %q", sqlState(err))
	}
	// Same-tenant reference succeeds.
	if _, err := pool.Exec(ctx,
		`INSERT INTO models (tenant_id, provider_id, model_id, type) VALUES ($1, $2, 'm', 'chat')`, t1, providerID,
	); err != nil {
		t.Fatalf("same-tenant model->provider must succeed: %v", err)
	}
}
