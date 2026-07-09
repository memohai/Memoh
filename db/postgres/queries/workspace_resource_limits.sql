-- name: GetBotWorkspaceResourceLimits :one
SELECT limits.*
FROM bot_workspace_resource_limits limits
JOIN bots b ON b.id = limits.bot_id
WHERE limits.bot_id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id);

-- name: UpsertBotWorkspaceResourceLimits :one
INSERT INTO bot_workspace_resource_limits (
  bot_id, cpu_millicores, memory_bytes, storage_bytes
)
SELECT
  b.id,
  sqlc.arg(cpu_millicores),
  sqlc.arg(memory_bytes),
  sqlc.arg(storage_bytes)
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)
ON CONFLICT (bot_id) DO UPDATE SET
  cpu_millicores = EXCLUDED.cpu_millicores,
  memory_bytes = EXCLUDED.memory_bytes,
  storage_bytes = EXCLUDED.storage_bytes,
  updated_at = now()
WHERE EXISTS (
  SELECT 1
  FROM bots b
  WHERE b.id = bot_workspace_resource_limits.bot_id
    AND b.team_id = sqlc.arg(team_id)
)
RETURNING *;
