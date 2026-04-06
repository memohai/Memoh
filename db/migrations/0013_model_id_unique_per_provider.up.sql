-- 0013_model_id_unique_per_provider
-- Change model_id uniqueness from global to per provider.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_model_id_unique') THEN
    ALTER TABLE models DROP CONSTRAINT models_model_id_unique;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_provider_model_id_unique') THEN
    ALTER TABLE models
      ADD CONSTRAINT models_provider_model_id_unique UNIQUE (llm_provider_id, model_id);
  END IF;
END
$$;
