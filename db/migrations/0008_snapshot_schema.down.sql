-- 0008_snapshot_schema
-- Roll back snapshot and version schema to previous shape.

DROP TABLE IF EXISTS container_versions;
DROP TABLE IF EXISTS snapshots;

CREATE TABLE IF NOT EXISTS snapshots (
  id TEXT PRIMARY KEY,
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  parent_snapshot_id TEXT REFERENCES snapshots(id) ON DELETE SET NULL,
  snapshotter TEXT NOT NULL,
  digest TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_snapshots_container_id ON snapshots(container_id);
CREATE INDEX IF NOT EXISTS idx_snapshots_parent_id ON snapshots(parent_snapshot_id);

CREATE TABLE IF NOT EXISTS container_versions (
  id TEXT PRIMARY KEY,
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  snapshot_id TEXT NOT NULL REFERENCES snapshots(id) ON DELETE RESTRICT,
  version INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (container_id, version)
);

CREATE INDEX IF NOT EXISTS idx_container_versions_container_id ON container_versions(container_id);
