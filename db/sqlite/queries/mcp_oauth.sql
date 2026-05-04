-- name: GetMCPOAuthToken :one
SELECT id, connection_id, resource_metadata_url, authorization_server_url,
       authorization_endpoint, token_endpoint, registration_endpoint,
       scopes_supported, client_id, client_secret, access_token, refresh_token,
       token_type, expires_at, scope, pkce_code_verifier, state_param,
       resource_uri, redirect_uri, created_at, updated_at
FROM mcp_oauth_tokens
WHERE connection_id = sqlc.arg(connection_id)
LIMIT 1;

-- name: GetMCPOAuthTokenByState :one
SELECT id, connection_id, resource_metadata_url, authorization_server_url,
       authorization_endpoint, token_endpoint, registration_endpoint,
       scopes_supported, client_id, client_secret, access_token, refresh_token,
       token_type, expires_at, scope, pkce_code_verifier, state_param,
       resource_uri, redirect_uri, created_at, updated_at
FROM mcp_oauth_tokens
WHERE state_param = sqlc.arg(state_param)
LIMIT 1;

-- name: UpsertMCPOAuthDiscovery :one
INSERT INTO mcp_oauth_tokens (id, connection_id, resource_metadata_url, authorization_server_url,
    authorization_endpoint, token_endpoint, registration_endpoint, scopes_supported,
    resource_uri)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(connection_id),
  sqlc.arg(resource_metadata_url),
  sqlc.arg(authorization_server_url),
  sqlc.arg(authorization_endpoint),
  sqlc.arg(token_endpoint),
  sqlc.arg(registration_endpoint),
  sqlc.arg(scopes_supported),
  sqlc.arg(resource_uri)
)
ON CONFLICT (connection_id)
DO UPDATE SET resource_metadata_url = EXCLUDED.resource_metadata_url,
              authorization_server_url = EXCLUDED.authorization_server_url,
              authorization_endpoint = EXCLUDED.authorization_endpoint,
              token_endpoint = EXCLUDED.token_endpoint,
              registration_endpoint = EXCLUDED.registration_endpoint,
              scopes_supported = EXCLUDED.scopes_supported,
              resource_uri = EXCLUDED.resource_uri,
              updated_at = CURRENT_TIMESTAMP
RETURNING id, connection_id, resource_metadata_url, authorization_server_url,
          authorization_endpoint, token_endpoint, registration_endpoint,
          scopes_supported, client_id, client_secret, access_token, refresh_token,
          token_type, expires_at, scope, pkce_code_verifier, state_param,
          resource_uri, redirect_uri, created_at, updated_at;

-- name: UpdateMCPOAuthPKCEState :exec
UPDATE mcp_oauth_tokens
SET pkce_code_verifier = sqlc.arg(pkce_code_verifier),
    state_param = sqlc.arg(state_param),
    client_id = sqlc.arg(client_id),
    redirect_uri = sqlc.arg(redirect_uri),
    updated_at = CURRENT_TIMESTAMP
WHERE connection_id = sqlc.arg(connection_id);

-- name: UpdateMCPOAuthTokens :exec
UPDATE mcp_oauth_tokens
SET access_token = sqlc.arg(access_token),
    refresh_token = sqlc.arg(refresh_token),
    token_type = sqlc.arg(token_type),
    expires_at = sqlc.arg(expires_at),
    scope = sqlc.arg(scope),
    pkce_code_verifier = '',
    state_param = '',
    updated_at = CURRENT_TIMESTAMP
WHERE connection_id = sqlc.arg(connection_id);

-- name: ClearMCPOAuthTokens :exec
UPDATE mcp_oauth_tokens
SET access_token = '',
    refresh_token = '',
    expires_at = NULL,
    scope = '',
    pkce_code_verifier = '',
    state_param = '',
    redirect_uri = '',
    updated_at = CURRENT_TIMESTAMP
WHERE connection_id = sqlc.arg(connection_id);

-- name: UpdateMCPOAuthClientSecret :exec
UPDATE mcp_oauth_tokens
SET client_secret = sqlc.arg(client_secret),
    updated_at = CURRENT_TIMESTAMP
WHERE connection_id = sqlc.arg(connection_id);

-- name: DeleteMCPOAuthToken :exec
DELETE FROM mcp_oauth_tokens
WHERE connection_id = sqlc.arg(connection_id);
