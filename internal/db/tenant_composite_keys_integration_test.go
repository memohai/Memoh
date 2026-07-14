//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTenantCompositeKeys verifies the tenant-key migration:
//   - existing primary keys stay stable and gain a tenant-prefixed unique key
//   - every tenant table has a root FK (tenant_id) -> tenants(id)
//   - every business FK is composite and carries tenant_id
//   - ON DELETE SET NULL clears only the original reference column
//   - a cross-tenant child insert is rejected; same-tenant self-reference works
func TestTenantCompositeKeys(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	// (1) Every tenant table has a helper unique key leading with tenant_id.
	rows, err := pool.Query(ctx, `
		SELECT c.relname, count(helper.oid)
		  FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		  LEFT JOIN pg_constraint helper
		    ON helper.conrelid=con.conrelid AND helper.contype='u'
		   AND helper.conname LIKE 'memoh_tenant_key_%'
		 WHERE con.contype = 'p' AND n.nspname = 'public'
		   AND EXISTS (SELECT 1 FROM pg_attribute a
		                WHERE a.attrelid=c.oid AND a.attname='tenant_id' AND NOT a.attisdropped)
		 GROUP BY c.relname`)
	if err != nil {
		t.Fatalf("enumerate PKs: %v", err)
	}
	for rows.Next() {
		var tbl string
		var helperCount int
		if err := rows.Scan(&tbl, &helperCount); err != nil {
			t.Fatalf("scan pk: %v", err)
		}
		if helperCount != 1 {
			t.Errorf("%s must have one tenant-prefixed helper key, got %d", tbl, helperCount)
		}
	}
	rows.Close()

	// (2) SET NULL must never include tenant_id in its target column list.
	var unsafeSetNull int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint con
		  JOIN pg_class c ON c.oid = con.conrelid
		  JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE con.contype = 'f' AND con.confdeltype = 'n'
		   AND n.nspname = 'public'
		   AND (con.confdelsetcols IS NULL OR EXISTS (
		       SELECT 1 FROM pg_attribute a
		        WHERE a.attrelid=con.conrelid AND a.attnum=ANY(con.confdelsetcols)
		          AND a.attname='tenant_id'))`).Scan(&unsafeSetNull); err != nil {
		t.Fatalf("count SET NULL: %v", err)
	}
	if unsafeSetNull != 0 {
		t.Errorf("found %d SET NULL FKs that can clear tenant_id", unsafeSetNull)
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
