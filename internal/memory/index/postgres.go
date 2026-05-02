package index

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresIndex struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) (*PostgresIndex, error) {
	if pool == nil {
		return nil, errors.New("postgres memory index requires a database pool")
	}
	return &PostgresIndex{pool: pool}, nil
}

func (*PostgresIndex) Name() string { return "postgres-vector" }

func (i *PostgresIndex) Health(ctx context.Context) error {
	return i.pool.QueryRow(ctx, `SELECT 1`).Scan(new(int))
}

func (i *PostgresIndex) EnsureDense(ctx context.Context, dimensions int) error {
	if dimensions <= 0 {
		return fmt.Errorf("postgres memory index: dense dimensions must be positive, got %d", dimensions)
	}
	return i.ensureBase(ctx)
}

func (i *PostgresIndex) EnsureSparse(ctx context.Context) error {
	return i.ensureBase(ctx)
}

func (i *PostgresIndex) ensureBase(ctx context.Context) error {
	_, err := i.pool.Exec(ctx, `
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS memory_index_points (
  point_id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  source_entry_id TEXT NOT NULL,
  memory TEXT NOT NULL,
  hash TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  dense_dimension INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  indexed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_index_points_source_unique UNIQUE (bot_id, source_entry_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_index_points_bot_id ON memory_index_points(bot_id);
CREATE INDEX IF NOT EXISTS idx_memory_index_points_source_entry_id ON memory_index_points(source_entry_id);

CREATE TABLE IF NOT EXISTS memory_dense_vectors (
  point_id TEXT PRIMARY KEY REFERENCES memory_index_points(point_id) ON DELETE CASCADE,
  bot_id TEXT NOT NULL,
  dimension INTEGER NOT NULL,
  embedding vector NOT NULL,
  indexed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_memory_dense_vectors_bot_id ON memory_dense_vectors(bot_id);

CREATE TABLE IF NOT EXISTS memory_sparse_terms (
  point_id TEXT NOT NULL REFERENCES memory_index_points(point_id) ON DELETE CASCADE,
  bot_id TEXT NOT NULL,
  dim BIGINT NOT NULL,
  value REAL NOT NULL,
  PRIMARY KEY (point_id, dim)
);

CREATE INDEX IF NOT EXISTS idx_memory_sparse_terms_bot_dim ON memory_sparse_terms(bot_id, dim);
CREATE INDEX IF NOT EXISTS idx_memory_sparse_terms_point_id ON memory_sparse_terms(point_id);
`)
	if err != nil {
		return fmt.Errorf("postgres memory index: ensure schema: %w", err)
	}
	return nil
}

func (i *PostgresIndex) UpsertDense(ctx context.Context, id string, vec DenseVector, payload map[string]string) error {
	if err := i.EnsureDense(ctx, len(vec.Values)); err != nil {
		return err
	}
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := postgresUpsertPoint(ctx, tx, id, payload, len(vec.Values)); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO memory_dense_vectors (point_id, bot_id, dimension, embedding, indexed_at)
VALUES ($1, $2, $3, $4::vector, now())
ON CONFLICT (point_id) DO UPDATE
SET bot_id = EXCLUDED.bot_id,
    dimension = EXCLUDED.dimension,
    embedding = EXCLUDED.embedding,
    indexed_at = now()
`, id, payloadValue(payload, "bot_id"), len(vec.Values), vectorLiteral(vec.Values))
	if err != nil {
		return fmt.Errorf("postgres memory index: upsert dense vector: %w", err)
	}
	return tx.Commit(ctx)
}

func (i *PostgresIndex) UpsertSparse(ctx context.Context, id string, vec SparseVector, payload map[string]string) error {
	if err := i.EnsureSparse(ctx); err != nil {
		return err
	}
	if len(vec.Indices) != len(vec.Values) {
		return fmt.Errorf("postgres memory index: sparse vector length mismatch: %d indices, %d values", len(vec.Indices), len(vec.Values))
	}
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if err := postgresUpsertPoint(ctx, tx, id, payload, 0); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM memory_sparse_terms WHERE point_id = $1`, id); err != nil {
		return fmt.Errorf("postgres memory index: delete sparse terms: %w", err)
	}
	for j, dim := range vec.Indices {
		value := vec.Values[j]
		if value == 0 {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO memory_sparse_terms (point_id, bot_id, dim, value)
VALUES ($1, $2, $3, $4)
ON CONFLICT (point_id, dim) DO UPDATE
SET value = EXCLUDED.value,
    bot_id = EXCLUDED.bot_id
`, id, payloadValue(payload, "bot_id"), int64(dim), value); err != nil {
			return fmt.Errorf("postgres memory index: insert sparse term: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func postgresUpsertPoint(ctx context.Context, tx pgx.Tx, id string, payload map[string]string, denseDimension int) error {
	data, err := payloadJSON(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO memory_index_points (
  point_id, bot_id, source_entry_id, memory, hash, payload,
  dense_dimension, created_at, updated_at, indexed_at
)
VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, now())
ON CONFLICT (point_id) DO UPDATE
SET bot_id = EXCLUDED.bot_id,
    source_entry_id = EXCLUDED.source_entry_id,
    memory = EXCLUDED.memory,
    hash = EXCLUDED.hash,
    payload = EXCLUDED.payload,
    dense_dimension = CASE WHEN EXCLUDED.dense_dimension > 0 THEN EXCLUDED.dense_dimension ELSE memory_index_points.dense_dimension END,
    created_at = EXCLUDED.created_at,
    updated_at = EXCLUDED.updated_at,
    indexed_at = now()
`, id, payloadValue(payload, "bot_id"), payloadValue(payload, "source_entry_id"), payloadValue(payload, "memory"), payloadValue(payload, "hash"), string(data), denseDimension, payloadValue(payload, "created_at"), payloadValue(payload, "updated_at"))
	if err != nil {
		return fmt.Errorf("postgres memory index: upsert point: %w", err)
	}
	return nil
}

func (i *PostgresIndex) SearchDense(ctx context.Context, vec DenseVector, botID string, limit int) ([]SearchResult, error) {
	if err := i.EnsureDense(ctx, len(vec.Values)); err != nil {
		return nil, err
	}
	rows, err := i.pool.Query(ctx, `
SELECT p.point_id,
       1 - (v.embedding <=> $2::vector) AS score,
       p.payload
FROM memory_dense_vectors v
JOIN memory_index_points p ON p.point_id = v.point_id
WHERE p.bot_id = $1
  AND v.dimension = $3
ORDER BY v.embedding <=> $2::vector
LIMIT $4
`, strings.TrimSpace(botID), vectorLiteral(vec.Values), len(vec.Values), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("postgres memory index: search dense: %w", err)
	}
	defer rows.Close()
	return scanPostgresResults(rows)
}

func (i *PostgresIndex) SearchSparse(ctx context.Context, vec SparseVector, botID string, limit int) ([]SearchResult, error) {
	if err := i.EnsureSparse(ctx); err != nil {
		return nil, err
	}
	pairs, err := querySparsePairs(vec)
	if err != nil {
		return nil, err
	}
	rows, err := i.pool.Query(ctx, `
WITH query_terms AS (
  SELECT (value->>0)::bigint AS dim,
         (value->>1)::real AS value
  FROM jsonb_array_elements($2::jsonb)
),
scored AS (
  SELECT t.point_id, SUM(t.value * q.value)::double precision AS score
  FROM memory_sparse_terms t
  JOIN query_terms q ON q.dim = t.dim
  WHERE t.bot_id = $1
  GROUP BY t.point_id
)
SELECT p.point_id, scored.score, p.payload
FROM scored
JOIN memory_index_points p ON p.point_id = scored.point_id
ORDER BY scored.score DESC, p.updated_at DESC, p.point_id ASC
LIMIT $3
`, strings.TrimSpace(botID), string(pairs), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("postgres memory index: search sparse: %w", err)
	}
	defer rows.Close()
	return scanPostgresResults(rows)
}

func (i *PostgresIndex) ScrollDense(ctx context.Context, botID string, limit int) ([]SearchResult, error) {
	return i.scroll(ctx, botID, limit, true)
}

func (i *PostgresIndex) ScrollSparse(ctx context.Context, botID string, limit int) ([]SearchResult, error) {
	return i.scroll(ctx, botID, limit, false)
}

func (i *PostgresIndex) scroll(ctx context.Context, botID string, limit int, dense bool) ([]SearchResult, error) {
	if err := i.ensureBase(ctx); err != nil {
		return nil, err
	}
	query := `
SELECT p.point_id, 0::double precision AS score, p.payload
FROM memory_index_points p
WHERE p.bot_id = $1
  AND EXISTS (SELECT 1 FROM memory_sparse_terms s WHERE s.point_id = p.point_id)
ORDER BY p.indexed_at ASC, p.point_id ASC
LIMIT $2
`
	if dense {
		query = `
SELECT p.point_id, 0::double precision AS score, p.payload
FROM memory_index_points p
JOIN memory_dense_vectors v ON v.point_id = p.point_id
WHERE p.bot_id = $1
ORDER BY p.indexed_at ASC, p.point_id ASC
LIMIT $2
`
	}
	rows, err := i.pool.Query(ctx, query, strings.TrimSpace(botID), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("postgres memory index: scroll: %w", err)
	}
	defer rows.Close()
	return scanPostgresResults(rows)
}

func (i *PostgresIndex) CountDense(ctx context.Context, botID string) (int, error) {
	if err := i.ensureBase(ctx); err != nil {
		return 0, err
	}
	var count int
	err := i.pool.QueryRow(ctx, `
SELECT COUNT(*)
FROM memory_dense_vectors v
JOIN memory_index_points p ON p.point_id = v.point_id
WHERE p.bot_id = $1
`, strings.TrimSpace(botID)).Scan(&count)
	return count, err
}

func (i *PostgresIndex) CountSparse(ctx context.Context, botID string) (int, error) {
	if err := i.ensureBase(ctx); err != nil {
		return 0, err
	}
	var count int
	err := i.pool.QueryRow(ctx, `
SELECT COUNT(DISTINCT point_id)
FROM memory_sparse_terms
WHERE bot_id = $1
`, strings.TrimSpace(botID)).Scan(&count)
	return count, err
}

func (i *PostgresIndex) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := i.ensureBase(ctx); err != nil {
		return err
	}
	_, err := i.pool.Exec(ctx, `DELETE FROM memory_index_points WHERE point_id = ANY($1)`, ids)
	if err != nil {
		return fmt.Errorf("postgres memory index: delete by ids: %w", err)
	}
	return nil
}

func (i *PostgresIndex) DeleteByBotID(ctx context.Context, botID string) error {
	if err := i.ensureBase(ctx); err != nil {
		return err
	}
	_, err := i.pool.Exec(ctx, `DELETE FROM memory_index_points WHERE bot_id = $1`, strings.TrimSpace(botID))
	if err != nil {
		return fmt.Errorf("postgres memory index: delete by bot_id: %w", err)
	}
	return nil
}

func scanPostgresResults(rows pgx.Rows) ([]SearchResult, error) {
	results := []SearchResult{}
	for rows.Next() {
		var id string
		var score float64
		var raw []byte
		if err := rows.Scan(&id, &score, &raw); err != nil {
			return nil, err
		}
		if math.IsNaN(score) || math.IsInf(score, 0) {
			score = 0
		}
		results = append(results, SearchResult{
			ID:      id,
			Score:   score,
			Payload: parsePayload(raw),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
