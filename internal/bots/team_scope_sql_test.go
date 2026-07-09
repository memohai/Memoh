package bots

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBotQueriesDeclareTeamScopedParameters(t *testing.T) {
	sql := readRepoFile(t, "db/postgres/queries/bots.sql")

	assertSQLContains(t, sql, "-- name: CreateBot :one", "INSERT INTO bots (team_id,")
	assertSQLContains(t, sql, "-- name: CreateBot :one", "sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: GetBotByID :one", "WHERE id = sqlc.arg(id)")
	assertSQLContains(t, sql, "-- name: GetBotByID :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: GetBotByName :one", "WHERE team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: ListBotsByOwner :many", "WHERE team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: ListAccessibleBots :many", "WHERE b.team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: UpdateBotProfile :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: UpdateBotOwner :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: UpdateBotStatus :exec", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: DeleteBotByID :exec", "AND team_id = sqlc.arg(team_id)::uuid")
}

func TestBotUserGrantQueriesDeclareTeamScopedParameters(t *testing.T) {
	sql := readRepoFile(t, "db/postgres/queries/bot_user_grants.sql")

	assertSQLContains(t, sql, "-- name: ListBotUserGrants :many", "WHERE g.bot_id = sqlc.arg(bot_id)")
	assertSQLContains(t, sql, "-- name: ListBotUserGrants :many", "AND g.team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: GetBotUserGrantByID :one", "WHERE id = sqlc.arg(id)")
	assertSQLContains(t, sql, "-- name: GetBotUserGrantByID :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: ListBotUserGrantsForUser :many", "WHERE bot_id = sqlc.arg(bot_id)")
	assertSQLContains(t, sql, "-- name: ListBotUserGrantsForUser :many", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: CreateBotUserGrant :one", "INSERT INTO bot_user_grants (team_id,")
	assertSQLContains(t, sql, "-- name: CreateBotUserGrant :one", "sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: UpdateBotUserGrantPermissions :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: DeleteBotUserGrantByID :exec", "AND team_id = sqlc.arg(team_id)::uuid")
}

func TestBotACLQueriesDeclareTeamScopedParameters(t *testing.T) {
	sql := readRepoFile(t, "db/postgres/queries/acl.sql")

	assertSQLContains(t, sql, "-- name: EvaluateBotACLRule :one", "r.team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: EvaluateBotACLRule :one", "WHERE b.id = sqlc.arg(bot_id)")
	assertSQLContains(t, sql, "-- name: EvaluateBotACLRule :one", "AND b.team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: GetBotACLDefaultEffect :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: SetBotACLDefaultEffect :exec", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: ListBotACLRules :many", "WHERE r.bot_id = sqlc.arg(bot_id)")
	assertSQLContains(t, sql, "-- name: ListBotACLRules :many", "AND r.team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: CreateBotACLRule :one", "team_id,")
	assertSQLContains(t, sql, "-- name: CreateBotACLRule :one", "sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: UpdateBotACLRule :one", "AND team_id = sqlc.arg(team_id)::uuid")
	assertSQLContains(t, sql, "-- name: DeleteBotACLRuleByID :exec", "AND team_id = sqlc.arg(team_id)::uuid")
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	// #nosec G304 -- tests read fixed checked-in SQL query files.
	payload, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(payload)
}

func assertSQLContains(t *testing.T, sql, marker, want string) {
	t.Helper()
	block := sqlBlock(t, sql, marker)
	if !strings.Contains(block, want) {
		t.Fatalf("%s missing %q\nblock:\n%s", marker, want, block)
	}
}

func sqlBlock(t *testing.T, sql, marker string) string {
	t.Helper()
	start := strings.Index(sql, marker)
	if start < 0 {
		t.Fatalf("missing query marker %q", marker)
	}
	rest := sql[start+len(marker):]
	next := strings.Index(rest, "-- name:")
	if next < 0 {
		return sql[start:]
	}
	return sql[start : start+len(marker)+next]
}
