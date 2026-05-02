package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
)

type SQLiteIndex struct {
	db *sql.DB
}

type sqliteExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func NewSQLite(db *sql.DB) (*SQLiteIndex, error) {
	if db == nil {
		return nil, errors.New("sqlite memory index requires a database handle")
	}
	return &SQLiteIndex{db: db}, nil
}

func (*SQLiteIndex) Name() string { return "sqlite-vec" }

func (i *SQLiteIndex) Health(ctx context.Context) error {
	var version string
	return i.db.QueryRowContext(ctx, `SELECT vec_version()`).Scan(&version)
}

func (i *SQLiteIndex) EnsureDense(ctx context.Context, dimensions int) error {
	if err := i.ensureBase(ctx); err != nil {
		return err
	}
	table, err := denseVectorTable(dimensions)
	if err != nil {
		return err
	}
	_, err = i.db.ExecContext(ctx, fmt.Sprintf(`CREATE VIRTUAL TABLE IF NOT EXISTS %s USING vec0(embedding float[%d])`, table, dimensions))
	if err != nil {
		return fmt.Errorf("sqlite memory index: ensure dense vector table: %w", err)
	}
	return nil
}

func (i *SQLiteIndex) EnsureSparse(ctx context.Context) error {
	return i.ensureBase(ctx)
}

func (i *SQLiteIndex) ensureBase(ctx context.Context) error {
	_, err := i.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS memory_index_points (
  point_id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL,
  source_entry_id TEXT NOT NULL,
  memory TEXT NOT NULL,
  hash TEXT NOT NULL DEFAULT '',
  payload TEXT NOT NULL DEFAULT '{}',
  dense_dimension INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  indexed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT memory_index_points_source_unique UNIQUE (bot_id, source_entry_id)
);

CREATE INDEX IF NOT EXISTS idx_memory_index_points_bot_id ON memory_index_points(bot_id);
CREATE INDEX IF NOT EXISTS idx_memory_index_points_source_entry_id ON memory_index_points(source_entry_id);

CREATE TABLE IF NOT EXISTS memory_dense_rowids (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  point_id TEXT NOT NULL UNIQUE REFERENCES memory_index_points(point_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS memory_sparse_terms (
  point_id TEXT NOT NULL REFERENCES memory_index_points(point_id) ON DELETE CASCADE,
  bot_id TEXT NOT NULL,
  dim INTEGER NOT NULL,
  value REAL NOT NULL,
  PRIMARY KEY (point_id, dim)
);

CREATE INDEX IF NOT EXISTS idx_memory_sparse_terms_bot_dim ON memory_sparse_terms(bot_id, dim);
CREATE INDEX IF NOT EXISTS idx_memory_sparse_terms_point_id ON memory_sparse_terms(point_id);
`)
	if err != nil {
		return fmt.Errorf("sqlite memory index: ensure schema: %w", err)
	}
	return nil
}

func (i *SQLiteIndex) UpsertDense(ctx context.Context, id string, vec DenseVector, payload map[string]string) error {
	dimensions := len(vec.Values)
	if err := i.EnsureDense(ctx, dimensions); err != nil {
		return err
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	oldDimension, _ := sqliteDenseDimension(ctx, tx, id)
	if err := sqliteUpsertPoint(ctx, tx, id, payload, dimensions); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO memory_dense_rowids (point_id) VALUES (?)`, id); err != nil {
		return fmt.Errorf("sqlite memory index: allocate dense rowid: %w", err)
	}
	var rowID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM memory_dense_rowids WHERE point_id = ?`, id).Scan(&rowID); err != nil {
		return fmt.Errorf("sqlite memory index: read dense rowid: %w", err)
	}
	if oldDimension > 0 && oldDimension != dimensions {
		if err := sqliteDeleteDenseRow(ctx, tx, oldDimension, rowID); err != nil {
			return err
		}
	}
	if err := sqliteDeleteDenseRow(ctx, tx, dimensions, rowID); err != nil {
		return err
	}
	table, _ := denseVectorTable(dimensions)
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s (rowid, embedding) VALUES (?, ?)`, table), rowID, vectorLiteral(vec.Values)); err != nil {
		return fmt.Errorf("sqlite memory index: insert dense vector: %w", err)
	}
	return tx.Commit()
}

func (i *SQLiteIndex) UpsertSparse(ctx context.Context, id string, vec SparseVector, payload map[string]string) error {
	if err := i.EnsureSparse(ctx); err != nil {
		return err
	}
	if len(vec.Indices) != len(vec.Values) {
		return fmt.Errorf("sqlite memory index: sparse vector length mismatch: %d indices, %d values", len(vec.Indices), len(vec.Values))
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if err := sqliteUpsertPoint(ctx, tx, id, payload, 0); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_sparse_terms WHERE point_id = ?`, id); err != nil {
		return fmt.Errorf("sqlite memory index: delete sparse terms: %w", err)
	}
	for j, dim := range vec.Indices {
		value := vec.Values[j]
		if value == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `
INSERT INTO memory_sparse_terms (point_id, bot_id, dim, value)
VALUES (?, ?, ?, ?)
ON CONFLICT (point_id, dim) DO UPDATE
SET value = excluded.value,
    bot_id = excluded.bot_id
`, id, payloadValue(payload, "bot_id"), int64(dim), value); err != nil {
			return fmt.Errorf("sqlite memory index: insert sparse term: %w", err)
		}
	}
	return tx.Commit()
}

func sqliteUpsertPoint(ctx context.Context, execer sqliteExecer, id string, payload map[string]string, denseDimension int) error {
	data, err := payloadJSON(payload)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(ctx, `
INSERT INTO memory_index_points (
  point_id, bot_id, source_entry_id, memory, hash, payload,
  dense_dimension, created_at, updated_at, indexed_at
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
ON CONFLICT (point_id) DO UPDATE
SET bot_id = excluded.bot_id,
    source_entry_id = excluded.source_entry_id,
    memory = excluded.memory,
    hash = excluded.hash,
    payload = excluded.payload,
    dense_dimension = CASE WHEN excluded.dense_dimension > 0 THEN excluded.dense_dimension ELSE memory_index_points.dense_dimension END,
    created_at = excluded.created_at,
    updated_at = excluded.updated_at,
    indexed_at = CURRENT_TIMESTAMP
`, id, payloadValue(payload, "bot_id"), payloadValue(payload, "source_entry_id"), payloadValue(payload, "memory"), payloadValue(payload, "hash"), string(data), denseDimension, payloadValue(payload, "created_at"), payloadValue(payload, "updated_at"))
	if err != nil {
		return fmt.Errorf("sqlite memory index: upsert point: %w", err)
	}
	return nil
}

func (i *SQLiteIndex) SearchDense(ctx context.Context, vec DenseVector, botID string, limit int) ([]SearchResult, error) {
	dimensions := len(vec.Values)
	if err := i.EnsureDense(ctx, dimensions); err != nil {
		return nil, err
	}
	table, _ := denseVectorTable(dimensions)
	rows, err := i.db.QueryContext(ctx, fmt.Sprintf(`
SELECT p.point_id, v.distance, p.payload
FROM %s v
JOIN memory_dense_rowids r ON r.id = v.rowid
JOIN memory_index_points p ON p.point_id = r.point_id
WHERE v.embedding MATCH ?
  AND v.k = ?
  AND p.bot_id = ?
ORDER BY v.distance
LIMIT ?
`, table), vectorLiteral(vec.Values), normalizeLimit(limit), strings.TrimSpace(botID), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite memory index: search dense: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	results, err := scanSQLiteResults(rows)
	if err != nil {
		return nil, err
	}
	for j := range results {
		results[j].Score = 1 / (1 + results[j].Score)
	}
	return results, nil
}

func (i *SQLiteIndex) SearchSparse(ctx context.Context, vec SparseVector, botID string, limit int) ([]SearchResult, error) {
	if err := i.EnsureSparse(ctx); err != nil {
		return nil, err
	}
	pairs, err := querySparsePairs(vec)
	if err != nil {
		return nil, err
	}
	rows, err := i.db.QueryContext(ctx, `
WITH query_terms AS (
  SELECT CAST(json_extract(value, '$[0]') AS INTEGER) AS dim,
         CAST(json_extract(value, '$[1]') AS REAL) AS value
  FROM json_each(?)
),
scored AS (
  SELECT t.point_id, SUM(t.value * q.value) AS score
  FROM memory_sparse_terms t
  JOIN query_terms q ON q.dim = t.dim
  WHERE t.bot_id = ?
  GROUP BY t.point_id
)
SELECT p.point_id, scored.score, p.payload
FROM scored
JOIN memory_index_points p ON p.point_id = scored.point_id
ORDER BY scored.score DESC, p.updated_at DESC, p.point_id ASC
LIMIT ?
`, string(pairs), strings.TrimSpace(botID), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite memory index: search sparse: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanSQLiteResults(rows)
}

func (i *SQLiteIndex) ScrollDense(ctx context.Context, botID string, limit int) ([]SearchResult, error) {
	if err := i.ensureBase(ctx); err != nil {
		return nil, err
	}
	rows, err := i.db.QueryContext(ctx, `
SELECT p.point_id, 0 AS score, p.payload
FROM memory_index_points p
JOIN memory_dense_rowids r ON r.point_id = p.point_id
WHERE p.bot_id = ?
ORDER BY p.indexed_at ASC, p.point_id ASC
LIMIT ?
`, strings.TrimSpace(botID), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite memory index: scroll dense: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanSQLiteResults(rows)
}

func (i *SQLiteIndex) ScrollSparse(ctx context.Context, botID string, limit int) ([]SearchResult, error) {
	if err := i.ensureBase(ctx); err != nil {
		return nil, err
	}
	rows, err := i.db.QueryContext(ctx, `
SELECT p.point_id, 0 AS score, p.payload
FROM memory_index_points p
WHERE p.bot_id = ?
  AND EXISTS (SELECT 1 FROM memory_sparse_terms s WHERE s.point_id = p.point_id)
ORDER BY p.indexed_at ASC, p.point_id ASC
LIMIT ?
`, strings.TrimSpace(botID), normalizeLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("sqlite memory index: scroll sparse: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	return scanSQLiteResults(rows)
}

func (i *SQLiteIndex) CountDense(ctx context.Context, botID string) (int, error) {
	if err := i.ensureBase(ctx); err != nil {
		return 0, err
	}
	var count int
	err := i.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM memory_index_points p
JOIN memory_dense_rowids r ON r.point_id = p.point_id
WHERE p.bot_id = ?
`, strings.TrimSpace(botID)).Scan(&count)
	return count, err
}

func (i *SQLiteIndex) CountSparse(ctx context.Context, botID string) (int, error) {
	if err := i.ensureBase(ctx); err != nil {
		return 0, err
	}
	var count int
	err := i.db.QueryRowContext(ctx, `
SELECT COUNT(DISTINCT point_id)
FROM memory_sparse_terms
WHERE bot_id = ?
`, strings.TrimSpace(botID)).Scan(&count)
	return count, err
}

func (i *SQLiteIndex) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	if err := i.ensureBase(ctx); err != nil {
		return err
	}
	tx, err := i.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	for _, id := range ids {
		if err := deleteSQLitePoint(ctx, tx, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (i *SQLiteIndex) DeleteByBotID(ctx context.Context, botID string) error {
	if err := i.ensureBase(ctx); err != nil {
		return err
	}
	rows, err := i.db.QueryContext(ctx, `SELECT point_id FROM memory_index_points WHERE bot_id = ?`, strings.TrimSpace(botID))
	if err != nil {
		return fmt.Errorf("sqlite memory index: list bot points: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	pointIDs := []string{}
	for rows.Next() {
		var pointID string
		if err := rows.Scan(&pointID); err != nil {
			return err
		}
		pointIDs = append(pointIDs, pointID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return i.DeleteByIDs(ctx, pointIDs)
}

func deleteSQLitePoint(ctx context.Context, tx *sql.Tx, pointID string) error {
	dimension, rowID := 0, int64(0)
	err := tx.QueryRowContext(ctx, `
SELECT p.dense_dimension, COALESCE(r.id, 0)
FROM memory_index_points p
LEFT JOIN memory_dense_rowids r ON r.point_id = p.point_id
WHERE p.point_id = ?
`, strings.TrimSpace(pointID)).Scan(&dimension, &rowID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("sqlite memory index: read point for delete: %w", err)
	}
	if dimension > 0 && rowID > 0 {
		if err := sqliteDeleteDenseRow(ctx, tx, dimension, rowID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_dense_rowids WHERE point_id = ?`, strings.TrimSpace(pointID)); err != nil {
		return fmt.Errorf("sqlite memory index: delete dense rowid: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_index_points WHERE point_id = ?`, strings.TrimSpace(pointID)); err != nil {
		return fmt.Errorf("sqlite memory index: delete point: %w", err)
	}
	return nil
}

func sqliteDenseDimension(ctx context.Context, execer sqliteExecer, pointID string) (int, error) {
	var dimension int
	err := execer.QueryRowContext(ctx, `SELECT dense_dimension FROM memory_index_points WHERE point_id = ?`, strings.TrimSpace(pointID)).Scan(&dimension)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return dimension, err
}

func sqliteDeleteDenseRow(ctx context.Context, execer sqliteExecer, dimensions int, rowID int64) error {
	table, err := denseVectorTable(dimensions)
	if err != nil {
		return err
	}
	if _, err := execer.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE rowid = ?`, table), rowID); err != nil {
		return fmt.Errorf("sqlite memory index: delete dense vector: %w", err)
	}
	return nil
}

func scanSQLiteResults(rows *sql.Rows) ([]SearchResult, error) {
	results := []SearchResult{}
	for rows.Next() {
		var id string
		var score float64
		var raw string
		if err := rows.Scan(&id, &score, &raw); err != nil {
			return nil, err
		}
		if math.IsNaN(score) || math.IsInf(score, 0) {
			score = 0
		}
		results = append(results, SearchResult{
			ID:      id,
			Score:   score,
			Payload: parsePayload([]byte(raw)),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
