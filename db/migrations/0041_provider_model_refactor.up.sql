-- 0041_provider_model_refactor
-- Move client_type to llm_providers, add icon, replace model columns with config JSONB.

-- 1. Add client_type and icon to llm_providers
ALTER TABLE llm_providers
  ADD COLUMN IF NOT EXISTS client_type TEXT NOT NULL DEFAULT 'openai-completions',
  ADD COLUMN IF NOT EXISTS icon TEXT;

-- 2. Back-fill provider client_type from existing models
UPDATE llm_providers p
SET client_type = sub.client_type
FROM (
  SELECT DISTINCT ON (llm_provider_id) llm_provider_id, client_type
  FROM models
  WHERE client_type IS NOT NULL AND client_type != ''
  ORDER BY llm_provider_id, created_at ASC
) sub
WHERE p.id = sub.llm_provider_id;

-- 3. Add CHECK constraint on provider client_type
ALTER TABLE llm_providers
  ADD CONSTRAINT llm_providers_client_type_check
  CHECK (client_type IN ('openai-responses', 'openai-completions', 'anthropic-messages', 'google-generative-ai'));

-- 4. Add config JSONB to models
ALTER TABLE models
  ADD COLUMN IF NOT EXISTS config JSONB NOT NULL DEFAULT '{}'::jsonb;

-- 5. Migrate existing columns into config
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

-- 6. Drop old columns and constraints from models
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_client_type_check;
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_chat_client_type_check;
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_dimensions_check;

ALTER TABLE models DROP COLUMN IF EXISTS client_type;
ALTER TABLE models DROP COLUMN IF EXISTS dimensions;
ALTER TABLE models DROP COLUMN IF EXISTS input_modalities;
ALTER TABLE models DROP COLUMN IF EXISTS supports_reasoning;
