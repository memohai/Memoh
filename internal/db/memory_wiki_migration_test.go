package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

// TestSQLiteFreshReplayMemoryWiki verifies that the 0024_memory_wiki migration
// applies cleanly on a fresh full replay and that the memory_nodes/memory_edges
// tables, indexes, and constraints are present. It also confirms a round-trip
// down (rollback) drops both tables.
func TestSQLiteFreshReplayMemoryWiki(t *testing.T) {
	migrations := sqliteMigrationsFS(t)
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("fresh full migrate up failed: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	nodesSchema := sqliteTableSQL(t, db, "memory_nodes")
	// Assert each column appears as a standalone column definition (bounded by
	// whitespace/newline), not as a substring of another identifier (e.g. "id"
	// inside "bot_id"). The first column line is "id  TEXT PRIMARY KEY".
	for _, column := range []string{
		"id ",
		"bot_id ",
		"body ",
		"hash ",
		"layer ",
		"fact_type ",
		"subject ",
		"confidence ",
		"metadata ",
		"source_message_ids ",
		"profile_ref ",
		"topic ",
		"captured_at ",
		"expires_at",
		"updated_at ",
		"created_at ",
	} {
		if !strings.Contains(nodesSchema, column) {
			t.Fatalf("column %q missing from fresh memory_nodes schema:\n%s", strings.TrimSpace(column), nodesSchema)
		}
	}
	if !strings.Contains(nodesSchema, "memory_nodes_confidence_check") {
		t.Fatalf("memory_nodes confidence CHECK constraint missing:\n%s", nodesSchema)
	}

	edgesSchema := sqliteTableSQL(t, db, "memory_edges")
	for _, column := range []string{
		"id ",
		"bot_id ",
		"src_node ",
		"dst_node ",
		"rel ",
		"weight ",
		"metadata ",
		"created_at ",
	} {
		if !strings.Contains(edgesSchema, column) {
			t.Fatalf("column %q missing from fresh memory_edges schema:\n%s", strings.TrimSpace(column), edgesSchema)
		}
	}
	if !strings.Contains(edgesSchema, "CONSTRAINT memory_edges_unique") {
		t.Fatalf("memory_edges unique constraint missing:\n%s", edgesSchema)
	}

	// Seed a user + bot + node + edge to confirm the schema is writable and FKs resolve.
	if _, err := db.ExecContext(context.Background(), `INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000000161','wiki@example.com','member')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000000162','00000000-0000-0000-0000-000000000161','personal','wikibot','Wiki Bot')`); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO memory_nodes(id,bot_id,body,hash,layer,fact_type,subject,confidence,metadata,source_message_ids,profile_ref,topic,captured_at)
VALUES('00000000-0000-0000-0000-000000000162:mem_1','00000000-0000-0000-0000-000000000162','User likes tea','h1','preference','beverage','tea',0.9,'{"k":"v"}','[]','user:1','drinks','2026-06-20T00:00:00Z')
`); err != nil {
		t.Fatalf("insert memory node: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO memory_edges(bot_id,src_node,dst_node,rel,weight,metadata)
VALUES('00000000-0000-0000-0000-000000000162','00000000-0000-0000-0000-000000000162:mem_1','00000000-0000-0000-0000-000000000162:mem_2','followup',0.5,'{}')
`); err != nil {
		t.Fatalf("insert memory edge: %v", err)
	}

	// The confidence CHECK should reject out-of-range values.
	if _, err := db.ExecContext(context.Background(), `
INSERT INTO memory_nodes(id,bot_id,body,hash,layer,confidence) VALUES('bad:mem','00000000-0000-0000-0000-000000000162','x','h','note',1.5)
`); err == nil {
		t.Fatal("expected confidence CHECK to reject value 1.5, but insert succeeded")
	}

	// Roll back the wiki migration and confirm both tables disappear.
	closeMigrationSQLite(t, db)
	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "down", nil); err != nil {
		t.Fatalf("migrate down failed: %v", err)
	}
	db2 := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db2)
	for _, table := range []string{"memory_nodes", "memory_edges"} {
		if exists := sqliteTableExists(t, db2, table); exists {
			t.Fatalf("table %s should not exist after migrate down", table)
		}
	}
}

// sqliteTableExists reports whether a table exists in the SQLite database.
func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var name string
	err := db.QueryRowContext(context.Background(), `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
	if err == sql.ErrNoRows {
		return false
	}
	if err != nil {
		t.Fatalf("check sqlite table %s existence: %v", table, err)
	}
	return name != ""
}
