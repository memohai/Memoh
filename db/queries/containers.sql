-- name: UpsertContainer :exec
INSERT INTO containers (
  bot_id, container_id, container_name, image, status, namespace, auto_start,
  host_path, container_path, last_started_at, last_stopped_at
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(container_id),
  sqlc.arg(container_name),
  sqlc.arg(image),
  sqlc.arg(status),
  sqlc.arg(namespace),
  sqlc.arg(auto_start),
  sqlc.arg(host_path),
  sqlc.arg(container_path),
  sqlc.arg(last_started_at),
  sqlc.arg(last_stopped_at)
)
ON CONFLICT (container_id) DO UPDATE SET
  bot_id = EXCLUDED.bot_id,
  container_name = EXCLUDED.container_name,
  image = EXCLUDED.image,
  status = EXCLUDED.status,
  namespace = EXCLUDED.namespace,
  auto_start = EXCLUDED.auto_start,
  host_path = EXCLUDED.host_path,
  container_path = EXCLUDED.container_path,
  last_started_at = EXCLUDED.last_started_at,
  last_stopped_at = EXCLUDED.last_stopped_at,
  updated_at = now();

-- name: GetContainerByBotID :one
SELECT * FROM containers WHERE bot_id = sqlc.arg(bot_id) ORDER BY updated_at DESC LIMIT 1;

-- name: DeleteContainerByBotID :exec
DELETE FROM containers WHERE bot_id = sqlc.arg(bot_id);

-- name: UpdateContainerStatus :exec
UPDATE containers
SET status = sqlc.arg(status), updated_at = now()
WHERE bot_id = sqlc.arg(bot_id);

-- name: UpdateContainerStarted :exec
UPDATE containers
SET status = 'running', last_started_at = now(), updated_at = now()
WHERE bot_id = sqlc.arg(bot_id);

-- name: UpdateContainerStopped :exec
UPDATE containers
SET status = 'stopped', last_stopped_at = now(), updated_at = now()
WHERE bot_id = sqlc.arg(bot_id);

-- name: ListAutoStartContainers :many
SELECT * FROM containers WHERE auto_start = true ORDER BY updated_at DESC;
