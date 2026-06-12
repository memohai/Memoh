-- 0018_fetch_providers
-- Add configurable web fetch providers and bot-level fetch provider selection.

CREATE TABLE IF NOT EXISTS fetch_providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  enable INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT fetch_providers_name_unique UNIQUE (name)
);

ALTER TABLE bots
  ADD COLUMN fetch_provider_id TEXT REFERENCES fetch_providers(id) ON DELETE SET NULL;
