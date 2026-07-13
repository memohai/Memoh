//go:build integration

package db_test

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// sqlState returns the PostgreSQL SQLSTATE for an error, or "".
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// runtimeConn opens a connection as memoh_runtime (non-bypass serving role).
// The migration creates the role NOLOGIN/no-password; we grant LOGIN + a test
// password here and connect over TCP. Returns the connection (cleaned up via t).
func runtimeConn(t *testing.T, ownerPool interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, baseDSN string) *pgx.Conn {
	t.Helper()
	ctx := context.Background()
	if _, err := ownerPool.Exec(ctx, "ALTER ROLE memoh_runtime LOGIN PASSWORD 'runtime_test_pw'"); err != nil {
		t.Fatalf("set runtime password: %v", err)
	}
	connCfg, err := pgx.ParseConfig(baseDSN)
	if err != nil {
		t.Fatalf("pgx parse: %v", err)
	}
	connCfg.User = "memoh_runtime"
	connCfg.Password = "runtime_test_pw"
	conn, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		t.Fatalf("connect as runtime: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	return conn
}

// TestTenantWriteFenceHelpers exercises the frozen security contract for
// app.tenant_write_fences and the four app.* helpers (security design §6).
func TestTenantWriteFenceHelpers(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	dsn := tenantMigrationDSN(t)
	const defaultTenant = "00000000-0000-0000-0000-000000000001"

	// (1) singleton fence exists after migrate: write_enabled=true, token > 0.
	var token int64
	var enabled bool
	if err := pool.QueryRow(ctx,
		"SELECT fencing_token, write_enabled FROM app.tenant_write_fences WHERE tenant_id = $1", defaultTenant,
	).Scan(&token, &enabled); err != nil {
		t.Fatalf("seeded fence missing: %v", err)
	}
	if token <= 0 || !enabled {
		t.Fatalf("seeded fence must be (token>0, enabled), got (%d, %v)", token, enabled)
	}

	rc := runtimeConn(t, pool, dsn)

	// helper: run fn in a tx as runtime with the two GUCs set.
	withGUC := func(tenant string, tok string, fn func(tx pgx.Tx) error) error {
		tx, err := rc.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if tenant != "" {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant); err != nil {
				return err
			}
		}
		if tok != "" {
			if _, err := tx.Exec(ctx, "SELECT set_config('app.fencing_token', $1, true)", tok); err != nil {
				return err
			}
		}
		return fn(tx)
	}

	tokStr := func(n int64) string { return strconv.FormatInt(n, 10) }

	// (2) assert success + matches true with exact token + enabled.
	if err := withGUC(defaultTenant, tokStr(token), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()"); err != nil {
			return err
		}
		var ok bool
		if err := tx.QueryRow(ctx, "SELECT app.tenant_write_fence_matches()").Scan(&ok); err != nil {
			return err
		}
		if !ok {
			t.Error("matches() must be true for exact token + enabled")
		}
		return nil
	}); err != nil {
		t.Fatalf("assert/matches happy path: %v", err)
	}

	// (3) tenant GUC missing -> 42501 (assert and matches both raise).
	if err := withGUC("", tokStr(token), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()")
		if got := sqlState(err); got != "42501" {
			t.Errorf("missing tenant GUC: assert SQLSTATE = %q, want 42501", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("case 3 tx: %v", err)
	}
	// malformed tenant UUID -> 22P02.
	if err := withGUC("not-a-uuid", tokStr(token), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()")
		if got := sqlState(err); got != "22P02" {
			t.Errorf("malformed tenant GUC: assert SQLSTATE = %q, want 22P02", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("case 3b tx: %v", err)
	}

	// (4) stale token: assert -> 42501; matches -> false (legal context).
	if err := withGUC(defaultTenant, tokStr(token+1), func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()")
		if got := sqlState(err); got != "42501" {
			t.Errorf("stale token: assert SQLSTATE = %q, want 42501", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("case 4 tx: %v", err)
	}
	if err := withGUC(defaultTenant, tokStr(token+1), func(tx pgx.Tx) error {
		var ok bool
		if err := tx.QueryRow(ctx, "SELECT app.tenant_write_fence_matches()").Scan(&ok); err != nil {
			return err
		}
		if ok {
			t.Error("matches() must be false for stale token")
		}
		return nil
	}); err != nil {
		t.Fatalf("case 4b tx: %v", err)
	}

	// fencing token missing / nonpositive -> 42501.
	for _, bad := range []string{"", "0", "-1", "abc"} {
		if err := withGUC(defaultTenant, bad, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT app.assert_tenant_write_fence()")
			if got := sqlState(err); got != "42501" {
				t.Errorf("bad token %q: assert SQLSTATE = %q, want 42501", bad, got)
			}
			return nil
		}); err != nil {
			t.Fatalf("bad-token tx %q: %v", bad, err)
		}
	}

	// (7) runtime role ACL: no direct table access; EXECUTE on the two helpers;
	// no EXECUTE on the CAS management function.
	if _, err := rc.Exec(ctx, "SELECT 1 FROM app.tenant_write_fences"); err == nil {
		t.Error("runtime must NOT be able to read app.tenant_write_fences directly")
	} else if got := sqlState(err); got != "42501" {
		t.Errorf("runtime direct table read SQLSTATE = %q, want 42501 (permission denied)", got)
	}
	if _, err := rc.Exec(ctx, "SELECT app.cas_tenant_write_fence($1, 1, 2, false)", defaultTenant); err == nil {
		t.Error("runtime must NOT be able to execute the CAS management function")
	}
}

// TestTenantWriteFenceCAS exercises the controlled CAS management contract
// (security design §6.5.1): advance only old->old+1 and land disabled; exact
// enable/disable keeps the token; no skipping, no advance-and-enable.
func TestTenantWriteFenceCAS(t *testing.T) {
	ctx := context.Background()
	pool := freshMigratedDB(t)
	const dt = "00000000-0000-0000-0000-000000000001"

	exec := func(sql string, args ...any) (bool, error) {
		var ok bool
		err := pool.QueryRow(ctx, sql, args...).Scan(&ok)
		return ok, err
	}

	// advance-and-enable in one step must be rejected.
	if _, err := exec("SELECT app.cas_tenant_write_fence($1, 1, 2, true)", dt); sqlState(err) != "23514" {
		t.Errorf("advance+enable: SQLSTATE = %q, want 23514", sqlState(err))
	}
	// skipping (F -> F+2) must be rejected.
	if _, err := exec("SELECT app.cas_tenant_write_fence($1, 1, 3, false)", dt); sqlState(err) != "23514" {
		t.Errorf("skip token: SQLSTATE = %q, want 23514", sqlState(err))
	}
	// lowering must be rejected.
	if _, err := exec("SELECT app.cas_tenant_write_fence($1, 2, 1, false)", dt); sqlState(err) != "23514" {
		t.Errorf("lower token: SQLSTATE = %q, want 23514", sqlState(err))
	}
	// exact disable (same token) succeeds and returns true.
	if ok, err := exec("SELECT app.cas_tenant_write_fence($1, 1, 1, false)", dt); err != nil || !ok {
		t.Fatalf("exact disable: ok=%v err=%v", ok, err)
	}
	// valid advance 1 -> 2 disabled succeeds.
	if ok, err := exec("SELECT app.cas_tenant_write_fence($1, 1, 2, false)", dt); err != nil || !ok {
		t.Fatalf("advance 1->2: ok=%v err=%v", ok, err)
	}
	// CAS with wrong expected token = 0 rows affected -> returns false (conflict).
	if ok, err := exec("SELECT app.cas_tenant_write_fence($1, 1, 2, false)", dt); err != nil || ok {
		t.Fatalf("stale CAS must return false (conflict), got ok=%v err=%v", ok, err)
	}
	// exact enable at current token 2 succeeds.
	if ok, err := exec("SELECT app.cas_tenant_write_fence($1, 2, 2, true)", dt); err != nil || !ok {
		t.Fatalf("exact enable at 2: ok=%v err=%v", ok, err)
	}

	// Direct UPDATE that decreases the token is blocked by the trigger.
	if _, err := pool.Exec(ctx,
		"UPDATE app.tenant_write_fences SET fencing_token = 1 WHERE tenant_id = $1", dt,
	); sqlState(err) != "23514" {
		t.Errorf("token decrease via trigger: SQLSTATE = %q, want 23514", sqlState(err))
	}
	// Direct UPDATE that advances the token while enabling is blocked.
	if _, err := pool.Exec(ctx,
		"UPDATE app.tenant_write_fences SET fencing_token = 3, write_enabled = true WHERE tenant_id = $1", dt,
	); sqlState(err) != "23514" {
		t.Errorf("advance+enable via trigger: SQLSTATE = %q, want 23514", sqlState(err))
	}
}
