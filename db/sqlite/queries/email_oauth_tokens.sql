-- name: UpsertEmailOAuthToken :one
INSERT INTO email_oauth_tokens (id, email_provider_id, email_address, access_token, refresh_token, expires_at, scope, state)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(email_provider_id),
  sqlc.arg(email_address),
  sqlc.arg(access_token),
  sqlc.arg(refresh_token),
  sqlc.arg(expires_at),
  sqlc.arg(scope),
  sqlc.arg(state)
)
ON CONFLICT (email_provider_id) DO UPDATE SET
  email_address  = EXCLUDED.email_address,
  access_token   = EXCLUDED.access_token,
  refresh_token  = EXCLUDED.refresh_token,
  expires_at     = EXCLUDED.expires_at,
  scope          = EXCLUDED.scope,
  state          = EXCLUDED.state,
  updated_at     = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetEmailOAuthTokenByProvider :one
SELECT * FROM email_oauth_tokens WHERE email_provider_id = sqlc.arg(email_provider_id);

-- name: GetEmailOAuthTokenByState :one
SELECT * FROM email_oauth_tokens WHERE state = sqlc.arg(state) AND state != '';

-- name: UpdateEmailOAuthState :exec
INSERT INTO email_oauth_tokens (id, email_provider_id, state)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(email_provider_id),
  sqlc.arg(state)
)
ON CONFLICT (email_provider_id) DO UPDATE SET
  state      = EXCLUDED.state,
  updated_at = CURRENT_TIMESTAMP;

-- name: DeleteEmailOAuthToken :exec
DELETE FROM email_oauth_tokens WHERE email_provider_id = sqlc.arg(email_provider_id);
