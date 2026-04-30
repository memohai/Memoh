-- name: UpsertProviderOAuthToken :one
INSERT INTO provider_oauth_tokens (id, provider_id, access_token, refresh_token, expires_at, scope, token_type, state, pkce_code_verifier)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(provider_id),
  sqlc.arg(access_token),
  sqlc.arg(refresh_token),
  sqlc.arg(expires_at),
  sqlc.arg(scope),
  sqlc.arg(token_type),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier)
)
ON CONFLICT (provider_id) DO UPDATE SET
  access_token = EXCLUDED.access_token,
  refresh_token = EXCLUDED.refresh_token,
  expires_at = EXCLUDED.expires_at,
  scope = EXCLUDED.scope,
  token_type = EXCLUDED.token_type,
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetProviderOAuthTokenByProvider :one
SELECT * FROM provider_oauth_tokens WHERE provider_id = sqlc.arg(provider_id);

-- name: GetProviderOAuthTokenByState :one
SELECT * FROM provider_oauth_tokens WHERE state = sqlc.arg(state) AND state != '';

-- name: UpdateProviderOAuthState :exec
INSERT INTO provider_oauth_tokens (id, provider_id, state, pkce_code_verifier)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(provider_id),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier)
)
ON CONFLICT (provider_id) DO UPDATE SET
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  updated_at = CURRENT_TIMESTAMP;

-- name: DeleteProviderOAuthToken :exec
DELETE FROM provider_oauth_tokens WHERE provider_id = sqlc.arg(provider_id);
