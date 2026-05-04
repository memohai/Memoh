-- name: CreateSessionEvent :one
INSERT INTO bot_session_events (id, bot_id, session_id, event_kind, event_data, external_message_id, sender_channel_identity_id, received_at_ms)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  sqlc.arg(event_kind),
  sqlc.arg(event_data),
  sqlc.arg(external_message_id),
  sqlc.arg(sender_channel_identity_id),
  sqlc.arg(received_at_ms)
)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: ListSessionEventsBySession :many
SELECT * FROM bot_session_events
WHERE session_id = sqlc.arg(session_id)
ORDER BY received_at_ms ASC;

-- name: ListSessionEventsBySessionAfter :many
SELECT * FROM bot_session_events
WHERE session_id = sqlc.arg(session_id) AND received_at_ms >= sqlc.arg(received_at_ms)
ORDER BY received_at_ms ASC;

-- name: CountSessionEvents :one
SELECT COUNT(*) FROM bot_session_events
WHERE session_id = sqlc.arg(session_id);
