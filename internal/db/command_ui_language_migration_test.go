package db

import (
	"context"
	"strings"
	"testing"
)

// TestSQLiteFreshReplayCommandUILanguageColumn guards a subtle, catastrophic
// ordering dependency in the SQLite migrations.
//
// The 0001 baseline CREATE TABLE bots already includes command_ui_language, AND
// 0011 adds it again via a bare `ALTER TABLE bots ADD COLUMN` (SQLite has no
// `IF NOT EXISTS` for ADD COLUMN). A fresh install replays every migration in
// order, so this only works because 0008 and 0010 fully rebuild the bots table
// (CREATE TABLE ... INSERT SELECT) WITHOUT command_ui_language — dropping the
// 0001 column so 0011 can re-add it cleanly, exactly once.
//
// If anyone later adds a bots-table rebuild between 0010 and 0011 that DOES
// carry the column forward, a fresh `up` would hit a "duplicate column name"
// error at 0011 and every new install would fail. This test fails the instant
// that happens. (Already-migrated installs are unaffected; they only run the
// incremental 0011.)
func TestSQLiteFreshReplayCommandUILanguageColumn(t *testing.T) {
	migrations := sqliteMigrationsFS(t)
	dsn := tempSQLiteMigrationDSN(t)

	// A fresh full replay must not error (no duplicate-column collision).
	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("fresh full migrate up failed (duplicate-column landmine?): %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	// The column must exist exactly once in the final bots schema.
	schema := sqliteTableSQL(t, db, "bots")
	if n := strings.Count(schema, "command_ui_language"); n != 1 {
		t.Fatalf("command_ui_language appears %d times in fresh bots schema, want exactly 1:\n%s", n, schema)
	}

	// And it must be usable with the expected default.
	if _, err := db.ExecContext(context.Background(), `INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-0000000000a1','lang@example.com','member')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-0000000000a2','00000000-0000-0000-0000-0000000000a1','personal','langbot','Lang Bot')`); err != nil {
		t.Fatalf("insert bot: %v", err)
	}
	var lang string
	if err := db.QueryRowContext(context.Background(), `SELECT command_ui_language FROM bots WHERE id='00000000-0000-0000-0000-0000000000a2'`).Scan(&lang); err != nil {
		t.Fatalf("select command_ui_language: %v", err)
	}
	if lang != "auto" {
		t.Fatalf("fresh bot command_ui_language default = %q, want %q", lang, "auto")
	}
}

func TestSQLiteFreshReplayReasoningEffortLadder(t *testing.T) {
	migrations := sqliteMigrationsFS(t)
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("fresh full migrate up failed: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	if _, err := db.ExecContext(context.Background(), `INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-0000000000b1','reasoning@example.com','member')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	for _, tc := range []struct {
		id     string
		name   string
		effort string
	}{
		{"00000000-0000-0000-0000-0000000000b2", "reason-none", "none"},
		{"00000000-0000-0000-0000-0000000000b3", "reason-xhigh", "xhigh"},
	} {
		_, err := db.ExecContext(context.Background(), `INSERT INTO bots(id,owner_user_id,type,name,display_name,reasoning_effort) VALUES(?,?,?,?,?,?)`,
			tc.id, "00000000-0000-0000-0000-0000000000b1", "personal", tc.name, tc.name, tc.effort)
		if err != nil {
			t.Fatalf("insert bot with reasoning_effort=%q: %v", tc.effort, err)
		}
	}
}
