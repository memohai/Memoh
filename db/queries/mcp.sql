-- name: GetMCPConnectionByID :one
SELECT id, bot_id, name, type, config, is_active, created_at, updated_at
FROM mcp_connections
WHERE bot_id = $1 AND id = $2
LIMIT 1;

-- name: ListMCPConnectionsByBotID :many
SELECT id, bot_id, name, type, config, is_active, created_at, updated_at
FROM mcp_connections
WHERE bot_id = $1
ORDER BY created_at DESC;

-- name: CreateMCPConnection :one
INSERT INTO mcp_connections (bot_id, name, type, config, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, bot_id, name, type, config, is_active, created_at, updated_at;

-- name: UpdateMCPConnection :one
UPDATE mcp_connections
SET name = $3,
    type = $4,
    config = $5,
    is_active = $6,
    updated_at = now()
WHERE bot_id = $1 AND id = $2
RETURNING id, bot_id, name, type, config, is_active, created_at, updated_at;

-- name: DeleteMCPConnection :exec
DELETE FROM mcp_connections
WHERE bot_id = $1 AND id = $2;

-- name: UpsertMCPConnectionByName :one
INSERT INTO mcp_connections (bot_id, name, type, config)
VALUES ($1, $2, $3, $4)
ON CONFLICT (bot_id, name)
DO UPDATE SET type = EXCLUDED.type,
              config = EXCLUDED.config,
              updated_at = now()
RETURNING id, bot_id, name, type, config, is_active, created_at, updated_at;
