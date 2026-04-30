-- name: GetMCPConnectionByID :one
SELECT id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type, created_at, updated_at
FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id)
LIMIT 1;

-- name: ListMCPConnectionsByBotID :many
SELECT id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type, created_at, updated_at
FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY created_at DESC;

-- name: CreateMCPConnection :one
INSERT INTO mcp_connections (id, bot_id, name, type, config, is_active, auth_type)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(name),
  sqlc.arg(type),
  sqlc.arg(config),
  sqlc.arg(is_active),
  sqlc.arg(auth_type)
)
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type, created_at, updated_at;

-- name: UpdateMCPConnection :one
UPDATE mcp_connections
SET name = sqlc.arg(name),
    type = sqlc.arg(type),
    config = sqlc.arg(config),
    is_active = sqlc.arg(is_active),
    auth_type = sqlc.arg(auth_type),
    updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id)
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type, created_at, updated_at;

-- name: UpdateMCPConnectionProbeResult :exec
UPDATE mcp_connections
SET status = sqlc.arg(status),
    tools_cache = sqlc.arg(tools_cache),
    last_probed_at = CURRENT_TIMESTAMP,
    status_message = sqlc.arg(status_message),
    updated_at = CURRENT_TIMESTAMP
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id);

-- name: UpdateMCPConnectionAuthType :exec
UPDATE mcp_connections
SET auth_type = sqlc.arg(auth_type),
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id);

-- name: DeleteMCPConnection :exec
DELETE FROM mcp_connections
WHERE bot_id = sqlc.arg(bot_id) AND id = sqlc.arg(id);

-- name: UpsertMCPConnectionByName :one
INSERT INTO mcp_connections (id, bot_id, name, type, config)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(name),
  sqlc.arg(type),
  sqlc.arg(config)
)
ON CONFLICT (bot_id, name)
DO UPDATE SET type = EXCLUDED.type,
              config = EXCLUDED.config,
              updated_at = CURRENT_TIMESTAMP
RETURNING id, bot_id, name, type, config, is_active, status, tools_cache, last_probed_at, status_message, auth_type, created_at, updated_at;
