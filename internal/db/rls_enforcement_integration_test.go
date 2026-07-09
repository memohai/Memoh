//go:build integration

// Package db_test contains the RLS enforcement acceptance gate.
//
// This test proves that FORCE ROW LEVEL SECURITY plus the non-owner, non-superuser
// memoh_app runtime role actually isolates teams at the database layer: a
// connection scoped to team A cannot see or modify team B's rows.
//
// Setup expectation: point MEMOH_TEST_OWNER_DSN at a scratch database that already
// has all migrations applied via the OWNER role (so memoh_app exists per migration
// 0107). The test sets the memoh_app password itself (ALTER ROLE) and connects as
// memoh_app. When MEMOH_TEST_OWNER_DSN is unset the test skips.
//
// Build the scratch DB and run, e.g.:
//
//	createdb memoh_p3b            # then apply migrations as owner
//	go test -tags integration ./internal/db/ -run TestRLS -count=1
package db_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/teams"
)

// uniq returns a lowercase, hyphen-safe suffix unique to this run so repeated
// runs on the same scratch DB do not collide on unique slugs/usernames.
func uniq() string {
	n := time.Now().UnixNano()
	var out []byte
	for n > 0 {
		out = append([]byte{byte('a' + n%26)}, out...)
		n /= 26
	}
	return string(out)
}

const (
	testAppRole = "memoh_app"
	// #nosec G101 -- test-only password for a scratch DB role, not a secret.
	testAppPassword = "memoh_app_test_pw"
)

// ownerDSN returns the scratch-DB owner DSN or skips the test.
func ownerDSN(t *testing.T) string {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("MEMOH_TEST_OWNER_DSN"))
	if dsn == "" {
		t.Skip("set MEMOH_TEST_OWNER_DSN to a migrated scratch DB (owner role) to run RLS integration tests")
	}
	return dsn
}

// appDSN derives the memoh_app DSN from the owner DSN by swapping credentials.
func appDSN(t *testing.T, owner string) string {
	t.Helper()
	cfg, err := pgx.ParseConfig(owner)
	if err != nil {
		t.Fatalf("parse owner DSN: %v", err)
	}
	cfg.User = testAppRole
	cfg.Password = testAppPassword
	// Reconstruct a keyword/value DSN pgx understands.
	kv := []string{
		"host=" + cfg.Host,
		"user=" + cfg.User,
		"password=" + cfg.Password,
		"dbname=" + cfg.Database,
	}
	if cfg.Port != 0 {
		kv = append(kv, "port="+itoa(int(cfg.Port)))
	}
	if sslmode := strings.TrimSpace(os.Getenv("MEMOH_TEST_SSLMODE")); sslmode != "" {
		kv = append(kv, "sslmode="+sslmode)
	} else {
		kv = append(kv, "sslmode=disable")
	}
	return strings.Join(kv, " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// setupOwner connects as owner, ensures the memoh_app password, and returns the conn.
func setupOwner(t *testing.T, ctx context.Context, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatalf("connect owner: %v", err)
	}
	if _, err := conn.Exec(ctx, "ALTER ROLE "+testAppRole+" PASSWORD '"+testAppPassword+"'"); err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("set memoh_app password: %v", err)
	}
	return conn
}

// seedTeamWithBot creates a team + owner user + a bot in that team via the owner
// connection and returns the team id and bot id.
func seedTeamWithBot(t *testing.T, ctx context.Context, owner *pgx.Conn, slug string) (teamID, botID string) {
	t.Helper()
	if err := owner.QueryRow(ctx,
		`INSERT INTO teams (slug, name) VALUES ($1, $1) RETURNING id`, slug,
	).Scan(&teamID); err != nil {
		t.Fatalf("insert team %s: %v", slug, err)
	}
	var userID string
	if err := owner.QueryRow(ctx,
		`INSERT INTO users (username, timezone) VALUES ($1, 'UTC') RETURNING id`, "user_"+slug,
	).Scan(&userID); err != nil {
		t.Fatalf("insert user for %s: %v", slug, err)
	}
	// bots.name must match ^[a-z0-9][a-z0-9-]{1,62}$ (lowercase, hyphens only).
	// heartbeat_enabled = true so the bot appears in the all-team
	// ListHeartbeatEnabledBots startup query used by the maintenance-pool test.
	if err := owner.QueryRow(ctx,
		`INSERT INTO bots (owner_user_id, name, team_id, heartbeat_enabled) VALUES ($1, $2, $3, true) RETURNING id`,
		userID, "bot-"+slug, teamID,
	).Scan(&botID); err != nil {
		t.Fatalf("insert bot for %s: %v", slug, err)
	}
	return teamID, botID
}

// TestRLSBlocksCrossTeam is the acceptance gate: connected as memoh_app with
// app.team_id = team A, the app role must see only team A's bots and must not be
// able to update team B's bots.
func TestRLSBlocksCrossTeam(t *testing.T) {
	ctx := context.Background()
	owner := setupOwner(t, ctx, ownerDSN(t))
	defer func() { _ = owner.Close(ctx) }()

	run := uniq()
	teamA, botA := seedTeamWithBot(t, ctx, owner, "rls-team-a-"+run)
	teamB, _ := seedTeamWithBot(t, ctx, owner, "rls-team-b-"+run)
	t.Cleanup(func() {
		_, _ = owner.Exec(ctx, "DELETE FROM teams WHERE id = ANY($1)", []string{teamA, teamB})
		_, _ = owner.Exec(ctx, "DELETE FROM users WHERE username LIKE $1", "user_rls-team-%-"+run)
	})

	appConn, err := pgx.Connect(ctx, appDSN(t, ownerDSN(t)))
	if err != nil {
		t.Fatalf("connect memoh_app: %v", err)
	}
	defer func() { _ = appConn.Close(ctx) }()

	if _, err := appConn.Exec(ctx, "SELECT set_config('app.team_id', $1, false)", teamA); err != nil {
		t.Fatalf("set app.team_id: %v", err)
	}

	// Only team A's bot is visible.
	var visible int
	if err := appConn.QueryRow(ctx, "SELECT count(*) FROM bots").Scan(&visible); err != nil {
		t.Fatalf("count bots: %v", err)
	}
	if visible != 1 {
		t.Fatalf("memoh_app scoped to team A saw %d bots, want exactly team A's 1", visible)
	}
	var botAVisible int
	if err := appConn.QueryRow(ctx, "SELECT count(*) FROM bots WHERE id = $1", botA).Scan(&botAVisible); err != nil {
		t.Fatalf("count botA: %v", err)
	}
	if botAVisible != 1 {
		t.Fatalf("team A's own bot not visible under RLS (got %d)", botAVisible)
	}

	// Updating team B's rows affects zero rows.
	tag, err := appConn.Exec(ctx, "UPDATE bots SET name = name WHERE team_id = $1", teamB)
	if err != nil {
		t.Fatalf("update team B bots: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("memoh_app updated %d team B rows, want 0", tag.RowsAffected())
	}

	// A blanket UPDATE (no team predicate) still only touches team A's row.
	tag, err = appConn.Exec(ctx, "UPDATE bots SET name = name")
	if err != nil {
		t.Fatalf("blanket update: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("blanket UPDATE under RLS touched %d rows, want only team A's 1", tag.RowsAffected())
	}
}

// TestTeamPoolDBTXEnforcesRLSOverPool validates Task 3 (Option A): the pooled,
// non-transactional DBTX path sets app.team_id on the same connection that runs
// the statement (via SendBatch), so real reads/writes on the restricted memoh_app
// pool return the scoped team's rows — and do not leak or exhaust connections.
func TestTeamPoolDBTXEnforcesRLSOverPool(t *testing.T) {
	ctx := context.Background()
	owner := setupOwner(t, ctx, ownerDSN(t))
	defer func() { _ = owner.Close(ctx) }()

	run := uniq()
	teamA, _ := seedTeamWithBot(t, ctx, owner, "rls-pool-a-"+run)
	teamB, _ := seedTeamWithBot(t, ctx, owner, "rls-pool-b-"+run)
	t.Cleanup(func() {
		_, _ = owner.Exec(ctx, "DELETE FROM teams WHERE id = ANY($1)", []string{teamA, teamB})
		_, _ = owner.Exec(ctx, "DELETE FROM users WHERE username LIKE $1", "user_rls-pool-%-"+run)
	})

	poolCfg, err := pgxpool.ParseConfig(appDSN(t, ownerDSN(t)))
	if err != nil {
		t.Fatalf("parse app pool cfg: %v", err)
	}
	// Small pool so a leaked connection would surface as exhaustion.
	poolCfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("build memoh_app pool: %v", err)
	}
	defer pool.Close()

	dbtx := postgresstore.NewTeamPoolDBTXForTest(pool)

	// Scope to team A.
	ctxA := teams.WithScope(ctx, teams.Scope{TeamID: teamA})

	// QueryRow through the pool path returns team A's row count under RLS.
	var count int
	if err := dbtx.QueryRow(ctxA, "SELECT count(*) FROM bots").Scan(&count); err != nil {
		t.Fatalf("QueryRow count: %v", err)
	}
	if count != 1 {
		t.Fatalf("pooled QueryRow saw %d bots for team A, want 1", count)
	}

	// Query (rows) through the pool path returns exactly team A's bot.
	rows, err := dbtx.Query(ctxA, "SELECT team_id FROM bots")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	var seen int
	for rows.Next() {
		var got string
		if err := rows.Scan(&got); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if got != teamA {
			t.Fatalf("pooled Query returned team %s, want only team A %s", got, teamA)
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows err: %v", err)
	}
	rows.Close()
	if seen != 1 {
		t.Fatalf("pooled Query saw %d team A bots, want 1", seen)
	}

	// Exec (write) scoped to team B affects zero rows when scoped to team A.
	tag, err := dbtx.Exec(ctxA, "UPDATE bots SET name = name WHERE team_id = $1", teamB)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if tag.RowsAffected() != 0 {
		t.Fatalf("pooled Exec scoped to A updated %d team B rows, want 0", tag.RowsAffected())
	}

	// Repeat enough times that any per-statement conn leak (MaxConns=2) would
	// exhaust the pool and hang/fail here.
	for i := 0; i < 20; i++ {
		var c int
		if err := dbtx.QueryRow(ctxA, "SELECT count(*) FROM bots").Scan(&c); err != nil {
			t.Fatalf("QueryRow iter %d: %v", i, err)
		}
	}
}

// TestMaintenanceQueriesSeeAllTeams validates Task 4: the all-team startup
// query ListHeartbeatEnabledBots returns rows for EVERY team on the maintenance
// (owner) pool, but returns only the scoped team's rows on the restricted
// memoh_app pool — which is exactly why the enumerated startup queries must run
// on the maintenance pool.
func TestMaintenanceQueriesSeeAllTeams(t *testing.T) {
	ctx := context.Background()
	owner := setupOwner(t, ctx, ownerDSN(t))
	defer func() { _ = owner.Close(ctx) }()

	run := uniq()
	teamA, botA := seedTeamWithBot(t, ctx, owner, "rls-maint-a-"+run)
	teamB, botB := seedTeamWithBot(t, ctx, owner, "rls-maint-b-"+run)
	t.Cleanup(func() {
		_, _ = owner.Exec(ctx, "DELETE FROM teams WHERE id = ANY($1)", []string{teamA, teamB})
		_, _ = owner.Exec(ctx, "DELETE FROM users WHERE username LIKE $1", "user_rls-maint-%-"+run)
	})

	seededIDs := map[string]bool{botA: true, botB: true}

	// Owner pool + raw DBTX = maintenance Queries (bypasses FORCE RLS, no
	// per-statement app.team_id).
	ownerPoolCfg, err := pgxpool.ParseConfig(ownerDSN(t))
	if err != nil {
		t.Fatalf("parse owner pool cfg: %v", err)
	}
	ownerPool, err := pgxpool.NewWithConfig(ctx, ownerPoolCfg)
	if err != nil {
		t.Fatalf("build owner pool: %v", err)
	}
	defer ownerPool.Close()
	maintenanceQueries := dbstore.NewMaintenanceQueries(
		postgresstore.NewQueriesWithPool(ownerPool, dbsqlc.New(ownerPool)),
	)

	maintRows, err := maintenanceQueries.ListHeartbeatEnabledBots(ctx)
	if err != nil {
		t.Fatalf("maintenance ListHeartbeatEnabledBots: %v", err)
	}
	maintSeen := map[string]bool{}
	for _, r := range maintRows {
		id := uuidString(r.ID.Bytes)
		if seededIDs[id] {
			maintSeen[id] = true
		}
	}
	if !maintSeen[botA] || !maintSeen[botB] {
		t.Fatalf("maintenance pool saw teams %v, want both teamA and teamB bots", maintSeen)
	}

	// Restricted memoh_app pool + teamPoolDBTX scoped to team A = per-request
	// Queries. Under FORCE RLS it only sees team A's heartbeat bot, proving the
	// all-team startup query would be wrong on the restricted pool.
	appPoolCfg, err := pgxpool.ParseConfig(appDSN(t, ownerDSN(t)))
	if err != nil {
		t.Fatalf("parse app pool cfg: %v", err)
	}
	appPool, err := pgxpool.NewWithConfig(ctx, appPoolCfg)
	if err != nil {
		t.Fatalf("build app pool: %v", err)
	}
	defer appPool.Close()
	appStore, err := postgresstore.New(appPool)
	if err != nil {
		t.Fatalf("app store: %v", err)
	}
	appQueries := postgresstore.NewQueriesWithPool(appPool, appStore.SQLC())

	ctxA := teams.WithScope(ctx, teams.Scope{TeamID: teamA})
	appRows, err := appQueries.ListHeartbeatEnabledBots(ctxA)
	if err != nil {
		t.Fatalf("restricted ListHeartbeatEnabledBots: %v", err)
	}
	appSeen := map[string]bool{}
	for _, r := range appRows {
		id := uuidString(r.ID.Bytes)
		if seededIDs[id] {
			appSeen[id] = true
		}
	}
	if !appSeen[botA] {
		t.Fatalf("restricted pool scoped to team A did not see team A's heartbeat bot")
	}
	if appSeen[botB] {
		t.Fatalf("restricted pool scoped to team A leaked team B's heartbeat bot")
	}
}

func uuidString(b [16]byte) string {
	const hex = "0123456789abcdef"
	var out [36]byte
	j := 0
	for i := 0; i < 16; i++ {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out[j] = '-'
			j++
		}
		out[j] = hex[b[i]>>4]
		out[j+1] = hex[b[i]&0x0f]
		j += 2
	}
	return string(out[:])
}
