package db

import (
	"context"
	"strings"
	"testing"
)

func TestSQLiteFreshReplayPluginSystem(t *testing.T) {
	migrations := sqliteMigrationsFS(t)
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("fresh full migrate up failed: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	mcpSchema := sqliteTableSQL(t, db, "mcp_connections")
	for _, column := range []string{
		"managed_by_plugin_installation_id",
		"managed_resource_key",
		"visible",
		"metadata",
	} {
		if n := strings.Count(mcpSchema, column); n != 1 {
			t.Fatalf("%s appears %d times in fresh mcp_connections schema, want exactly 1:\n%s", column, n, mcpSchema)
		}
	}
	if schema := sqliteTableSQL(t, db, "bot_plugin_installations"); !strings.Contains(schema, "CONSTRAINT bot_plugin_installations_unique") {
		t.Fatalf("bot_plugin_installations unique constraint missing:\n%s", schema)
	}
	if schema := sqliteTableSQL(t, db, "bot_plugin_resources"); !strings.Contains(schema, "CONSTRAINT bot_plugin_resources_unique") {
		t.Fatalf("bot_plugin_resources unique constraint missing:\n%s", schema)
	}

	if _, err := db.ExecContext(context.Background(), `INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000000151','plugin@example.com','member')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000000152','00000000-0000-0000-0000-000000000151','personal','pluginbot','Plugin Bot')`); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO bot_plugin_installations(id,bot_id,plugin_id,plugin_name,version)
VALUES('00000000-0000-0000-0000-000000000153','00000000-0000-0000-0000-000000000152','notion','Notion','0.1.0')
`); err != nil {
		t.Fatalf("insert plugin installation: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO mcp_connections(id,bot_id,name,type,managed_by_plugin_installation_id,managed_resource_key,visible,metadata)
VALUES('00000000-0000-0000-0000-000000000154','00000000-0000-0000-0000-000000000152','notion_notion','stdio','00000000-0000-0000-0000-000000000153','notion',0,'{"plugin_id":"notion"}')
`); err != nil {
		t.Fatalf("insert plugin-managed mcp connection: %v", err)
	}
}
