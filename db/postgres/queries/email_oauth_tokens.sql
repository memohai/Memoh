-- name: UpsertEmailOAuthToken :one
INSERT INTO email_oauth_tokens (email_provider_id, email_address, access_token, refresh_token, expires_at, scope, state)
SELECT ep.id, sqlc.arg(email_address), sqlc.arg(access_token), sqlc.arg(refresh_token), sqlc.arg(expires_at), sqlc.arg(scope), sqlc.arg(state)
FROM email_providers ep
WHERE ep.id = sqlc.arg(email_provider_id)
  AND ep.team_id = sqlc.arg(team_id)
ON CONFLICT (email_provider_id) DO UPDATE SET
  email_address  = EXCLUDED.email_address,
  access_token   = EXCLUDED.access_token,
  refresh_token  = EXCLUDED.refresh_token,
  expires_at     = EXCLUDED.expires_at,
  scope          = EXCLUDED.scope,
  state          = EXCLUDED.state,
  updated_at     = now()
RETURNING *;

-- name: GetEmailOAuthTokenByProvider :one
SELECT tok.*
FROM email_oauth_tokens tok
JOIN email_providers ep ON ep.id = tok.email_provider_id
WHERE tok.email_provider_id = sqlc.arg(email_provider_id)
  AND ep.team_id = sqlc.arg(team_id);

-- name: GetEmailOAuthTokenByState :one
SELECT tok.*
FROM email_oauth_tokens tok
JOIN email_providers ep ON ep.id = tok.email_provider_id
WHERE tok.state = sqlc.arg(state)
  AND tok.state != ''
  AND ep.team_id = sqlc.arg(team_id);

-- name: UpdateEmailOAuthState :exec
INSERT INTO email_oauth_tokens (email_provider_id, state)
SELECT ep.id, sqlc.arg(state)
FROM email_providers ep
WHERE ep.id = sqlc.arg(email_provider_id)
  AND ep.team_id = sqlc.arg(team_id)
ON CONFLICT (email_provider_id) DO UPDATE SET
  state      = EXCLUDED.state,
  updated_at = now();

-- name: DeleteEmailOAuthToken :exec
DELETE FROM email_oauth_tokens tok
USING email_providers ep
WHERE tok.email_provider_id = sqlc.arg(email_provider_id)
  AND ep.id = tok.email_provider_id
  AND ep.team_id = sqlc.arg(team_id);
