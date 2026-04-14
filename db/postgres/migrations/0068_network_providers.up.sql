-- 0068_network_providers
-- Add bot-scoped SD-WAN network configuration fields.
-- Each bot gets its own isolated sidecar instance so a single network_config
-- JSONB column is sufficient — no provider/binding split needed.

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS network_provider TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS network_enabled  BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS network_config   JSONB NOT NULL DEFAULT '{}'::jsonb;
