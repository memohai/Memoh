-- 0002_container_workspace_backend
-- SQLite cannot drop columns portably; rebuild containers without workspace_backend.

PRAGMA foreign_keys = OFF;

CREATE TABLE containers_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  container_id TEXT NOT NULL,
  container_name TEXT NOT NULL,
  image TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'created',
  namespace TEXT NOT NULL DEFAULT 'default',
  auto_start INTEGER NOT NULL DEFAULT 1,
  container_path TEXT NOT NULL DEFAULT '/data',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_started_at TEXT,
  last_stopped_at TEXT,
  CONSTRAINT containers_container_id_unique UNIQUE (container_id),
  CONSTRAINT containers_container_name_unique UNIQUE (container_name)
);

INSERT INTO containers_new (
  id, bot_id, container_id, container_name, image, status, namespace, auto_start,
  container_path, created_at, updated_at, last_started_at, last_stopped_at
)
SELECT
  id, bot_id, container_id, container_name, image, status, namespace, auto_start,
  container_path, created_at, updated_at, last_started_at, last_stopped_at
FROM containers;

DROP TABLE containers;
ALTER TABLE containers_new RENAME TO containers;
CREATE INDEX IF NOT EXISTS idx_containers_bot_id ON containers(bot_id);

PRAGMA foreign_keys = ON;
