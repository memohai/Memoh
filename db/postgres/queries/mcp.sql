-- name: GetMCPConnectionByID :one
SELECT id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type,
       managed_by_plugin_installation_id, managed_resource_key, visible, metadata, created_at, updated_at
FROM mcp_connections
WHERE bot_id = $1 AND id = $2
LIMIT 1;

-- name: ListMCPConnectionsByBotID :many
SELECT id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type,
       managed_by_plugin_installation_id, managed_resource_key, visible, metadata, created_at, updated_at
FROM mcp_connections
WHERE bot_id = $1
ORDER BY created_at DESC;

-- name: CreateMCPConnection :one
INSERT INTO mcp_connections (bot_id, name, type, config, is_active, auth_type)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type,
          managed_by_plugin_installation_id, managed_resource_key, visible, metadata, created_at, updated_at;

-- name: CreateManagedMCPConnection :one
INSERT INTO mcp_connections (
  bot_id, name, type, config, is_active, auth_type,
  managed_by_plugin_installation_id, managed_resource_key, visible, metadata
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (bot_id, name)
DO UPDATE SET type = EXCLUDED.type,
              config = EXCLUDED.config,
              is_active = EXCLUDED.is_active,
              auth_type = EXCLUDED.auth_type,
              managed_by_plugin_installation_id = EXCLUDED.managed_by_plugin_installation_id,
              managed_resource_key = EXCLUDED.managed_resource_key,
              visible = EXCLUDED.visible,
              metadata = EXCLUDED.metadata,
              status = 'unknown',
              tools_cache = '[]'::jsonb,
              last_probed_at = NULL,
              status_message = '',
              updated_at = now()
WHERE mcp_connections.managed_by_plugin_installation_id = EXCLUDED.managed_by_plugin_installation_id
  AND mcp_connections.managed_resource_key = EXCLUDED.managed_resource_key
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type,
          managed_by_plugin_installation_id, managed_resource_key, visible, metadata, created_at, updated_at;

-- name: UpdateMCPConnection :one
UPDATE mcp_connections
SET name = $3,
    type = $4,
    config = $5,
    is_active = $6,
    auth_type = $7,
    updated_at = now()
WHERE bot_id = $1 AND id = $2
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type,
          managed_by_plugin_installation_id, managed_resource_key, visible, metadata, created_at, updated_at;

-- name: UpdateMCPConnectionActive :exec
UPDATE mcp_connections
SET is_active = $3,
    updated_at = now()
WHERE bot_id = $1 AND id = $2;

-- name: UpdateMCPConnectionsActiveByPlugin :exec
UPDATE mcp_connections
SET is_active = $3,
    updated_at = now()
WHERE bot_id = $1 AND managed_by_plugin_installation_id = $2;

-- name: UpdateMCPConnectionProbeResult :exec
UPDATE mcp_connections
SET status = $3,
    tools_cache = $4,
    last_probed_at = now(),
    status_message = $5,
    updated_at = now()
WHERE bot_id = $1 AND id = $2;

-- name: UpdateMCPConnectionAuthType :exec
UPDATE mcp_connections
SET auth_type = $2,
    updated_at = now()
WHERE id = $1;

-- name: DeleteMCPConnection :exec
DELETE FROM mcp_connections
WHERE bot_id = $1 AND id = $2;

-- name: DeleteMCPConnectionsByPlugin :exec
DELETE FROM mcp_connections
WHERE bot_id = $1 AND managed_by_plugin_installation_id = $2;

-- name: UpsertMCPConnectionByName :one
INSERT INTO mcp_connections (bot_id, name, type, config)
VALUES ($1, $2, $3, $4)
ON CONFLICT (bot_id, name)
DO UPDATE SET type = EXCLUDED.type,
              config = EXCLUDED.config,
              updated_at = now()
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type,
          managed_by_plugin_installation_id, managed_resource_key, visible, metadata, created_at, updated_at;
