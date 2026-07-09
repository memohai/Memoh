-- name: GetMCPOAuthToken :one
SELECT *
FROM mcp_oauth_tokens t
WHERE connection_id = sqlc.arg(connection_id)
  AND team_id = (SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id))
LIMIT 1;

-- name: GetMCPOAuthTokenByState :one
SELECT t.*
FROM mcp_oauth_tokens t
JOIN mcp_connections c
  ON c.id = t.connection_id
 AND c.team_id = t.team_id
WHERE t.state_param = sqlc.arg(state_param)
  AND t.state_param <> ''
LIMIT 1;

-- name: UpsertMCPOAuthDiscovery :one
INSERT INTO mcp_oauth_tokens (team_id, connection_id, resource_metadata_url, authorization_server_url,
    authorization_endpoint, token_endpoint, registration_endpoint, scopes_supported,
    resource_uri)
SELECT c.team_id, c.id, sqlc.arg(resource_metadata_url), sqlc.arg(authorization_server_url), sqlc.arg(authorization_endpoint), sqlc.arg(token_endpoint), sqlc.arg(registration_endpoint), sqlc.arg(scopes_supported), sqlc.arg(resource_uri)
FROM mcp_connections c
WHERE c.id = sqlc.arg(connection_id)
ON CONFLICT (team_id, connection_id)
DO UPDATE SET resource_metadata_url = EXCLUDED.resource_metadata_url,
              authorization_server_url = EXCLUDED.authorization_server_url,
              authorization_endpoint = EXCLUDED.authorization_endpoint,
              token_endpoint = EXCLUDED.token_endpoint,
              registration_endpoint = EXCLUDED.registration_endpoint,
              scopes_supported = EXCLUDED.scopes_supported,
              resource_uri = EXCLUDED.resource_uri,
              updated_at = now()
RETURNING *;

-- name: UpdateMCPOAuthPKCEState :exec
UPDATE mcp_oauth_tokens
SET pkce_code_verifier = sqlc.arg(pkce_code_verifier),
    state_param = sqlc.arg(state_param),
    client_id = sqlc.arg(client_id),
    redirect_uri = sqlc.arg(redirect_uri),
    updated_at = now()
WHERE connection_id = sqlc.arg(connection_id)
  AND team_id = (SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id));

-- name: UpdateMCPOAuthTokens :exec
UPDATE mcp_oauth_tokens
SET access_token = sqlc.arg(access_token),
    refresh_token = sqlc.arg(refresh_token),
    token_type = sqlc.arg(token_type),
    expires_at = sqlc.arg(expires_at),
    scope = sqlc.arg(scope),
    pkce_code_verifier = '',
    state_param = '',
    updated_at = now()
WHERE connection_id = sqlc.arg(connection_id)
  AND team_id = (SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id));

-- name: ClearMCPOAuthTokens :exec
UPDATE mcp_oauth_tokens
SET access_token = '',
    refresh_token = '',
    expires_at = NULL,
    scope = '',
    pkce_code_verifier = '',
    state_param = '',
    redirect_uri = '',
    updated_at = now()
WHERE connection_id = sqlc.arg(connection_id)
  AND team_id = (SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id));

-- name: UpdateMCPOAuthClientSecret :exec
UPDATE mcp_oauth_tokens
SET client_secret = sqlc.arg(client_secret),
    updated_at = now()
WHERE connection_id = sqlc.arg(connection_id)
  AND team_id = (SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id));

-- name: DeleteMCPOAuthToken :exec
DELETE FROM mcp_oauth_tokens
WHERE connection_id = sqlc.arg(connection_id)
  AND team_id = (SELECT c.team_id FROM mcp_connections c WHERE c.id = sqlc.arg(connection_id));
