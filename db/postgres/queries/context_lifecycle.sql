-- name: ListRecentAssistantMessagesBySession :many
SELECT
  id,
  run_id,
  role,
  metadata,
  created_at
FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id)
  AND role = 'assistant'
  AND metadata IS NOT NULL
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(max_count);

-- name: CreateContextLifecycle :one
INSERT INTO context_lifecycles (
  run_id,
  bot_id,
  session_id,
  status,
  error_code,
  snapshot
)
VALUES (
  sqlc.arg(run_id),
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.arg(status),
  sqlc.narg(error_code)::text,
  sqlc.arg(snapshot)
)
RETURNING *;

-- name: GetContextLifecycleByRunID :one
SELECT *
FROM context_lifecycles
WHERE team_id = public.memoh_current_team_id()
  AND run_id = sqlc.arg(run_id);

-- name: UpdateAbortedContextLifecycleSnapshot :one
UPDATE context_lifecycles
SET snapshot = sqlc.arg(snapshot)
WHERE team_id = public.memoh_current_team_id()
  AND run_id = sqlc.arg(run_id)
  AND bot_id = sqlc.arg(bot_id)
  AND session_id = sqlc.arg(session_id)
  AND status = 'aborted'
RETURNING *;

-- name: UpsertAbortedContextLifecycle :one
INSERT INTO context_lifecycles (
  run_id,
  bot_id,
  session_id,
  status,
  error_code,
  snapshot
)
VALUES (
  sqlc.arg(run_id),
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  'aborted',
  NULL,
  sqlc.arg(snapshot)
)
ON CONFLICT (run_id) DO UPDATE
SET
  status = 'aborted',
  error_code = NULL
WHERE context_lifecycles.team_id = public.memoh_current_team_id()
  AND context_lifecycles.bot_id = EXCLUDED.bot_id
  AND context_lifecycles.session_id = EXCLUDED.session_id
RETURNING *;

-- name: GetLatestAssistantContextLifecycleByRunID :one
SELECT id, metadata
FROM bot_history_messages
WHERE team_id = public.memoh_current_team_id()
  AND run_id = sqlc.arg(run_id)
  AND role = 'assistant'
  AND metadata ? 'context_lifecycle'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestAssistantContextLifecycleMetadataByRunID :one
SELECT metadata
FROM bot_history_messages
WHERE team_id = public.memoh_current_team_id()
  AND run_id = sqlc.arg(run_id)
  AND role = 'assistant'
  AND metadata ? 'context_lifecycle'
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: ListRecentContextLifecyclesBySession :many
SELECT
  run_id,
  status,
  error_code,
  created_at,
  snapshot
FROM context_lifecycles
WHERE team_id = public.memoh_current_team_id()
  AND session_id = sqlc.arg(session_id)
ORDER BY created_at DESC, run_id DESC
LIMIT sqlc.arg(max_count);
