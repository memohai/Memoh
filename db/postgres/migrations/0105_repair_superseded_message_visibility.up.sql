-- 0105_repair_superseded_message_visibility
-- Hide superseded history messages that stayed visible because ReplaceHistoryTurn
-- previously updated superseded metadata and visibility in separate CTE updates.
-- Also hide incomplete fork sessions left behind by the old
-- ForkSessionFromAssistantMessage CTE when the request returned an error after
-- copying messages.

UPDATE bot_history_messages
SET turn_visible = false
WHERE turn_visible = true
  AND turn_superseded_at IS NOT NULL;

WITH incomplete_fork_sessions AS (
  SELECT s.id, s.created_at
  FROM bot_sessions s
  WHERE s.deleted_at IS NULL
    AND s.type = 'chat'
    AND s.session_mode = 'chat'
    AND s.metadata ? 'forked_from'
    AND (s.metadata->'forked_from') ? 'session_id'
    AND (s.metadata->'forked_from') ? 'message_id'
    AND NOT ((s.metadata->'forked_from') ? 'fork_message_id')
),
retained_incomplete_forks AS (
  SELECT f.id, f.created_at
  FROM incomplete_fork_sessions f
  WHERE EXISTS (
    SELECT 1
    FROM bot_history_messages m
    WHERE m.session_id = f.id
      AND m.created_at > f.created_at
  )
),
copied_prefix AS (
  SELECT
    f.id AS session_id,
    COALESCE(MAX(m.turn_position), 0)::bigint AS max_position
  FROM retained_incomplete_forks f
  LEFT JOIN bot_history_messages m
    ON m.session_id = f.id
   AND m.turn_id IS NOT NULL
   AND m.turn_position IS NOT NULL
   AND m.created_at <= f.created_at
  GROUP BY f.id
),
post_fork_turns AS (
  SELECT
    f.id AS session_id,
    m.turn_id,
    MIN(m.created_at) AS first_created_at,
    MIN(m.turn_position) AS old_position
  FROM retained_incomplete_forks f
  JOIN bot_history_messages m
    ON m.session_id = f.id
   AND m.turn_id IS NOT NULL
   AND m.turn_position IS NOT NULL
  GROUP BY f.id, f.created_at, m.turn_id
  HAVING MIN(m.created_at) > f.created_at
),
renumber_plan AS (
  SELECT
    p.session_id,
    p.turn_id,
    copied_prefix.max_position
      + (ROW_NUMBER() OVER (
          PARTITION BY p.session_id
          ORDER BY p.first_created_at ASC, p.old_position ASC, p.turn_id ASC
        ))::bigint AS turn_position
  FROM post_fork_turns p
  JOIN copied_prefix ON copied_prefix.session_id = p.session_id
),
renumbered_messages AS (
  UPDATE bot_history_messages m
  SET turn_position = renumber_plan.turn_position
  FROM renumber_plan
  WHERE m.session_id = renumber_plan.session_id
    AND m.turn_id = renumber_plan.turn_id
  RETURNING m.session_id
),
repaired_next_turn_position AS (
  SELECT
    f.id AS session_id,
    (
      GREATEST(
        COALESCE(copied_prefix.max_position, 0),
        COALESCE(MAX(renumber_plan.turn_position), 0)
      ) + 1
    )::bigint AS value
  FROM retained_incomplete_forks f
  JOIN copied_prefix ON copied_prefix.session_id = f.id
  LEFT JOIN renumber_plan ON renumber_plan.session_id = f.id
  GROUP BY f.id, copied_prefix.max_position
)
UPDATE bot_sessions s
SET next_turn_position = GREATEST(s.next_turn_position, repaired_next_turn_position.value),
    updated_at = now(),
    metadata = jsonb_set(
      s.metadata,
      '{repair}',
      COALESCE(s.metadata->'repair', '{}'::jsonb) || jsonb_build_object('0105_incomplete_fork_turn_positions', true),
      true
    )
FROM repaired_next_turn_position
WHERE s.id = repaired_next_turn_position.session_id
  AND EXISTS (
    SELECT 1
    FROM renumbered_messages
    WHERE renumbered_messages.session_id = s.id
  );

WITH incomplete_forks AS (
  SELECT s.id
  FROM bot_sessions s
  WHERE s.deleted_at IS NULL
    AND s.type = 'chat'
    AND s.session_mode = 'chat'
    AND s.metadata ? 'forked_from'
    AND (s.metadata->'forked_from') ? 'session_id'
    AND (s.metadata->'forked_from') ? 'message_id'
    AND NOT ((s.metadata->'forked_from') ? 'fork_message_id')
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_messages m
      WHERE m.session_id = s.id
        AND m.created_at > s.created_at
    )
)
UPDATE bot_sessions s
SET deleted_at = now(),
    updated_at = now(),
    metadata = jsonb_set(
      s.metadata,
      '{repair}',
      COALESCE(s.metadata->'repair', '{}'::jsonb) || jsonb_build_object('0105_incomplete_fork_session', true),
      true
    )
FROM incomplete_forks f
WHERE s.id = f.id;
