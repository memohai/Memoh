-- name: NextSessionEventCursor :one
SELECT nextval('bot_session_event_cursor_seq')::bigint;

-- name: CreateSessionEvent :one
INSERT INTO bot_session_events (
  bot_id,
  session_id,
  event_kind,
  event_data,
  external_message_id,
  sender_channel_identity_id,
  received_at_ms
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT DO NOTHING
RETURNING id;

-- name: RestoreSessionEventDeliveryCompletion :execrows
UPDATE bot_session_events
SET delivery_completed_at = sqlc.arg(delivery_completed_at)::timestamptz,
    delivery_claim_token = NULL,
    delivery_claimed_until = NULL
WHERE team_id = public.memoh_current_team_id()
  AND id = sqlc.arg(event_id)::uuid;

-- name: ClaimSessionEventDelivery :one
WITH locked AS MATERIALIZED (
  SELECT event.id,
         event.delivery_claim_token,
         event.delivery_claimed_until,
         event.delivery_completed_at
  FROM bot_session_events event
  WHERE event.team_id = public.memoh_current_team_id()
    AND event.id = sqlc.arg(event_id)::uuid
  FOR NO KEY UPDATE
)
UPDATE bot_session_events event
SET delivery_claim_token = sqlc.arg(claim_token)::uuid,
    delivery_claimed_until = clock_timestamp() + sqlc.arg(lease_ms)::bigint * INTERVAL '1 millisecond'
FROM locked
WHERE event.id = locked.id
  AND locked.delivery_completed_at IS NULL
  AND (
    locked.delivery_claim_token = sqlc.arg(claim_token)::uuid
    OR locked.delivery_claimed_until IS NULL
    OR locked.delivery_claimed_until <= clock_timestamp()
  )
RETURNING event.delivery_claimed_until;

-- name: CompleteSessionEventDelivery :execrows
WITH locked AS MATERIALIZED (
  SELECT event.id,
         event.session_id,
         event.delivery_claim_token,
         event.delivery_claimed_until,
         event.delivery_completed_at
  FROM bot_session_events event
  WHERE event.team_id = public.memoh_current_team_id()
    AND event.id = sqlc.arg(event_id)::uuid
  FOR NO KEY UPDATE
)
UPDATE bot_session_events event
SET delivery_completed_at = clock_timestamp(),
    delivery_claim_token = NULL,
    delivery_claimed_until = NULL
FROM locked
WHERE event.id = locked.id
  AND locked.delivery_claim_token = sqlc.arg(claim_token)::uuid
  AND locked.delivery_claimed_until > clock_timestamp()
  AND locked.delivery_completed_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages history
    WHERE history.team_id = public.memoh_current_team_id()
      AND history.event_id = locked.id
      AND history.session_id = locked.session_id
      AND history.role = 'user'
      AND (
        COALESCE(history.metadata->>'pipeline_delivery_state', '') <> 'pending'
        OR EXISTS (
          SELECT 1
          FROM bot_history_messages response
          WHERE response.team_id = public.memoh_current_team_id()
            AND response.session_id = locked.session_id
            AND response.turn_id = history.turn_id
            AND response.role IN ('assistant', 'tool')
            AND response.turn_message_seq > history.turn_message_seq
        )
      )
  );

-- name: CompleteSessionEventDeliveryWithResponse :execrows
UPDATE bot_session_events event
SET delivery_completed_at = now(),
    delivery_claim_token = NULL,
    delivery_claimed_until = NULL
WHERE event.team_id = public.memoh_current_team_id()
  AND event.id = sqlc.arg(event_id)::uuid
  AND event.delivery_claim_token = sqlc.arg(claim_token)::uuid
  AND event.delivery_claimed_until > clock_timestamp()
  AND event.delivery_completed_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages history
    WHERE history.team_id = public.memoh_current_team_id()
      AND history.event_id = event.id
      AND history.session_id = event.session_id
      AND history.role = 'user'
  )
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages response
    WHERE response.team_id = public.memoh_current_team_id()
      AND response.id = sqlc.arg(response_message_id)::uuid
      AND response.session_id = event.session_id
      AND response.role IN ('assistant', 'tool')
  );

-- name: RenewSessionEventDelivery :one
WITH locked AS MATERIALIZED (
  SELECT id,
         delivery_claim_token,
         delivery_claimed_until
  FROM bot_session_events
  WHERE team_id = public.memoh_current_team_id()
    AND id = sqlc.arg(event_id)::uuid
  FOR NO KEY UPDATE
)
UPDATE bot_session_events event
SET delivery_claimed_until = clock_timestamp() + sqlc.arg(lease_ms)::bigint * INTERVAL '1 millisecond'
FROM locked
WHERE event.id = locked.id
  AND locked.delivery_claim_token = sqlc.arg(claim_token)::uuid
  AND locked.delivery_claimed_until > clock_timestamp()
RETURNING event.delivery_claimed_until;

-- name: ReleaseSessionEventDelivery :execrows
UPDATE bot_session_events
SET delivery_claim_token = NULL,
    delivery_claimed_until = NULL
WHERE team_id = public.memoh_current_team_id()
  AND id = sqlc.arg(event_id)::uuid
  AND delivery_claim_token = sqlc.arg(claim_token)::uuid;

-- name: LockSessionEventDeliveryClaim :one
WITH locked AS MATERIALIZED (
  SELECT event.delivery_claim_token,
         event.delivery_claimed_until,
         event.delivery_completed_at
  FROM bot_session_events event
  WHERE event.team_id = public.memoh_current_team_id()
    AND event.id = sqlc.arg(event_id)::uuid
    AND event.bot_id = sqlc.arg(bot_id)::uuid
    AND event.session_id = sqlc.arg(session_id)::uuid
  FOR NO KEY UPDATE
)
SELECT TRUE::bool
FROM locked
WHERE delivery_claim_token = sqlc.arg(claim_token)::uuid
  AND delivery_claimed_until > clock_timestamp()
  AND delivery_completed_at IS NULL;

-- name: IsSessionEventDeliveryCompleted :one
SELECT (delivery_completed_at IS NOT NULL)::bool
FROM bot_session_events
WHERE team_id = public.memoh_current_team_id()
  AND id = sqlc.arg(event_id)::uuid;

-- name: GetSessionEventIDByIdentity :one
SELECT event.id
FROM bot_session_events event
WHERE event.team_id = public.memoh_current_team_id()
  AND event.session_id = sqlc.arg(session_id)::uuid
  AND event.event_kind = sqlc.arg(event_kind)
  AND event.external_message_id = sqlc.arg(external_message_id)::text
LIMIT 1;

-- name: GetSessionEventDeliveryState :one
SELECT
  event.id,
  event.event_kind,
  event.event_data,
  (event.delivery_completed_at IS NOT NULL)::bool AS delivery_completed,
  history.id AS history_message_id,
  COALESCE(history.metadata->>'pipeline_delivery_state' = 'pending', FALSE)::bool AS history_delivery_pending,
  CASE
    WHEN history.id IS NULL THEN FALSE
    WHEN history.metadata->>'pipeline_delivery_state' = 'pending' THEN EXISTS (
      SELECT 1
      FROM bot_history_messages response
      WHERE response.team_id = public.memoh_current_team_id()
        AND response.session_id = event.session_id
        AND response.turn_id = history.turn_id
        AND response.role IN ('assistant', 'tool')
        AND response.turn_message_seq > history.turn_message_seq
    )
    ELSE TRUE
  END AS history_persisted,
  (CASE
    WHEN history.id IS NULL THEN FALSE
    ELSE EXISTS (
      SELECT 1
      FROM bot_history_messages response
      WHERE response.team_id = public.memoh_current_team_id()
        AND response.session_id = event.session_id
        AND response.turn_id = history.turn_id
        AND response.role IN ('assistant', 'tool')
        AND response.turn_message_seq > history.turn_message_seq
    )
  END)::bool AS response_persisted,
  (CASE
    WHEN history.id IS NULL THEN FALSE
    ELSE EXISTS (
      SELECT 1
      FROM bot_history_messages visible_history
      JOIN bot_history_messages response
        ON response.team_id = public.memoh_current_team_id()
       AND response.session_id = event.session_id
       AND response.turn_id = visible_history.turn_id
       AND response.role IN ('assistant', 'tool')
       AND response.turn_visible = TRUE
       AND response.turn_message_seq > visible_history.turn_message_seq
      WHERE visible_history.team_id = public.memoh_current_team_id()
        AND visible_history.session_id = event.session_id
        AND visible_history.id = history.id
        AND visible_history.role = 'user'
        AND visible_history.turn_visible = TRUE
        AND NOT EXISTS (
          SELECT 1
          FROM bot_history_messages next_request
          WHERE next_request.team_id = public.memoh_current_team_id()
            AND next_request.session_id = event.session_id
            AND next_request.turn_id = visible_history.turn_id
            AND next_request.role = 'user'
            AND next_request.event_id IS NOT NULL
            AND next_request.turn_message_seq > visible_history.turn_message_seq
            AND next_request.turn_message_seq < response.turn_message_seq
        )
    )
  END)::bool AS replay_response_persisted
FROM bot_session_events event
LEFT JOIN LATERAL (
  SELECT message.id, message.metadata, message.turn_id, message.turn_message_seq
  FROM bot_history_messages message
  WHERE message.team_id = public.memoh_current_team_id()
    AND message.event_id = event.id
    AND message.session_id = event.session_id
    AND message.role = 'user'
  ORDER BY message.created_at, message.id
  LIMIT 1
) history ON TRUE
WHERE event.team_id = public.memoh_current_team_id()
  AND event.id = sqlc.arg(event_id)::uuid
LIMIT 1;

-- name: ListSessionEventsBySession :many
SELECT event.*
FROM bot_session_events event
WHERE event.team_id = public.memoh_current_team_id()
  AND event.session_id = $1
  AND (
    event.delivery_completed_at IS NOT NULL
    OR EXISTS (
      SELECT 1
      FROM bot_history_messages history
      WHERE history.team_id = public.memoh_current_team_id()
        AND history.event_id = event.id
        AND history.session_id = event.session_id
        AND history.role = 'user'
    )
  )
ORDER BY event.received_at_ms ASC, event.created_at ASC, event.id ASC;

-- name: ListSessionEventsBySessionAfter :many
SELECT * FROM bot_session_events
WHERE team_id = public.memoh_current_team_id() AND session_id = $1 AND received_at_ms >= $2
ORDER BY received_at_ms ASC, created_at ASC, id ASC;

-- name: ListSessionEventsByBot :many
SELECT * FROM bot_session_events
WHERE team_id = public.memoh_current_team_id() AND bot_id = $1
ORDER BY received_at_ms ASC, id ASC;

-- name: CountSessionEvents :one
SELECT COUNT(*) FROM bot_session_events
WHERE team_id = public.memoh_current_team_id() AND session_id = $1;

-- name: DeleteSessionEventsByBot :exec
DELETE FROM bot_session_events
WHERE team_id = public.memoh_current_team_id() AND bot_id = $1;
