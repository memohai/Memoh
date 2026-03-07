-- 0026_tts_providers
-- Add tts_providers table for pluggable TTS service backends

CREATE TABLE IF NOT EXISTS tts_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT tts_providers_name_unique UNIQUE (name)
);
