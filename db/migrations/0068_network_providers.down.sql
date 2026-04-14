-- 0068_network_providers
-- Remove bot-scoped network configuration fields.

ALTER TABLE bots
  DROP COLUMN IF EXISTS network_config,
  DROP COLUMN IF EXISTS network_enabled,
  DROP COLUMN IF EXISTS network_provider;
