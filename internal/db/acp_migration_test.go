package db

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	embeddeddb "github.com/memohai/memoh/db"
	"github.com/memohai/memoh/internal/config"
)

func TestSQLiteACPAgentSessionTypeMigration(t *testing.T) {
	migrations := sqliteMigrationsFS(t)

	t.Run("down succeeds without acp agent rows", func(t *testing.T) {
		dsn := tempSQLiteMigrationDSN(t)
		if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		db := openMigrationSQLite(t, dsn)
		if schema := sqliteTableSQL(t, db, "bot_sessions"); !strings.Contains(schema, "acp_agent") {
			t.Fatalf("bot_sessions CHECK missing acp_agent after up: %s", schema)
		}
		closeMigrationSQLite(t, db)

		if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "down", nil); err != nil {
			t.Fatalf("migrate down without acp_agent rows: %v", err)
		}
	})

	t.Run("down fails before rebuild when acp agent rows exist", func(t *testing.T) {
		dsn := tempSQLiteMigrationDSN(t)
		if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
			t.Fatalf("migrate up: %v", err)
		}
		db := openMigrationSQLite(t, dsn)
		insertSQLiteACPAgentSession(t, db)
		before := sqliteTableSQL(t, db, "bot_sessions")
		closeMigrationSQLite(t, db)

		if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "down", nil); err == nil {
			t.Fatal("migrate down succeeded with acp_agent rows; want guard failure")
		}

		db = openMigrationSQLite(t, dsn)
		defer closeMigrationSQLite(t, db)
		after := sqliteTableSQL(t, db, "bot_sessions")
		if before != after || !strings.Contains(after, "acp_agent") {
			t.Fatalf("guard failure should leave bot_sessions schema unchanged\nbefore: %s\nafter: %s", before, after)
		}
		var count int
		if err := db.QueryRowContext(context.Background(), `SELECT count(*) FROM bot_sessions WHERE type = 'acp_agent'`).Scan(&count); err != nil {
			t.Fatalf("count acp sessions: %v", err)
		}
		if count != 1 {
			t.Fatalf("acp_agent row count = %d, want 1", count)
		}
	})
}

func TestPostgresACPAgentSessionTypeMigrationFiles(t *testing.T) {
	baseline := readEmbeddedMigration(t, "postgres/migrations/0001_init.up.sql")
	if !strings.Contains(baseline, "type IN ('chat', 'heartbeat', 'schedule', 'subagent', 'discuss', 'acp_agent')") {
		t.Fatal("postgres baseline bot_sessions type CHECK missing acp_agent")
	}
	up := readEmbeddedMigration(t, "postgres/migrations/0082_acp_agent_session_type.up.sql")
	if !strings.Contains(up, "DROP CONSTRAINT IF EXISTS bot_sessions_type_check") ||
		!strings.Contains(up, "'acp_agent'") {
		t.Fatal("postgres 0082 up migration does not widen bot_sessions_type_check to acp_agent")
	}
	down := readEmbeddedMigration(t, "postgres/migrations/0082_acp_agent_session_type.down.sql")
	if !strings.Contains(down, "WHERE type = 'acp_agent'") ||
		!strings.Contains(down, "RAISE EXCEPTION") {
		t.Fatal("postgres 0082 down migration must guard existing acp_agent rows without touching tool approvals")
	}
}

func TestToolApprovalRequestsConstrainOperationNotToolName(t *testing.T) {
	for _, path := range []string{
		"postgres/migrations/0001_init.up.sql",
		"sqlite/migrations/0001_init.up.sql",
	} {
		t.Run(path, func(t *testing.T) {
			sql := readEmbeddedMigration(t, path)
			tableSQL := toolApprovalTableSQL(sql)
			const operationColumn = "operation TEXT NOT NULL"
			if !strings.Contains(tableSQL, operationColumn) {
				t.Fatalf("%s missing tool approval operation column %q", path, operationColumn)
			}
			const operationCheck = "CHECK (operation IN ('read', 'write', 'exec', 'tool'))"
			if !strings.Contains(tableSQL, operationCheck) {
				t.Fatalf("%s missing Memoh-native tool approval operation CHECK %q", path, operationCheck)
			}
			if strings.Contains(tableSQL, "CHECK (tool_name IN") {
				t.Fatalf("%s tool_approval_requests must not constrain real tool_name values", path)
			}
			if strings.Contains(tableSQL, "acp_agent") {
				t.Fatalf("%s tool_approval_requests CHECK must not include ACP tools", path)
			}
		})
	}
}

func sqliteMigrationsFS(t *testing.T) fs.FS {
	t.Helper()
	migrations, err := fs.Sub(embeddeddb.MigrationsFS, "sqlite/migrations")
	if err != nil {
		t.Fatalf("sqlite migrations fs: %v", err)
	}
	return migrations
}

func sqliteMigrationsFSUpTo(t *testing.T, maxVersion int) fs.FS {
	t.Helper()
	migrations := sqliteMigrationsFS(t)
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatalf("read sqlite migrations: %v", err)
	}

	out := fstest.MapFS{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		sep := strings.IndexByte(name, '_')
		if sep <= 0 {
			continue
		}
		version, err := strconv.Atoi(name[:sep])
		if err != nil || version > maxVersion {
			continue
		}
		data, err := fs.ReadFile(migrations, name)
		if err != nil {
			t.Fatalf("read sqlite migration %s: %v", name, err)
		}
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}

func tempSQLiteMigrationDSN(t *testing.T) string {
	t.Helper()
	return "sqlite://" + filepath.Join(t.TempDir(), "memoh.db")
}

func openMigrationSQLite(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := OpenSQLite(context.Background(), config.SQLiteConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

func closeMigrationSQLite(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
}

func sqliteTableSQL(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var schema string
	if err := db.QueryRowContext(context.Background(), `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&schema); err != nil {
		t.Fatalf("read sqlite schema for %s: %v", table, err)
	}
	return schema
}

func insertSQLiteACPAgentSession(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`INSERT INTO users(id,email,role) VALUES('00000000-0000-0000-0000-000000000001','acp@example.com','member')`,
		`INSERT INTO bots(id,owner_user_id,type,name,display_name) VALUES('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','personal','acp-bot','ACP Bot')`,
		`INSERT INTO bot_sessions(id,bot_id,type,title,metadata) VALUES('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000002','acp_agent','Codex','{}')`,
	}
	for _, stmt := range statements {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("exec %q: %v", stmt, err)
		}
	}
}

func toolApprovalTableSQL(sql string) string {
	start := strings.Index(sql, "CREATE TABLE IF NOT EXISTS tool_approval_requests")
	if start < 0 {
		return ""
	}
	tail := sql[start:]
	end := strings.Index(tail, "CREATE INDEX")
	if end < 0 {
		return tail
	}
	return tail[:end]
}

func readEmbeddedMigration(t *testing.T, path string) string {
	t.Helper()
	data, err := embeddeddb.MigrationsFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", path, err)
	}
	return string(data)
}
