-- 0050_tts_provider_enable
-- Add enable column to tts_providers table for toggling providers on/off.
-- NOTE: No-op on fresh databases where tts_providers no longer exists.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tts_providers') THEN
    ALTER TABLE tts_providers ADD COLUMN IF NOT EXISTS enable BOOLEAN NOT NULL DEFAULT false;
  END IF;
END $$;
