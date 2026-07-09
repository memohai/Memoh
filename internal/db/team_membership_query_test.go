package db_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dbpkg "github.com/memohai/memoh/internal/db"
	dbsqlc "github.com/memohai/memoh/internal/db/postgres/sqlc"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
	dbstore "github.com/memohai/memoh/internal/db/store"
	"github.com/memohai/memoh/internal/teams"
)

// openTestPool opens a pgxpool, skipping the test if TEST_POSTGRES_DSN is unset
// or the DB is unreachable. Matches the pattern used by other integration tests.
func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("skip integration test: TEST_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("skip integration test: cannot connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skip integration test: database ping failed: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// newIntegrationQueries returns a store-backed Queries and a cleanup func.
func newIntegrationQueries(t *testing.T) (dbstore.Queries, *pgxpool.Pool) {
	t.Helper()
	pool := openTestPool(t)
	q := postgresstore.NewQueriesWithPool(pool, dbsqlc.New(pool))
	return q, pool
}

// seedUser creates a user and inserts a team_members row for the default team
// with role='member'. Returns the pgtype.UUID of the new user.
func seedUser(t *testing.T, pool *pgxpool.Pool, q dbstore.Queries, name string) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	userRow, err := q.CreateUser(ctx, dbsqlc.CreateUserParams{
		IsActive: true,
		Metadata: []byte(`{"source":"team-membership-test","name":"` + name + `"}`),
	})
	if err != nil {
		t.Fatalf("seedUser create user %q: %v", name, err)
	}
	pgTeamID, err := dbpkg.ParseUUID(teams.DefaultTeamID)
	if err != nil {
		t.Fatalf("parse DefaultTeamID: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO team_members (team_id, user_id, role)
		 VALUES ($1, $2, 'member') ON CONFLICT DO NOTHING`,
		pgTeamID, userRow.ID,
	)
	if err != nil {
		t.Fatalf("seedUser insert team_member for %q: %v", name, err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, "DELETE FROM team_members WHERE user_id = $1", userRow.ID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userRow.ID)
	})
	return userRow.ID
}

// TestGetTeamMembershipReturnsRoleForMember verifies a member of the default
// team can query their role via GetTeamMembership.
func TestGetTeamMembershipReturnsRoleForMember(t *testing.T) {
	q, pool := newIntegrationQueries(t)
	ctx := context.Background()

	pgTeamID, err := dbpkg.ParseUUID(teams.DefaultTeamID)
	if err != nil {
		t.Fatalf("parse DefaultTeamID: %v", err)
	}
	userID := seedUser(t, pool, q, "member-a")

	row, err := q.GetTeamMembership(ctx, dbsqlc.GetTeamMembershipParams{
		TeamID: pgTeamID,
		UserID: userID,
	})
	if err != nil {
		t.Fatalf("GetTeamMembership: %v", err)
	}
	if row.Role != "member" {
		t.Fatalf("role = %q, want member", row.Role)
	}
}
