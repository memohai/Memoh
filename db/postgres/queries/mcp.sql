-- name: GetMCPConnectionByID :one
SELECT *
FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id)
  AND mcp_connections.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
LIMIT 1;

-- name: ListMCPConnectionsByBotID :many
SELECT *
FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
ORDER BY created_at DESC;

-- name: CreateMCPConnection :one
INSERT INTO mcp_connections (team_id, bot_id, name, type, config, is_active, auth_type)
SELECT b.team_id, b.id, sqlc.arg(name), sqlc.arg(type), sqlc.arg(config), sqlc.arg(is_active), sqlc.arg(auth_type)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
RETURNING *;

-- name: CreateManagedMCPConnection :one
INSERT INTO mcp_connections (
  team_id, bot_id, name, type, config, is_active, auth_type,
  managed_by_plugin_installation_id, managed_resource_key, visible, metadata
)
SELECT b.team_id, b.id, sqlc.arg(name), sqlc.arg(type), sqlc.arg(config), sqlc.arg(is_active), sqlc.arg(auth_type), i.id, sqlc.arg(managed_resource_key), sqlc.arg(visible), sqlc.arg(metadata)
FROM bots b
JOIN bot_plugin_installations i
  ON i.id = sqlc.arg(managed_by_plugin_installation_id)
 AND i.bot_id = b.id
 AND i.team_id = b.team_id
WHERE b.id = sqlc.arg(bot_id)
RETURNING *;

-- name: UpdateMCPConnection :one
UPDATE mcp_connections
SET name = sqlc.arg(name),
    type = sqlc.arg(type),
    config = sqlc.arg(config),
    is_active = sqlc.arg(is_active),
    auth_type = sqlc.arg(auth_type),
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND mcp_connections.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id))
RETURNING *;

-- name: UpdateMCPConnectionActive :exec
UPDATE mcp_connections
SET is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND mcp_connections.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id));

-- name: UpdateMCPConnectionsActiveByPlugin :exec
UPDATE mcp_connections
SET is_active = sqlc.arg(is_active),
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND managed_by_plugin_installation_id = sqlc.arg(managed_by_plugin_installation_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id));

-- name: UpdateMCPConnectionProbeResult :exec
UPDATE mcp_connections
SET status = sqlc.arg(status),
    tools_cache = sqlc.arg(tools_cache),
    last_probed_at = now(),
    status_message = sqlc.arg(status_message),
    updated_at = now()
WHERE bot_id = sqlc.arg(bot_id)
  AND mcp_connections.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id));

-- name: UpdateMCPConnectionAuthType :exec
UPDATE mcp_connections
SET auth_type = sqlc.arg(auth_type),
    updated_at = now()
WHERE mcp_connections.id = sqlc.arg(id)
  AND team_id = (SELECT t.team_id FROM mcp_oauth_tokens t WHERE t.connection_id = sqlc.arg(id));

-- name: DeleteMCPConnection :exec
DELETE FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id)
  AND mcp_connections.id = sqlc.arg(id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id));

-- name: DeleteMCPConnectionsByPlugin :exec
DELETE FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id)
  AND managed_by_plugin_installation_id = sqlc.arg(managed_by_plugin_installation_id)
  AND team_id = (SELECT b.team_id FROM bots b WHERE b.id = sqlc.arg(bot_id));

-- name: UpsertMCPConnectionByName :one
INSERT INTO mcp_connections (team_id, bot_id, name, type, config)
SELECT b.team_id, b.id, sqlc.arg(name), sqlc.arg(type), sqlc.arg(config)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
ON CONFLICT (team_id, bot_id, name)
DO UPDATE SET type = EXCLUDED.type,
              config = EXCLUDED.config,
              updated_at = now()
RETURNING *;
