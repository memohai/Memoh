-- 0019_memory_providers
-- Add memory_providers table, migrate bot memory/embedding model into provider config,
-- and drop the now-redundant columns from bots.

CREATE TABLE IF NOT EXISTS memory_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT memory_providers_name_unique UNIQUE (name)
);

ALTER TABLE bots ADD COLUMN IF NOT EXISTS memory_provider_id UUID REFERENCES memory_providers(id) ON DELETE SET NULL;

-- Migrate: create a default builtin provider with existing model IDs, then link bots to it.
DO $$
DECLARE
  _provider_id UUID;
BEGIN
  -- Only migrate if any bot has memory_model_id or embedding_model_id set.
  IF EXISTS (SELECT 1 FROM bots WHERE memory_model_id IS NOT NULL OR embedding_model_id IS NOT NULL) THEN
    INSERT INTO memory_providers (name, provider, config, is_default)
    VALUES ('Built-in Memory', 'builtin', '{}'::jsonb, true)
    ON CONFLICT (name) DO UPDATE SET updated_at = now()
    RETURNING id INTO _provider_id;

    UPDATE bots
    SET memory_provider_id = _provider_id
    WHERE memory_model_id IS NOT NULL OR embedding_model_id IS NOT NULL;
  END IF;
END $$;

-- Drop the old columns.
ALTER TABLE bots DROP COLUMN IF EXISTS memory_model_id;
ALTER TABLE bots DROP COLUMN IF EXISTS embedding_model_id;
