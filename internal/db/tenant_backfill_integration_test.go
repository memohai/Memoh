//go:build integration

package db_test

import (
	"context"
	"testing"
)

func TestTenantIDBackfilledOnFreshInstall(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)

	// Enumerate applied tenant tables.
	rows, err := pool.Query(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
		  AND table_name NOT IN ('schema_migrations', 'tenants')
		ORDER BY table_name`)
	if err != nil {
		t.Fatalf("enumerate tenant tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		tables = append(tables, n)
	}
	rows.Close()
	if len(tables) == 0 {
		t.Fatal("expected a non-empty set of tenant tables")
	}

	// Every tenant table must now have a tenant_id column, and it must be fully
	// backfilled (zero NULLs) to DefaultTenantID.
	for _, tbl := range tables {
		var hasCol bool
		if err := pool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM information_schema.columns
				WHERE table_schema='public' AND table_name=$1 AND column_name='tenant_id')`, tbl,
		).Scan(&hasCol); err != nil {
			t.Fatalf("check tenant_id on %s: %v", tbl, err)
		}
		if !hasCol {
			t.Fatalf("tenant table %q is missing a tenant_id column", tbl)
		}

		var nulls int
		if err := pool.QueryRow(ctx,
			"SELECT count(*) FROM "+quoteIdent(tbl)+" WHERE tenant_id IS NULL",
		).Scan(&nulls); err != nil {
			t.Fatalf("count NULL tenant_id on %s: %v", tbl, err)
		}
		if nulls != 0 {
			t.Fatalf("tenant table %q has %d rows with NULL tenant_id after backfill", tbl, nulls)
		}
	}

}

// quoteIdent double-quotes a SQL identifier for safe interpolation of a table
// name that comes from information_schema (not user input).
func quoteIdent(id string) string {
	out := make([]byte, 0, len(id)+2)
	out = append(out, '"')
	for i := 0; i < len(id); i++ {
		if id[i] == '"' {
			out = append(out, '"')
		}
		out = append(out, id[i])
	}
	out = append(out, '"')
	return string(out)
}
