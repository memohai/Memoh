//go:build integration

package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	memohdb "github.com/memohai/memoh/internal/db"
	postgresstore "github.com/memohai/memoh/internal/db/postgres/store"
)

const (
	packageTeamID   = "00000000-0000-0000-0000-000000000001"
	packageUserID   = "10000000-0000-4000-8000-000000000001"
	packageBotOneID = "20000000-0000-4000-8000-000000000001"
	packageBotTwoID = "20000000-0000-4000-8000-000000000002"
	packageOneID    = "30000000-0000-4000-8000-000000000001"
	pluginOneID     = "40000000-0000-4000-8000-000000000001"
	pluginTwoID     = "40000000-0000-4000-8000-000000000002"
	pluginOtherID   = "40000000-0000-4000-8000-000000000003"
	packageRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	otherRevision   = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestSkillPackageOwnershipConstraints(t *testing.T) {
	ctx := context.Background()
	pool := teamScopedPool(t)
	seedPackageBots(t, pool)

	if _, err := pool.Exec(ctx, `
		INSERT INTO bot_skill_package_installations
			(id, bot_id, workspace_target_id, registry_id, package_id, revision)
		VALUES ($1, $2, 'native', 'openai', 'documents', $3)`,
		packageOneID, packageBotOneID, packageRevision); err != nil {
		t.Fatalf("seed Package: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO bot_plugin_package_references
			(bot_id, workspace_target_id, plugin_installation_id, package_installation_id, required_revision)
		VALUES ($1, 'native', $2, $3, $4)`,
		packageBotOneID, pluginOneID, packageOneID, packageRevision); err != nil {
		t.Fatalf("reference Package: %v", err)
	}

	assertSQLState(t, pool, `
		INSERT INTO bot_plugin_package_references
			(bot_id, workspace_target_id, plugin_installation_id, package_installation_id, required_revision)
		VALUES ($1, 'native', $2, $3, $4)`, "23503", packageBotTwoID, pluginOtherID, packageOneID, packageRevision)
	assertSQLState(t, pool, `
		INSERT INTO bot_plugin_package_references
			(bot_id, workspace_target_id, plugin_installation_id, package_installation_id, required_revision)
		VALUES ($1, 'native', $2, $3, $4)`, "23503", packageBotOneID, pluginTwoID, packageOneID, otherRevision)
	assertSQLState(t, pool, `
		INSERT INTO bot_plugin_package_references
			(bot_id, workspace_target_id, plugin_installation_id, package_installation_id, required_revision)
		VALUES ($1, 'remote-target', $2, $3, $4)`, "23503", packageBotOneID, pluginTwoID, packageOneID, packageRevision)
	assertSQLState(t, pool, `
		INSERT INTO bot_skill_package_installations
			(bot_id, workspace_target_id, registry_id, package_id, revision)
		VALUES ($1, '', 'openai', 'empty-target', $2)`, "23514", packageBotOneID, packageRevision)
	assertSQLState(t, pool, `
		INSERT INTO bot_plugin_installations (bot_id, plugin_id, workspace_target_id)
		VALUES ($1, 'invalid-target', ' remote ')`, "23514", packageBotOneID)
}

func TestRemoteRuntimeDeletionRespectsPackageAndPluginOwners(t *testing.T) {
	for _, test := range []struct {
		name string
		seed func(context.Context, *pgxpool.Pool, string) error
	}{
		{name: "Package", seed: func(ctx context.Context, pool *pgxpool.Pool, targetID string) error {
			_, err := pool.Exec(ctx, `
				INSERT INTO bot_skill_package_installations
					(bot_id, workspace_target_id, registry_id, package_id, revision, directly_installed)
				VALUES ($1, $2, 'openai', 'documents', $3, true)`, packageBotOneID, targetID, packageRevision)
			return err
		}},
		{name: "Plugin", seed: func(ctx context.Context, pool *pgxpool.Pool, targetID string) error {
			_, err := pool.Exec(ctx, `UPDATE bot_plugin_installations SET workspace_target_id = $1, status = 'ready' WHERE id = $2`, targetID, pluginOneID)
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			pool := teamScopedPool(t)
			seedPackageBots(t, pool)
			targetID := seedRemoteRuntimeTarget(t, pool)
			if err := test.seed(ctx, pool, targetID); err != nil {
				t.Fatalf("seed owner: %v", err)
			}
			store, err := postgresstore.New(pool)
			if err != nil {
				t.Fatalf("create store: %v", err)
			}
			if err := store.DeleteMount(ctx, packageBotOneID, targetID); !errors.Is(err, memohdb.ErrNotFound) {
				t.Fatalf("DeleteMount() error = %v, want deletion to be rejected", err)
			}
		})
	}
}

func TestRemoteRuntimeDeletionIgnoresUninstalledPlugin(t *testing.T) {
	ctx := context.Background()
	pool := teamScopedPool(t)
	seedPackageBots(t, pool)
	targetID := seedRemoteRuntimeTarget(t, pool)
	if _, err := pool.Exec(ctx, `UPDATE bot_plugin_installations SET workspace_target_id = $1, status = 'uninstalled' WHERE id = $2`, targetID, pluginOneID); err != nil {
		t.Fatalf("seed uninstalled Plugin: %v", err)
	}
	store, err := postgresstore.New(pool)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := store.DeleteMount(ctx, packageBotOneID, targetID); err != nil {
		t.Fatalf("DeleteMount(): %v", err)
	}
}

func assertSQLState(t *testing.T, pool *pgxpool.Pool, query, want string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), query, args...)
	if got := sqlState(err); got != want {
		t.Fatalf("SQLSTATE = %q, want %q (error %v)", got, want, err)
	}
}

func seedRemoteRuntimeTarget(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	const runtimeID = "50000000-0000-4000-8000-000000000001"
	const targetID = "60000000-0000-4000-8000-000000000001"
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO user_runtimes (id, user_id, name, api_token) VALUES ($1, $2, 'package-runtime', 'token')`, runtimeID, packageUserID); err != nil {
		t.Fatalf("seed Remote Runtime: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO bot_remote_runtime_bindings (id, bot_id, runtime_id) VALUES ($1, $2, $3)`, targetID, packageBotOneID, runtimeID); err != nil {
		t.Fatalf("seed target: %v", err)
	}
	return targetID
}

func teamScopedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	_ = freshMigratedDB(t)
	cfg, err := pgxpool.ParseConfig(teamMigrationDSN(t))
	if err != nil {
		t.Fatalf("parse test DSN: %v", err)
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SELECT set_config('memoh.team_id', $1, false)`, packageTeamID)
		return err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("create team pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedPackageBots(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, username) VALUES ($1, 'package-owner')`, []any{packageUserID}},
		{`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, []any{packageTeamID, packageUserID}},
		{`INSERT INTO bots (id, team_id, owner_user_id, name) VALUES ($3, $2, $1, 'bot-one'), ($4, $2, $1, 'bot-two')`, []any{packageUserID, packageTeamID, packageBotOneID, packageBotTwoID}},
		{`INSERT INTO bot_plugin_installations (id, bot_id, plugin_id) VALUES ($1, $2, 'plugin-one'), ($3, $2, 'plugin-two'), ($4, $5, 'plugin-other')`, []any{pluginOneID, packageBotOneID, pluginTwoID, pluginOtherID, packageBotTwoID}},
	}
	for _, statement := range statements {
		if _, err := pool.Exec(ctx, statement.query, statement.args...); err != nil {
			t.Fatalf("seed Package ownership fixture: %v", err)
		}
	}
}
