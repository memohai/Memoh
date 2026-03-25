-- name: UpsertLlmProviderOAuthToken :one
INSERT INTO llm_provider_oauth_tokens (
  llm_provider_id,
  access_token,
  refresh_token,
  expires_at,
  scope,
  token_type,
  state,
  pkce_code_verifier
)
VALUES (
  sqlc.arg(llm_provider_id),
  sqlc.arg(access_token),
  sqlc.arg(refresh_token),
  sqlc.arg(expires_at),
  sqlc.arg(scope),
  sqlc.arg(token_type),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier)
)
ON CONFLICT (llm_provider_id) DO UPDATE SET
  access_token = EXCLUDED.access_token,
  refresh_token = EXCLUDED.refresh_token,
  expires_at = EXCLUDED.expires_at,
  scope = EXCLUDED.scope,
  token_type = EXCLUDED.token_type,
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  updated_at = now()
RETURNING *;

-- name: GetLlmProviderOAuthTokenByProvider :one
SELECT * FROM llm_provider_oauth_tokens WHERE llm_provider_id = sqlc.arg(llm_provider_id);

-- name: GetLlmProviderOAuthTokenByState :one
SELECT * FROM llm_provider_oauth_tokens WHERE state = sqlc.arg(state) AND state != '';

-- name: UpdateLlmProviderOAuthState :exec
INSERT INTO llm_provider_oauth_tokens (llm_provider_id, state, pkce_code_verifier)
VALUES (
  sqlc.arg(llm_provider_id),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier)
)
ON CONFLICT (llm_provider_id) DO UPDATE SET
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  updated_at = now();

-- name: DeleteLlmProviderOAuthToken :exec
DELETE FROM llm_provider_oauth_tokens WHERE llm_provider_id = sqlc.arg(llm_provider_id);
