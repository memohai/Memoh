-- 0041_provider_model_refactor
-- Move client_type to llm_providers, add icon, replace model columns with config JSONB.
-- NOTE: This migration is a no-op on fresh databases where the canonical schema already applies.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'llm_providers') THEN
    RETURN;
  END IF;

  ALTER TABLE llm_providers
    ADD COLUMN IF NOT EXISTS client_type TEXT NOT NULL DEFAULT 'openai-completions',
    ADD COLUMN IF NOT EXISTS icon TEXT;

  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'models' AND column_name = 'client_type') THEN
    UPDATE llm_providers p
    SET client_type = sub.client_type
    FROM (
      SELECT DISTINCT ON (llm_provider_id) llm_provider_id, client_type
      FROM models
      WHERE client_type IS NOT NULL AND client_type != ''
      ORDER BY llm_provider_id, created_at ASC
    ) sub
    WHERE p.id = sub.llm_provider_id;
  END IF;

  IF NOT EXISTS (SELECT 1 FROM information_schema.table_constraints WHERE constraint_name = 'llm_providers_client_type_check') THEN
    ALTER TABLE llm_providers
      ADD CONSTRAINT llm_providers_client_type_check
      CHECK (client_type IN ('openai-responses', 'openai-completions', 'anthropic-messages', 'google-generative-ai'));
  END IF;

  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'models' AND column_name = 'config') THEN
    ALTER TABLE models ADD COLUMN config JSONB NOT NULL DEFAULT '{}'::jsonb;
  END IF;

  IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'models' AND column_name = 'dimensions') THEN
    UPDATE models SET config = jsonb_strip_nulls(jsonb_build_object(
      'dimensions', dimensions,
      'compatibilities', (
        SELECT coalesce(jsonb_agg(c), '[]'::jsonb) FROM (
          SELECT 'tool-call' AS c
          UNION ALL
          SELECT 'vision' WHERE 'image' = ANY(input_modalities)
          UNION ALL
          SELECT 'reasoning' WHERE supports_reasoning = true
        ) AS caps
      ),
      'context_window', NULL
    ));
  END IF;
END $$;

ALTER TABLE models DROP CONSTRAINT IF EXISTS models_client_type_check;
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_chat_client_type_check;
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_dimensions_check;
ALTER TABLE models DROP COLUMN IF EXISTS client_type;
ALTER TABLE models DROP COLUMN IF EXISTS dimensions;
ALTER TABLE models DROP COLUMN IF EXISTS input_modalities;
ALTER TABLE models DROP COLUMN IF EXISTS supports_reasoning;
