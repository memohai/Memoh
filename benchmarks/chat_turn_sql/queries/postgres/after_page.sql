-- Benchmark scenario: after_page
-- Production query: db/postgres/queries/messages.sql ListMessagesAfterBySession
WITH RECURSIVE selected_head AS (
  SELECT
    bs.id AS session_id,
    CASE
      WHEN $2::uuid IS NULL THEN bs.default_head_turn_id
      ELSE h.head_turn_id
    END AS head_turn_id
  FROM bot_sessions bs
  LEFT JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
    AND h.head_turn_id = $2::uuid
  WHERE bs.id = $1::uuid
    AND bs.deleted_at IS NULL
), visible_turns AS (
  SELECT t.id, t.parent_turn_id, 0::bigint AS depth
  FROM selected_head sh
  JOIN bot_history_turns t ON t.id = sh.head_turn_id
  JOIN bot_sessions bs ON bs.id = sh.session_id
    AND bs.bot_id = t.bot_id
  UNION ALL
  SELECT p.id, p.parent_turn_id, vt.depth + 1
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.turn_id,
  m.turn_message_seq,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM visible_turns vt
JOIN bot_history_messages m ON m.turn_id = vt.id
LEFT JOIN bot_history_messages cursor_message ON cursor_message.id = $3::uuid
LEFT JOIN visible_turns cursor_turn ON cursor_turn.id = cursor_message.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE (
  (
    $3::uuid IS NOT NULL
    AND cursor_turn.id IS NOT NULL
    AND (
      (-vt.depth, COALESCE(m.turn_message_seq, 0)::bigint, m.created_at, m.id)
      > (-cursor_turn.depth, COALESCE(cursor_message.turn_message_seq, 0)::bigint, cursor_message.created_at, cursor_message.id)
    )
  )
  OR (
    $3::uuid IS NULL
    AND m.created_at > $4::timestamptz
  )
)
ORDER BY vt.depth DESC, COALESCE(m.turn_message_seq, 0) ASC, m.created_at ASC, m.id ASC
LIMIT $5::bigint;
