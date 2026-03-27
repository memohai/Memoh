-- 0047_add_openai_codex_client_type
-- Add openai-codex as a first-class client_type and migrate existing codex-oauth providers.

ALTER TABLE llm_providers DROP CONSTRAINT IF EXISTS llm_providers_client_type_check;
ALTER TABLE llm_providers ADD CONSTRAINT llm_providers_client_type_check
  CHECK (client_type IN ('openai-responses', 'openai-completions', 'anthropic-messages', 'google-generative-ai', 'openai-codex'));

UPDATE llm_providers
SET client_type = 'openai-codex',
    updated_at  = now()
WHERE client_type = 'openai-responses'
  AND metadata->>'auth_type' = 'openai-codex-oauth';
