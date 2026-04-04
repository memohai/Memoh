-- name: CreateSessionEvent :exec
INSERT INTO bot_session_events (
  bot_id,
  session_id,
  event_kind,
  event_data,
  external_message_id,
  sender_channel_identity_id,
  received_at_ms
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING;

-- name: ListSessionEventsBySession :many
SELECT * FROM bot_session_events
WHERE session_id = $1
ORDER BY received_at_ms ASC;

-- name: ListSessionEventsBySessionAfter :many
SELECT * FROM bot_session_events
WHERE session_id = $1 AND received_at_ms >= $2
ORDER BY received_at_ms ASC;

-- name: CountSessionEvents :one
SELECT COUNT(*) FROM bot_session_events
WHERE session_id = $1;
