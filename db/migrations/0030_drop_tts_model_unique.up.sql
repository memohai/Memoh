-- 0030_drop_tts_model_unique
-- Drop unique constraint on (tts_provider_id, model_id) to allow multiple
-- models with the same model_id under one provider (different configs).
-- NOTE: No-op on fresh databases where tts_models no longer exists.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'tts_models') THEN
    ALTER TABLE tts_models DROP CONSTRAINT IF EXISTS tts_models_provider_model_id_unique;
  END IF;
END $$;
