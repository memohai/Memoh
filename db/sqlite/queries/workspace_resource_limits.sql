-- name: GetBotWorkspaceResourceLimits :one
SELECT * FROM bot_workspace_resource_limits WHERE bot_id = sqlc.arg(bot_id);

-- name: UpsertBotWorkspaceResourceLimits :one
INSERT INTO bot_workspace_resource_limits (
  bot_id, cpu_millicores, memory_bytes, storage_bytes
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(cpu_millicores),
  sqlc.arg(memory_bytes),
  sqlc.arg(storage_bytes)
)
ON CONFLICT (bot_id) DO UPDATE SET
  cpu_millicores = EXCLUDED.cpu_millicores,
  memory_bytes = EXCLUDED.memory_bytes,
  storage_bytes = EXCLUDED.storage_bytes,
  updated_at = CURRENT_TIMESTAMP
RETURNING *;
