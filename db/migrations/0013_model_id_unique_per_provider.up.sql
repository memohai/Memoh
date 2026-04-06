-- 0013_model_id_unique_per_provider
-- Change model_id uniqueness from global to per provider.
-- NOTE: On fresh databases the canonical schema already has the correct constraint.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_model_id_unique') THEN
    ALTER TABLE models DROP CONSTRAINT models_model_id_unique;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_provider_model_id_unique')
     AND NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_provider_id_model_id_unique') THEN
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'models' AND column_name = 'llm_provider_id') THEN
      ALTER TABLE models
        ADD CONSTRAINT models_provider_model_id_unique UNIQUE (llm_provider_id, model_id);
    END IF;
  END IF;
END
$$;
