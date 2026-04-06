-- 0046_llm_provider_oauth
-- Add OAuth token storage for LLM providers to support OpenAI Codex OAuth.

CREATE TABLE IF NOT EXISTS llm_provider_oauth_tokens (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  llm_provider_id UUID NOT NULL UNIQUE REFERENCES llm_providers(id) ON DELETE CASCADE,
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ,
  scope TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_llm_provider_oauth_tokens_state ON llm_provider_oauth_tokens(state) WHERE state != '';
