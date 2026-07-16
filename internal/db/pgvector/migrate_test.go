package pgvector

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/pgvector/pgvector-go"

	"github.com/memohai/memoh/internal/config"
	pgvectorsqlc "github.com/memohai/memoh/internal/db/pgvector/sqlc"
)

func TestPGVectorMigrationsAreVersioned(t *testing.T) {
	t.Parallel()
	migrations, err := MigrationsFS()
	if err != nil {
		t.Fatalf("MigrationsFS: %v", err)
	}
	entries, err := fs.ReadDir(migrations, ".")
	if err != nil {
		t.Fatalf("read migrations: %v", err)
	}
	files := make(map[string]bool, len(entries))
	for _, entry := range entries {
		files[entry.Name()] = true
	}
	for version := uint(1); version <= SchemaVersion; version++ {
		prefix := fmt.Sprintf("%04d_", version)
		var up, down bool
		for name := range files {
			up = up || (strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".up.sql"))
			down = down || (strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".down.sql"))
		}
		if !up || !down {
			t.Fatalf("migration %d pair: up=%t down=%t", version, up, down)
		}
	}
}

func TestPGVectorMigrationAndQueriesIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_PGVECTOR_DSN")
	if dsn == "" {
		t.Skip("TEST_PGVECTOR_DSN is not set")
	}
	ctx := context.Background()
	adminCfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse pgvector DSN: %v", err)
	}
	adminConn, err := pgx.ConnectConfig(ctx, adminCfg)
	if err != nil {
		t.Fatalf("connect pgvector admin database: %v", err)
	}
	databaseName := fmt.Sprintf("memoh_pgvector_migration_test_%d", time.Now().UnixNano())
	quotedDatabase := pgx.Identifier{databaseName}.Sanitize()
	if _, err := adminConn.Exec(ctx, "CREATE DATABASE "+quotedDatabase); err != nil {
		_ = adminConn.Close(ctx)
		t.Fatalf("create isolated pgvector test database: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminConn.Exec(context.Background(), `
SELECT pg_terminate_backend(pid)
FROM pg_stat_activity
WHERE datname = $1 AND pid <> pg_backend_pid()
`, databaseName)
		_, _ = adminConn.Exec(context.Background(), "DROP DATABASE IF EXISTS "+quotedDatabase)
		_ = adminConn.Close(context.Background())
	})

	connCfg := adminCfg.Copy()
	connCfg.Database = databaseName
	conn, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		t.Fatalf("connect isolated pgvector database: %v", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	// Simulate a database created by the legacy component-local bootstrap. The
	// first standard migration must adopt it without losing existing rows.
	if _, err := conn.Exec(ctx, `
CREATE EXTENSION vector;
CREATE TABLE public.memory_node_embeddings (
    bot_id UUID NOT NULL,
    node_id TEXT NOT NULL,
    model_id UUID NOT NULL,
    dimensions INTEGER NOT NULL,
    body_hash TEXT NOT NULL DEFAULT '',
    embedding vector NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bot_id, node_id, model_id),
    CONSTRAINT memory_node_embeddings_dimensions_check CHECK (dimensions > 0)
);
INSERT INTO public.memory_node_embeddings (
    bot_id, node_id, model_id, dimensions, body_hash, embedding
) VALUES (
    '00000000-0000-0000-0000-000000000001',
    'legacy-node',
    '00000000-0000-0000-0000-000000000002',
    2,
    'legacy-hash',
    '[1,2]'::vector
);
`); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}

	vectorCfg := config.PGVectorConfig{
		Enabled:  true,
		Host:     connCfg.Host,
		Port:     int(connCfg.Port),
		User:     connCfg.User,
		Password: connCfg.Password,
		Database: databaseName,
		SSLMode:  "disable",
	}

	const workers = 12
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			errs <- MigrateUp(slog.New(slog.DiscardHandler), vectorCfg)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent migrate: %v", err)
		}
	}
	if err := MigrateUp(slog.New(slog.DiscardHandler), vectorCfg); err != nil {
		t.Fatalf("repeat migrate: %v", err)
	}

	var version int
	var dirty bool
	if err := conn.QueryRow(ctx, `SELECT version, dirty FROM public.schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read standard migration version: %v", err)
	}
	if version != int(SchemaVersion) || dirty {
		t.Fatalf("migration status = version %d dirty %t", version, dirty)
	}
	var legacyRows int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM public.memory_node_embeddings WHERE node_id = 'legacy-node'`).Scan(&legacyRows); err != nil {
		t.Fatalf("count legacy rows: %v", err)
	}
	if legacyRows != 1 {
		t.Fatalf("legacy rows = %d, want 1", legacyRows)
	}

	store, err := Open(ctx, slog.New(slog.DiscardHandler), vectorCfg)
	if err != nil {
		t.Fatalf("open typed store: %v", err)
	}
	defer store.Close()
	queries := store.Queries()
	botID := uuidValue(3)
	modelID := uuidValue(4)
	if err := queries.UpsertMemoryNodeEmbedding(ctx, pgvectorsqlc.UpsertMemoryNodeEmbeddingParams{
		BotID:      botID,
		NodeID:     "sqlc-node",
		ModelID:    modelID,
		Dimensions: 2,
		BodyHash:   "sqlc-hash",
		Embedding:  pgvector.NewVector([]float32{1, 0}),
	}); err != nil {
		t.Fatalf("sqlc upsert: %v", err)
	}
	rows, err := queries.SearchMemoryNodeEmbeddings(ctx, pgvectorsqlc.SearchMemoryNodeEmbeddingsParams{
		Embedding: pgvector.NewVector([]float32{1, 0}),
		BotID:     botID,
		ModelID:   modelID,
		RowLimit:  5,
	})
	if err != nil {
		t.Fatalf("sqlc search: %v", err)
	}
	if len(rows) != 1 || rows[0].NodeID != "sqlc-node" {
		t.Fatalf("sqlc search rows = %#v", rows)
	}

	if _, err := conn.Exec(ctx, `DROP TABLE public.memory_node_embeddings`); err != nil {
		t.Fatalf("drop embeddings table: %v", err)
	}
	if _, err := queries.MemoryNodeEmbeddingsExist(ctx); err == nil {
		t.Fatal("read-only health query repaired a missing table")
	}
	if err := MigrateUp(slog.New(slog.DiscardHandler), vectorCfg); err != nil {
		t.Fatalf("migrate current version: %v", err)
	}
	var tableExists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.memory_node_embeddings') IS NOT NULL`).Scan(&tableExists); err != nil {
		t.Fatalf("inspect embeddings table: %v", err)
	}
	if tableExists {
		t.Fatal("current migration version unexpectedly repaired schema drift")
	}

	if _, err := conn.Exec(ctx, `UPDATE public.schema_migrations SET version = $1`, SchemaVersion+1); err != nil {
		t.Fatalf("set future schema version: %v", err)
	}
	if err := MigrateUp(slog.New(slog.DiscardHandler), vectorCfg); err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("future schema version error = %v", err)
	}
}

func uuidValue(lastByte byte) pgtype.UUID {
	var bytes [16]byte
	bytes[15] = lastByte
	return pgtype.UUID{Bytes: bytes, Valid: true}
}
