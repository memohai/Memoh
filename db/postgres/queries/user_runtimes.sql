-- name: CreateUserRuntime :one
INSERT INTO user_runtimes (user_id, name, api_token)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(api_token))
RETURNING *;

-- name: GetUserRuntimeByAPIToken :one
SELECT runtime.*
FROM user_runtimes runtime
JOIN users owner
  ON owner.id = runtime.user_id
 AND owner.is_active = TRUE
WHERE runtime.api_token = sqlc.arg(api_token)
  AND runtime.revoked_at IS NULL;

-- name: ListUserRuntimes :many
SELECT * FROM user_runtimes
WHERE user_id = sqlc.arg(user_id) AND revoked_at IS NULL
ORDER BY created_at ASC, id ASC;

-- name: RevokeUserRuntime :one
UPDATE user_runtimes
SET revoked_at = now(), updated_at = now()
WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL
RETURNING *;

-- name: UpsertBotRemoteRuntimeBinding :one
INSERT INTO bot_remote_runtime_bindings (bot_id, runtime_id, workspace_path)
SELECT b.id, r.id, sqlc.arg(workspace_path)
FROM bots b
JOIN user_runtimes r
  ON r.id = sqlc.arg(runtime_id)
 AND r.user_id = b.owner_user_id
 AND r.revoked_at IS NULL
JOIN users owner
  ON owner.id = b.owner_user_id
 AND owner.is_active = TRUE
WHERE b.id = sqlc.arg(bot_id)
ON CONFLICT (bot_id) DO UPDATE SET
  runtime_id = EXCLUDED.runtime_id,
  workspace_path = EXCLUDED.workspace_path,
  updated_at = now()
RETURNING *;

-- name: GetBotRemoteRuntimeBinding :one
SELECT
  binding.bot_id,
  binding.runtime_id,
  binding.workspace_path,
  binding.created_at,
  binding.updated_at,
  runtime.name AS runtime_name,
  runtime.user_id AS runtime_user_id,
  (runtime.revoked_at IS NOT NULL OR NOT owner.is_active) AS runtime_unavailable,
  bot.owner_user_id AS bot_owner_user_id
FROM bot_remote_runtime_bindings binding
JOIN user_runtimes runtime ON runtime.id = binding.runtime_id
JOIN bots bot ON bot.id = binding.bot_id
JOIN users owner ON owner.id = bot.owner_user_id
WHERE binding.bot_id = sqlc.arg(bot_id);

-- name: DeleteBotRemoteRuntimeBinding :one
DELETE FROM bot_remote_runtime_bindings
WHERE bot_id = sqlc.arg(bot_id)
RETURNING *;
