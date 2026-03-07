-- 0027_bot_tts_provider
-- Add tts_provider_id FK to bots table for per-bot TTS configuration

ALTER TABLE bots
  ADD COLUMN IF NOT EXISTS tts_provider_id UUID REFERENCES tts_providers(id) ON DELETE SET NULL;
