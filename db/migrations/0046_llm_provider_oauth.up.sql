-- 0046_llm_provider_oauth
-- Add OAuth token storage for providers to support OpenAI Codex OAuth.
-- NOTE: On fresh databases, table is named provider_oauth_tokens via 0061.

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'llm_providers') THEN
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
  ELSIF NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'provider_oauth_tokens') THEN
    CREATE TABLE IF NOT EXISTS provider_oauth_tokens (
      id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      provider_id UUID NOT NULL UNIQUE REFERENCES providers(id) ON DELETE CASCADE,
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
    CREATE INDEX IF NOT EXISTS idx_provider_oauth_tokens_state ON provider_oauth_tokens(state) WHERE state != '';
  END IF;
END $$;
