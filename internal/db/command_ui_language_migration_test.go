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

func TestSQLiteRelaxReasoningPreservesFetchProviderID(t *testing.T) {
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFSUpTo(t, 18), "up", nil); err != nil {
		t.Fatalf("migrate through 0018_fetch_providers: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	if _, err := db.ExecContext(context.Background(), `INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-0000000000c1','fetch@example.com','member')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO fetch_providers(id,name,provider) VALUES('fetch-provider-preserve','Fetch Preserve','generic')`); err != nil {
		t.Fatalf("insert fetch provider: %v", err)
	}
	if _, err := db.ExecContext(context.Background(), `INSERT INTO bots(id,owner_user_id,type,name,display_name,fetch_provider_id) VALUES('00000000-0000-0000-0000-0000000000c2','00000000-0000-0000-0000-0000000000c1','personal','fetch-bot','Fetch Bot','fetch-provider-preserve')`); err != nil {
		t.Fatalf("insert bot with fetch_provider_id: %v", err)
	}
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through 0019_relax_reasoning_effort: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	schema := sqliteTableSQL(t, db, "bots")
	if n := strings.Count(schema, "fetch_provider_id"); n != 1 {
		t.Fatalf("fetch_provider_id appears %d times after 0019 rebuild, want exactly 1:\n%s", n, schema)
	}

	var providerID string
	if err := db.QueryRowContext(context.Background(), `SELECT fetch_provider_id FROM bots WHERE id='00000000-0000-0000-0000-0000000000c2'`).Scan(&providerID); err != nil {
		t.Fatalf("select fetch_provider_id after 0019 rebuild: %v", err)
	}
	if providerID != "fetch-provider-preserve" {
		t.Fatalf("fetch_provider_id after 0019 rebuild = %q, want %q", providerID, "fetch-provider-preserve")
	}
}

func TestSQLiteSessionBranchesMigratesExistingSessionHistory(t *testing.T) {
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFSUpTo(t, 20), "up", nil); err != nil {
		t.Fatalf("migrate through 0020_channel_access_redesign: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	statements := []string{
		`INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000000201','branch-migrate@example.com','member')`,
		`INSERT INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000000202','00000000-0000-0000-0000-000000000201','personal','branchbot','Branch Bot')`,
		`INSERT INTO bot_sessions(id,bot_id,type,title,metadata,created_by_user_id,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000203','00000000-0000-0000-0000-000000000202','chat','Branch Session','{}','00000000-0000-0000-0000-000000000201','2026-01-01 00:00:00','2026-01-01 00:00:02')`,
		`INSERT INTO bot_history_messages(id,bot_id,session_id,role,content,metadata,created_at) VALUES('00000000-0000-0000-0000-000000000204','00000000-0000-0000-0000-000000000202','00000000-0000-0000-0000-000000000203','user','{"content":"hello"}','{}','2026-01-01 00:00:01')`,
		`INSERT INTO bot_history_messages(id,bot_id,session_id,role,content,metadata,created_at) VALUES('00000000-0000-0000-0000-000000000205','00000000-0000-0000-0000-000000000202','00000000-0000-0000-0000-000000000203','assistant','{"content":"hi"}','{}','2026-01-01 00:00:02')`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
	closeMigrationSQLite(t, db)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, sqliteMigrationsFS(t), "up", nil); err != nil {
		t.Fatalf("migrate through 0022_session_branches: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	sessionSchema := sqliteTableSQL(t, db, "bot_sessions")
	if strings.Contains(sessionSchema, "bot_sessions_old") {
		t.Fatalf("bot_sessions schema still references bot_sessions_old:\n%s", sessionSchema)
	}
	messageSchema := sqliteTableSQL(t, db, "bot_history_messages")
	if strings.Contains(messageSchema, "bot_sessions_old") {
		t.Fatalf("bot_history_messages schema still references bot_sessions_old:\n%s", messageSchema)
	}

	var activeBranchID string
	if err := db.QueryRowContext(context.Background(), `SELECT active_branch_id FROM bot_sessions WHERE id='00000000-0000-0000-0000-000000000203'`).Scan(&activeBranchID); err != nil {
		t.Fatalf("select active_branch_id: %v", err)
	}
	if activeBranchID == "" {
		t.Fatal("active_branch_id was not backfilled")
	}

	var rootCount int
	if err := db.QueryRowContext(context.Background(), `
SELECT count(*)
FROM bot_session_branches
WHERE id = ?
  AND session_id = '00000000-0000-0000-0000-000000000203'
  AND parent_branch_id IS NULL
  AND fork_from_message_id IS NULL
`, activeBranchID).Scan(&rootCount); err != nil {
		t.Fatalf("count root branches: %v", err)
	}
	if rootCount != 1 {
		t.Fatalf("root branch count for active branch = %d, want 1", rootCount)
	}

	rows, err := db.QueryContext(context.Background(), `
SELECT id, branch_id, branch_seq
FROM bot_history_messages
WHERE session_id = '00000000-0000-0000-0000-000000000203'
ORDER BY created_at ASC, id ASC
`)
	if err != nil {
		t.Fatalf("select migrated messages: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("close migrated messages rows: %v", err)
		}
	}()

	wantSeq := int64(1)
	for rows.Next() {
		var id, branchID string
		var branchSeq int64
		if err := rows.Scan(&id, &branchID, &branchSeq); err != nil {
			t.Fatalf("scan migrated message: %v", err)
		}
		if branchID != activeBranchID {
			t.Fatalf("message %s branch_id = %q, want active root branch %q", id, branchID, activeBranchID)
		}
		if branchSeq != wantSeq {
			t.Fatalf("message %s branch_seq = %d, want %d", id, branchSeq, wantSeq)
		}
		wantSeq++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate migrated messages: %v", err)
	}
	if wantSeq != 3 {
		t.Fatalf("migrated message count = %d, want 2", wantSeq-1)
	}
}
