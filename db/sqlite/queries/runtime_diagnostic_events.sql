-- name: CreateRuntimeDiagnosticEvent :one
INSERT INTO runtime_diagnostic_events (
  id,
  bot_id,
  scope,
  agent_id,
  session_id,
  runtime_id,
  phase,
  severity,
  code,
  message,
  metadata
) VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(scope),
  sqlc.arg(agent_id),
  sqlc.arg(session_id),
  sqlc.arg(runtime_id),
  sqlc.arg(phase),
  sqlc.arg(severity),
  sqlc.arg(code),
  sqlc.arg(message),
  sqlc.arg(metadata)
)
RETURNING id, bot_id, scope, agent_id, session_id, runtime_id, phase, severity, code, message, metadata, created_at;

-- name: ListRuntimeDiagnosticEventsByBot :many
SELECT id, bot_id, scope, agent_id, session_id, runtime_id, phase, severity, code, message, metadata, created_at
FROM runtime_diagnostic_events
WHERE bot_id = sqlc.arg(bot_id)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(limit_count);

-- name: DeleteRuntimeDiagnosticEventsBefore :exec
DELETE FROM runtime_diagnostic_events
WHERE created_at < sqlc.arg(before);

-- name: ListRuntimeDiagnosticEventBotIDs :many
SELECT DISTINCT bot_id
FROM runtime_diagnostic_events;

-- name: PruneRuntimeDiagnosticEventsByBotLimit :exec
DELETE FROM runtime_diagnostic_events
WHERE runtime_diagnostic_events.bot_id = sqlc.arg(bot_id)
  AND id NOT IN (
    SELECT id
    FROM runtime_diagnostic_events AS keep
    WHERE keep.bot_id = sqlc.arg(bot_id)
    ORDER BY created_at DESC, id DESC
    LIMIT sqlc.arg(keep_count)
  );
