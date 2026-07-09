-- name: UpsertProviderOAuthToken :one
INSERT INTO provider_oauth_tokens (
  team_id,
  provider_id,
  access_token,
  refresh_token,
  expires_at,
  scope,
  token_type,
  state,
  pkce_code_verifier
)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(provider_id),
  sqlc.arg(access_token),
  sqlc.arg(refresh_token),
  sqlc.arg(expires_at),
  sqlc.arg(scope),
  sqlc.arg(token_type),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier)
)
ON CONFLICT (team_id, provider_id) DO UPDATE SET
  access_token = EXCLUDED.access_token,
  refresh_token = EXCLUDED.refresh_token,
  expires_at = EXCLUDED.expires_at,
  scope = EXCLUDED.scope,
  token_type = EXCLUDED.token_type,
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  updated_at = now()
RETURNING *;

-- name: GetProviderOAuthTokenByProvider :one
SELECT * FROM provider_oauth_tokens WHERE provider_id = sqlc.arg(provider_id);

-- name: GetProviderOAuthTokenByProviderForTeam :one
SELECT * FROM provider_oauth_tokens
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id);

-- name: GetProviderOAuthTokenByState :one
SELECT * FROM provider_oauth_tokens WHERE state = sqlc.arg(state) AND state != '';

-- name: GetProviderOAuthTokenByStateForTeam :one
SELECT * FROM provider_oauth_tokens
WHERE team_id = sqlc.arg(team_id)
  AND state = sqlc.arg(state)
  AND state != '';

-- name: UpdateProviderOAuthState :exec
INSERT INTO provider_oauth_tokens (team_id, provider_id, state, pkce_code_verifier)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(provider_id),
  sqlc.arg(state),
  sqlc.arg(pkce_code_verifier)
)
ON CONFLICT (team_id, provider_id) DO UPDATE SET
  state = EXCLUDED.state,
  pkce_code_verifier = EXCLUDED.pkce_code_verifier,
  updated_at = now();

-- name: DeleteProviderOAuthToken :exec
DELETE FROM provider_oauth_tokens WHERE provider_id = sqlc.arg(provider_id);

-- name: DeleteProviderOAuthTokenForTeam :exec
DELETE FROM provider_oauth_tokens
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id);
