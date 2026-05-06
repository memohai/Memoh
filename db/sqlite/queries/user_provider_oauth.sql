-- name: UpsertUserProviderOAuthToken :one
INSERT INTO iam_user_provider_oauth_tokens (id, provider_id, user_id, access_token, refresh_token, expires_at, scope, token_type, state, pkce_code_verifier, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(provider_id),
  sqlc.arg(user_id),
  sqlc.arg(access_token),
  sqlc.arg(refresh_token),
  sqlc.arg(expires_at),
  sqlc.arg(scope),
  sqlc.arg(token_type),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier),
  sqlc.arg(metadata)
)
ON CONFLICT (provider_id, user_id) DO UPDATE SET
  access_token = EXCLUDED.access_token,
  refresh_token = EXCLUDED.refresh_token,
  expires_at = EXCLUDED.expires_at,
  scope = EXCLUDED.scope,
  token_type = EXCLUDED.token_type,
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  metadata = EXCLUDED.metadata,
  updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetUserProviderOAuthToken :one
SELECT * FROM iam_user_provider_oauth_tokens
WHERE provider_id = sqlc.arg(provider_id)
  AND user_id = sqlc.arg(user_id);

-- name: GetUserProviderOAuthTokenByState :one
SELECT * FROM iam_user_provider_oauth_tokens
WHERE state = sqlc.arg(state)
  AND state != '';

-- name: UpdateUserProviderOAuthState :exec
INSERT INTO iam_user_provider_oauth_tokens (id, provider_id, user_id, state, pkce_code_verifier, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(provider_id),
  sqlc.arg(user_id),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier),
  sqlc.arg(metadata)
)
ON CONFLICT (provider_id, user_id) DO UPDATE SET
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  metadata = EXCLUDED.metadata,
  updated_at = CURRENT_TIMESTAMP;

-- name: DeleteUserProviderOAuthToken :exec
DELETE FROM iam_user_provider_oauth_tokens
WHERE provider_id = sqlc.arg(provider_id)
  AND user_id = sqlc.arg(user_id);
