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

WITH incomplete_forks AS (
  SELECT s.id
  FROM bot_sessions s
  WHERE s.deleted_at IS NULL
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
