-- 0008_snapshot_schema_and_drop_media_assets
-- Part 1: Rebuild snapshot and version schema.
-- Part 2: bot_history_message_assets soft link only (message_id, content_hash, role, ordinal); drop media_assets.
-- MIME/size/storage_key are derived from storage at read time.

-- Part 1: Snapshot schema
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

-- Part 2: Soft link only (content_hash); drop asset_id and media_assets
ALTER TABLE bot_history_message_assets
  ADD COLUMN IF NOT EXISTS content_hash TEXT;

UPDATE bot_history_message_assets ma SET
  content_hash = a.content_hash
FROM media_assets a WHERE a.id = ma.asset_id;

ALTER TABLE bot_history_message_assets
  DROP CONSTRAINT IF EXISTS bot_history_message_assets_asset_id_fkey;
ALTER TABLE bot_history_message_assets
  DROP CONSTRAINT IF EXISTS message_asset_unique;

ALTER TABLE bot_history_message_assets
  DROP COLUMN IF EXISTS asset_id;

ALTER TABLE bot_history_message_assets
  DROP COLUMN IF EXISTS original_name,
  DROP COLUMN IF EXISTS width,
  DROP COLUMN IF EXISTS height,
  DROP COLUMN IF EXISTS duration_ms,
  DROP COLUMN IF EXISTS media_type,
  DROP COLUMN IF EXISTS mime,
  DROP COLUMN IF EXISTS size_bytes,
  DROP COLUMN IF EXISTS storage_key;

ALTER TABLE bot_history_message_assets
  ADD CONSTRAINT message_asset_content_unique UNIQUE (message_id, content_hash);

DROP INDEX IF EXISTS idx_message_assets_asset_id;
DROP TABLE IF EXISTS media_assets;
