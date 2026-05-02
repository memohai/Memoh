-- 0003_memory_sql_index
-- Add database-backed memory vector indexes to replace the external vector service.

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
