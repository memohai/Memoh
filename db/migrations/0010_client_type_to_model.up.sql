-- 0010_client_type_to_model
-- Move client_type from llm_providers to models, rename to new values, drop unsupported types.
-- NOTE: This migration is a no-op on fresh databases where the canonical schema already applies.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'llm_providers') THEN
    RETURN;
  END IF;

  -- 1) Add client_type column to models (nullable)
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'models' AND column_name = 'client_type') THEN
    ALTER TABLE models ADD COLUMN client_type TEXT;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'llm_providers' AND column_name = 'client_type'
  ) THEN
    UPDATE models SET client_type = CASE p.client_type
        WHEN 'openai' THEN 'openai-responses'
        WHEN 'openai-compat' THEN 'openai-completions'
        WHEN 'anthropic' THEN 'anthropic-messages'
        WHEN 'google' THEN 'google-generative-ai'
    END
    FROM llm_providers p
    WHERE models.llm_provider_id = p.id
      AND p.client_type IN ('openai', 'openai-compat', 'anthropic', 'google');

    DELETE FROM models WHERE client_type IS NULL AND type = 'chat';
    DELETE FROM llm_providers WHERE client_type NOT IN ('openai', 'openai-compat', 'anthropic', 'google');
  END IF;

  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_client_type_check') THEN
    ALTER TABLE models ADD CONSTRAINT models_client_type_check
      CHECK (client_type IS NULL OR client_type IN ('openai-responses', 'openai-completions', 'anthropic-messages', 'google-generative-ai'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'models_chat_client_type_check') THEN
    ALTER TABLE models ADD CONSTRAINT models_chat_client_type_check
      CHECK (type != 'chat' OR client_type IS NOT NULL);
  END IF;

  ALTER TABLE llm_providers DROP CONSTRAINT IF EXISTS llm_providers_client_type_check;
  ALTER TABLE llm_providers DROP COLUMN IF EXISTS client_type;
END $$;
