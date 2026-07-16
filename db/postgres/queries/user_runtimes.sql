-- name: CreateUserRuntime :one
INSERT INTO user_runtimes (user_id, name, api_token)
VALUES (sqlc.arg(user_id), sqlc.arg(name), sqlc.arg(api_token))
RETURNING *;

-- name: GetUserRuntimeByAPIToken :one
SELECT runtime.*
FROM user_runtimes runtime
JOIN users owner
  ON owner.id = runtime.user_id
 AND owner.team_id = public.memoh_current_team_id()
 AND owner.is_active = TRUE
WHERE runtime.team_id = public.memoh_current_team_id()
  AND runtime.api_token = sqlc.arg(api_token)
  AND runtime.revoked_at IS NULL;

-- name: ListUserRuntimes :many
SELECT * FROM user_runtimes
WHERE team_id = public.memoh_current_team_id()
  AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL
ORDER BY created_at ASC, id ASC;

-- name: RevokeUserRuntime :one
UPDATE user_runtimes
SET revoked_at = now(), updated_at = now()
WHERE team_id = public.memoh_current_team_id()
  AND id = sqlc.arg(id) AND user_id = sqlc.arg(user_id) AND revoked_at IS NULL
RETURNING *;

-- name: CreateOrUpdateBotRemoteRuntimeMount :one
INSERT INTO bot_remote_runtime_bindings (bot_id, runtime_id, workspace_path)
SELECT b.id, r.id, sqlc.arg(workspace_path)
FROM bots b
JOIN user_runtimes r
  ON r.id = sqlc.arg(runtime_id)
 AND r.team_id = public.memoh_current_team_id()
 AND r.user_id = b.owner_user_id
 AND r.revoked_at IS NULL
JOIN users owner
  ON owner.id = b.owner_user_id
 AND owner.team_id = public.memoh_current_team_id()
 AND owner.is_active = TRUE
WHERE b.team_id = public.memoh_current_team_id()
  AND b.id = sqlc.arg(bot_id)
ON CONFLICT (team_id, bot_id, runtime_id) DO UPDATE SET
  workspace_path = EXCLUDED.workspace_path,
  updated_at = now()
RETURNING id;

-- name: ListBotRemoteRuntimeMounts :many
SELECT
  binding.id,
  binding.bot_id,
  binding.runtime_id,
  binding.workspace_path,
  binding.is_primary,
  binding.tool_approval_config,
  binding.created_at,
  binding.updated_at,
  runtime.name AS runtime_name,
  runtime.user_id AS runtime_user_id,
  (runtime.revoked_at IS NOT NULL OR NOT owner.is_active) AS runtime_unavailable,
  bot.owner_user_id AS bot_owner_user_id
FROM bot_remote_runtime_bindings binding
JOIN user_runtimes runtime ON runtime.id = binding.runtime_id AND runtime.team_id = public.memoh_current_team_id()
JOIN bots bot ON bot.id = binding.bot_id AND bot.team_id = public.memoh_current_team_id()
JOIN users owner ON owner.id = bot.owner_user_id AND owner.team_id = public.memoh_current_team_id()
WHERE binding.team_id = public.memoh_current_team_id()
  AND binding.bot_id = sqlc.arg(bot_id)
ORDER BY binding.created_at ASC, binding.id ASC;

-- name: GetBotRemoteRuntimeMount :one
SELECT
  binding.id,
  binding.bot_id,
  binding.runtime_id,
  binding.workspace_path,
  binding.is_primary,
  binding.tool_approval_config,
  binding.created_at,
  binding.updated_at,
  runtime.name AS runtime_name,
  runtime.user_id AS runtime_user_id,
  (runtime.revoked_at IS NOT NULL OR NOT owner.is_active) AS runtime_unavailable,
  bot.owner_user_id AS bot_owner_user_id
FROM bot_remote_runtime_bindings binding
JOIN user_runtimes runtime ON runtime.id = binding.runtime_id AND runtime.team_id = public.memoh_current_team_id()
JOIN bots bot ON bot.id = binding.bot_id AND bot.team_id = public.memoh_current_team_id()
JOIN users owner ON owner.id = bot.owner_user_id AND owner.team_id = public.memoh_current_team_id()
WHERE binding.team_id = public.memoh_current_team_id()
  AND binding.bot_id = sqlc.arg(bot_id)
  AND binding.id = sqlc.arg(target_id);

-- name: GetPrimaryBotRemoteRuntimeMount :one
SELECT
  binding.id,
  binding.bot_id,
  binding.runtime_id,
  binding.workspace_path,
  binding.is_primary,
  binding.tool_approval_config,
  binding.created_at,
  binding.updated_at,
  runtime.name AS runtime_name,
  runtime.user_id AS runtime_user_id,
  (runtime.revoked_at IS NOT NULL OR NOT owner.is_active) AS runtime_unavailable,
  bot.owner_user_id AS bot_owner_user_id
FROM bot_remote_runtime_bindings binding
JOIN user_runtimes runtime ON runtime.id = binding.runtime_id AND runtime.team_id = public.memoh_current_team_id()
JOIN bots bot ON bot.id = binding.bot_id AND bot.team_id = public.memoh_current_team_id()
JOIN users owner ON owner.id = bot.owner_user_id AND owner.team_id = public.memoh_current_team_id()
WHERE binding.team_id = public.memoh_current_team_id()
  AND binding.bot_id = sqlc.arg(bot_id)
  AND binding.is_primary = TRUE;

-- name: ClearBotRemoteRuntimePrimary :exec
UPDATE bot_remote_runtime_bindings
SET is_primary = FALSE,
    updated_at = CASE WHEN is_primary THEN now() ELSE updated_at END
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = sqlc.arg(bot_id);

-- name: SetBotRemoteRuntimePrimary :execrows
UPDATE bot_remote_runtime_bindings
SET is_primary = TRUE,
    updated_at = CASE WHEN is_primary THEN updated_at ELSE now() END
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = sqlc.arg(bot_id)
  AND id = sqlc.arg(target_id);

-- name: UpdateBotRemoteRuntimeMountToolApproval :one
UPDATE bot_remote_runtime_bindings
SET tool_approval_config = sqlc.arg(tool_approval_config),
    updated_at = now()
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = sqlc.arg(bot_id)
  AND id = sqlc.arg(target_id)
RETURNING id;

-- name: DeleteBotRemoteRuntimeMount :one
DELETE FROM bot_remote_runtime_bindings
WHERE team_id = public.memoh_current_team_id()
  AND bot_id = sqlc.arg(bot_id)
  AND id = sqlc.arg(target_id)
RETURNING id;
