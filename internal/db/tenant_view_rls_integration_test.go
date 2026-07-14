//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

// TestViewDoesNotBypassRLS is the regression test for the SEV1 cross-tenant read
// through the bot_visible_history_messages view. A view without security_invoker
// runs as its (superuser) owner and bypasses FORCE RLS; this test seeds a
// tenant-1 message and asserts a memoh_runtime connection bound to tenant 2 sees
// ZERO rows through the view (and can read the view at all — it must be granted).
func TestViewDoesNotBypassRLS(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)
	const t1 = "00000000-0000-0000-0000-000000000001"
	const t2 = "00000000-0000-0000-0000-0000000000f2"

	// Seed tenant 2 + fence.
	if _, err := pool.Exec(ctx, `INSERT INTO tenants (id, slug) VALUES ($1, 't2')`, t2); err != nil {
		t.Fatalf("seed tenant2: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO app.tenant_write_fences (tenant_id, fencing_token, write_enabled) VALUES ($1, 1, true)`, t2); err != nil {
		t.Fatalf("seed tenant2 fence: %v", err)
	}

	// Seed a tenant-1 visible message (as owner/migrator, bypassing RLS for setup).
	if _, err := pool.Exec(ctx, `
		WITH u AS (
			INSERT INTO users (tenant_id, username, is_active, metadata)
			VALUES ($1, 't1u', true, '{}') RETURNING id
		), b AS (
			INSERT INTO bots (tenant_id, owner_user_id, name, status, metadata)
			SELECT $1, u.id, 'secretbot', 'ready', '{}' FROM u RETURNING id
		), s AS (
			INSERT INTO bot_sessions (tenant_id, bot_id, channel_type, title, metadata)
			SELECT $1, b.id, 'local', 's', '{}' FROM b RETURNING id, bot_id
		)
		INSERT INTO bot_history_messages
			(tenant_id, bot_id, session_id, role, content, metadata, usage,
			 turn_id, turn_position, turn_message_seq, turn_visible)
		SELECT $1, s.bot_id, s.id, 'assistant', '"SECRET-TENANT1"'::jsonb, '{}', '{}',
			gen_random_uuid(), 1, 1, true
		FROM s`, t1); err != nil {
		t.Fatalf("seed tenant1 message: %v", err)
	}

	rc := runtimeConn(t, pool, dsn)

	// Bind runtime to tenant 2.
	if _, err := rc.Exec(ctx, "SELECT set_config('app.tenant_id', $1, false)", t2); err != nil {
		t.Fatalf("bind tenant2: %v", err)
	}

	// Runtime MUST be able to read the view (it must be granted SELECT).
	var baseRows, viewRows int
	if err := rc.QueryRow(ctx, "SELECT count(*) FROM bot_history_messages").Scan(&baseRows); err != nil {
		t.Fatalf("runtime read base table: %v", err)
	}
	if err := rc.QueryRow(ctx, "SELECT count(*) FROM bot_visible_history_messages").Scan(&viewRows); err != nil {
		t.Fatalf("runtime must be able to read the view (needs GRANT SELECT): %v", err)
	}

	// The base table is correctly RLS-scoped (0 tenant-1 rows visible to tenant 2).
	if baseRows != 0 {
		t.Errorf("base table leaked %d tenant-1 rows to tenant 2 (RLS broken)", baseRows)
	}
	// The view must NOT bypass RLS.
	if viewRows != 0 {
		t.Errorf("SEV1: view leaked %d tenant-1 rows to tenant 2 (view bypasses RLS)", viewRows)
	}

	// The view must project tenant_id and it must equal the caller's tenant.
	var vTenant string
	err := rc.QueryRow(ctx, "SELECT tenant_id::text FROM bot_visible_history_messages LIMIT 1").Scan(&vTenant)
	if err != nil && err != pgx.ErrNoRows {
		t.Fatalf("view must project tenant_id: %v", err)
	}
}
