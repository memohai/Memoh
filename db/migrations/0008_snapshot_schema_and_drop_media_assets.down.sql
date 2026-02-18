-- 0008_snapshot_schema_and_drop_media_assets (rollback)
-- Part 1: Restore media_assets and asset_id on bot_history_message_assets.
-- Part 2: Restore previous snapshot/container_versions schema.

-- Part 1: Restore media_assets and bot_history_message_assets legacy columns
CREATE TABLE IF NOT EXISTS media_assets (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  storage_provider_id UUID REFERENCES storage_providers(id) ON DELETE SET NULL,
  content_hash TEXT NOT NULL,
  media_type TEXT NOT NULL,
  mime TEXT NOT NULL DEFAULT 'application/octet-stream',
  size_bytes BIGINT NOT NULL DEFAULT 0,
  storage_key TEXT NOT NULL,
  original_name TEXT,
  width INTEGER,
  height INTEGER,
  duration_ms BIGINT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT media_assets_bot_hash_unique UNIQUE (bot_id, content_hash)
);

CREATE INDEX IF NOT EXISTS idx_media_assets_bot_id ON media_assets(bot_id);
CREATE INDEX IF NOT EXISTS idx_media_assets_content_hash ON media_assets(content_hash);

ALTER TABLE bot_history_message_assets
  ADD COLUMN IF NOT EXISTS asset_id UUID;
ALTER TABLE bot_history_message_assets
  ADD COLUMN IF NOT EXISTS original_name TEXT,
  ADD COLUMN IF NOT EXISTS width INTEGER,
  ADD COLUMN IF NOT EXISTS height INTEGER,
  ADD COLUMN IF NOT EXISTS duration_ms BIGINT;

ALTER TABLE bot_history_message_assets
  DROP CONSTRAINT IF EXISTS message_asset_content_unique;
ALTER TABLE bot_history_message_assets
  ADD CONSTRAINT message_asset_unique UNIQUE (message_id, asset_id);
ALTER TABLE bot_history_message_assets
  ADD CONSTRAINT bot_history_message_assets_asset_id_fkey
  FOREIGN KEY (asset_id) REFERENCES media_assets(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_message_assets_asset_id ON bot_history_message_assets(asset_id);

ALTER TABLE bot_history_message_assets
  DROP COLUMN IF EXISTS content_hash,
  DROP COLUMN IF EXISTS mime,
  DROP COLUMN IF EXISTS size_bytes,
  DROP COLUMN IF EXISTS storage_key;

-- Part 2: Restore previous snapshot/container_versions schema
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
