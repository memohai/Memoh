-- 0042_provider_enable
-- Add enable column to llm_providers for built-in provider registry support.
-- NOTE: This migration is a no-op on fresh databases where the canonical schema already applies.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'llm_providers') THEN
    ALTER TABLE llm_providers ADD COLUMN IF NOT EXISTS enable BOOLEAN NOT NULL DEFAULT true;
  END IF;
END $$;
