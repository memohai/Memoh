-- 0008_snapshot_schema
-- Rebuild snapshot and version schema for full-chain consistency.

DROP TABLE IF EXISTS container_versions;
DROP TABLE IF EXISTS snapshots;

CREATE TABLE IF NOT EXISTS snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  runtime_snapshot_name TEXT NOT NULL,
  parent_runtime_snapshot_name TEXT,
  snapshotter TEXT NOT NULL,
  source TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_container_runtime_name
  ON snapshots(container_id, runtime_snapshot_name);
CREATE INDEX IF NOT EXISTS idx_snapshots_container_created_at
  ON snapshots(container_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_snapshots_runtime_name
  ON snapshots(runtime_snapshot_name);

CREATE TABLE IF NOT EXISTS container_versions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  container_id TEXT NOT NULL REFERENCES containers(container_id) ON DELETE CASCADE,
  snapshot_id UUID NOT NULL REFERENCES snapshots(id) ON DELETE RESTRICT,
  version INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (container_id, version)
);

CREATE INDEX IF NOT EXISTS idx_container_versions_container_id ON container_versions(container_id);
CREATE INDEX IF NOT EXISTS idx_container_versions_snapshot_id ON container_versions(snapshot_id);
