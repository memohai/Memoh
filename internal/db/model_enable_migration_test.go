package db

import (
	"context"
	"database/sql"
	"strings"
	"testing"
)

func TestSQLiteModelEnableMigrationFreshReplay(t *testing.T) {
	migrations := sqliteMigrationsFS(t)
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("fresh full migrate up failed: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	schema := sqliteTableSQL(t, db, "models")
	if n := strings.Count(schema, "enable"); n != 1 {
		t.Fatalf("models.enable appears %d times in fresh schema, want exactly 1:\n%s", n, schema)
	}
}

func TestSQLiteModelEnableMigrationPreservesExistingModels(t *testing.T) {
	dsn := tempSQLiteMigrationDSN(t)

	db := openMigrationSQLite(t, dsn)
	ctx := context.Background()
	createPreModelEnableSchema(t, db)
	if _, err := db.ExecContext(ctx, `INSERT INTO providers(id,name,client_type,config,metadata) VALUES('00000000-0000-0000-0000-000000000701','Provider','openai-completions','{}','{}')`); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO models(id,model_id,name,provider_id,type,config,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000702','legacy-gpt','Legacy GPT','00000000-0000-0000-0000-000000000701','chat','{}','2026-01-02 03:04:04','2026-01-02 03:04:05')`); err != nil {
		t.Fatalf("insert legacy model: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO model_variants(id,model_uuid,variant_id,weight,metadata) VALUES('00000000-0000-0000-0000-000000000703','00000000-0000-0000-0000-000000000702','legacy-gpt-mini',1,'{}')`); err != nil {
		t.Fatalf("insert model variant: %v", err)
	}
	closeMigrationSQLite(t, db)

	migrations := sqliteMigrationsFSUpTo(t, 22)
	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "force", []string{"21"}); err != nil {
		t.Fatalf("force migration version 21: %v", err)
	}
	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("migrate through 0022_model_enable: %v", err)
	}

	db = openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	var enable int
	if err := db.QueryRowContext(ctx, `SELECT enable FROM models WHERE id='00000000-0000-0000-0000-000000000702'`).Scan(&enable); err != nil {
		t.Fatalf("select migrated enable: %v", err)
	}
	if enable != 1 {
		t.Fatalf("migrated enable = %d, want 1", enable)
	}
	var config string
	var updatedAt string
	if err := db.QueryRowContext(ctx, `SELECT config, updated_at FROM models WHERE id='00000000-0000-0000-0000-000000000702'`).Scan(&config, &updatedAt); err != nil {
		t.Fatalf("select migrated config/timestamp: %v", err)
	}
	if config != "{}" {
		t.Fatalf("migrated config = %q, want {}", config)
	}
	if updatedAt != "2026-01-02 03:04:05" {
		t.Fatalf("migrated updated_at = %q, want original timestamp", updatedAt)
	}

	var variantModelID string
	if err := db.QueryRowContext(ctx, `SELECT model_uuid FROM model_variants WHERE id='00000000-0000-0000-0000-000000000703'`).Scan(&variantModelID); err != nil {
		t.Fatalf("select model variant: %v", err)
	}
	if variantModelID != "00000000-0000-0000-0000-000000000702" {
		t.Fatalf("model variant model_uuid = %q, want preserved model id", variantModelID)
	}
	variantSchema := sqliteTableSQL(t, db, "model_variants")
	if strings.Contains(variantSchema, "models_old") {
		t.Fatalf("model_variants FK references models_old after migration:\n%s", variantSchema)
	}
}

func TestSQLiteModelEnableMigrationDownDropsEnableSafely(t *testing.T) {
	migrations := sqliteMigrationsFS(t)
	dsn := tempSQLiteMigrationDSN(t)

	if err := RunMigrateTarget(nil, MigrationTarget{Driver: DriverSQLite, DSN: dsn}, migrations, "up", nil); err != nil {
		t.Fatalf("migrate up: %v", err)
	}

	db := openMigrationSQLite(t, dsn)
	defer closeMigrationSQLite(t, db)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO providers(id,name,client_type,config,metadata) VALUES('00000000-0000-0000-0000-000000000711','Provider','openai-completions','{}','{}')`); err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO models(id,model_id,name,provider_id,type,enable,config) VALUES('00000000-0000-0000-0000-000000000712','rollback-gpt','Rollback GPT','00000000-0000-0000-0000-000000000711','chat',0,'{}')`); err != nil {
		t.Fatalf("insert model: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO model_variants(id,model_uuid,variant_id,weight,metadata) VALUES('00000000-0000-0000-0000-000000000713','00000000-0000-0000-0000-000000000712','rollback-gpt-mini',1,'{}')`); err != nil {
		t.Fatalf("insert model variant: %v", err)
	}

	downSQL := readEmbeddedMigration(t, "sqlite/migrations/0022_model_enable.down.sql")
	if _, err := db.ExecContext(ctx, downSQL); err != nil {
		t.Fatalf("execute 0022 down migration: %v", err)
	}

	schema := sqliteTableSQL(t, db, "models")
	if strings.Contains(schema, "enable") {
		t.Fatalf("models.enable still present after 0022 down:\n%s", schema)
	}
	if rows := sqliteForeignKeyCheckRows(t, db); rows != 0 {
		t.Fatalf("foreign key check rows after 0022 down = %d, want 0", rows)
	}
}

func createPreModelEnableSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
CREATE TABLE providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  client_type TEXT NOT NULL DEFAULT 'openai-completions',
  icon TEXT,
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT providers_name_unique UNIQUE (name)
);

CREATE TABLE models (
  id TEXT PRIMARY KEY,
  model_id TEXT NOT NULL,
  name TEXT,
  provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'chat',
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT models_provider_id_model_id_unique UNIQUE (provider_id, model_id),
  CONSTRAINT models_type_check CHECK (type IN ('chat', 'embedding', 'speech', 'transcription'))
);

CREATE TABLE model_variants (
  id TEXT PRIMARY KEY,
  model_uuid TEXT NOT NULL REFERENCES models(id) ON DELETE CASCADE,
  variant_id TEXT NOT NULL,
  weight INTEGER NOT NULL,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Minimal stub so later bot-level settings migrations can apply while this
-- fixture focuses on preserving legacy model rows.
CREATE TABLE bots (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT
);

-- Minimal stub of bot_sessions so migrations targeting that table (e.g.
-- the 0023 paged-sessions index) parse and apply against this pre-22
-- fixture. Columns mirror only what newer migrations reference; this is
-- not a full replica of the production schema.
CREATE TABLE bot_sessions (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  deleted_at TEXT
);
`)
	if err != nil {
		t.Fatalf("create pre-model-enable schema: %v", err)
	}
}

func sqliteForeignKeyCheckRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("run foreign key check: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Fatalf("close foreign key check rows: %v", err)
		}
	}()
	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate foreign key check rows: %v", err)
	}
	return count
}
