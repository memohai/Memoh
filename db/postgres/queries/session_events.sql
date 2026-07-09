-- name: CreateSessionEvent :one
INSERT INTO bot_session_events (
  team_id,
  bot_id,
  session_id,
  event_kind,
  event_data,
  external_message_id,
  sender_channel_identity_id,
  received_at_ms
) VALUES (
  sqlc.arg(team_id)::uuid,
  sqlc.arg(bot_id)::uuid,
  sqlc.arg(session_id)::uuid,
  sqlc.arg(event_kind),
  sqlc.arg(event_data),
  sqlc.narg(external_message_id)::text,
  sqlc.narg(sender_channel_identity_id)::uuid,
  sqlc.arg(received_at_ms)
)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: ListSessionEventsBySession :many
SELECT * FROM bot_session_events
WHERE session_id = sqlc.arg(session_id)
  AND team_id = sqlc.arg(team_id)::uuid
ORDER BY received_at_ms ASC;

-- name: ListSessionEventsBySessionAfter :many
SELECT * FROM bot_session_events
WHERE session_id = sqlc.arg(session_id)
  AND team_id = sqlc.arg(team_id)::uuid
  AND received_at_ms >= sqlc.arg(received_at_ms)
ORDER BY received_at_ms ASC;

-- name: ListSessionEventsByBot :many
SELECT * FROM bot_session_events
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid
ORDER BY received_at_ms ASC, id ASC;

-- name: CountSessionEvents :one
SELECT COUNT(*) FROM bot_session_events
WHERE session_id = sqlc.arg(session_id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: DeleteSessionEventsByBot :exec
DELETE FROM bot_session_events
WHERE bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid;
