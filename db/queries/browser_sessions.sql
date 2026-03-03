-- name: CreateBrowserSession :one
INSERT INTO browser_sessions (
  bot_id, session_id, provider, remote_session_id, worker_id, status, current_url, context_dir, idle_ttl_seconds,
  action_count, metadata, created_at, updated_at, last_used_at, expires_at
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.arg(provider),
  sqlc.arg(remote_session_id),
  sqlc.arg(worker_id),
  sqlc.arg(status),
  sqlc.arg(current_url),
  sqlc.arg(context_dir),
  sqlc.arg(idle_ttl_seconds),
  sqlc.arg(action_count),
  sqlc.arg(metadata),
  now(),
  now(),
  now(),
  sqlc.arg(expires_at)
)
RETURNING *;

-- name: GetBrowserSessionBySessionID :one
SELECT * FROM browser_sessions WHERE session_id = sqlc.arg(session_id) LIMIT 1;

-- name: ListBrowserSessionsByBot :many
SELECT * FROM browser_sessions
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY updated_at DESC;

-- name: CountActiveBrowserSessionsByBot :one
SELECT COUNT(*) FROM browser_sessions
WHERE bot_id = sqlc.arg(bot_id) AND status = 'active';

-- name: TouchBrowserSession :one
UPDATE browser_sessions
SET current_url = sqlc.arg(current_url),
    action_count = sqlc.arg(action_count),
    metadata = sqlc.arg(metadata),
    updated_at = now(),
    last_used_at = now(),
    expires_at = sqlc.arg(expires_at)
WHERE session_id = sqlc.arg(session_id)
RETURNING *;

-- name: CloseBrowserSession :one
UPDATE browser_sessions
SET status = 'closed',
    metadata = sqlc.arg(metadata),
    updated_at = now(),
    expires_at = now()
WHERE session_id = sqlc.arg(session_id)
RETURNING *;

-- name: ExpireBrowserSessionsBefore :many
UPDATE browser_sessions
SET status = 'expired',
    updated_at = now()
WHERE status = 'active' AND expires_at <= sqlc.arg(now_at)
RETURNING *;

-- name: CloseBrowserSessionsByBot :exec
UPDATE browser_sessions
SET status = 'closed',
    updated_at = now(),
    expires_at = now()
WHERE bot_id = sqlc.arg(bot_id) AND status = 'active';
