-- name: CreateMessage :one
WITH branch_lock AS (
  SELECT id
  FROM bot_session_branches
  WHERE id = sqlc.narg(branch_id)::uuid
  FOR UPDATE
),
turn_lock AS (
  SELECT id
  FROM bot_history_turns
  WHERE id = sqlc.narg(turn_id)::uuid
  FOR UPDATE
)
INSERT INTO bot_history_messages (
  bot_id,
  session_id,
  branch_id,
  branch_seq,
  turn_id,
  turn_message_seq,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  model_id,
  event_id,
  display_text
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(session_id)::uuid,
  sqlc.narg(branch_id)::uuid,
  CASE
    WHEN sqlc.narg(branch_id)::uuid IS NULL THEN NULL
    ELSE (
      SELECT COALESCE(MAX(existing.branch_seq), 0) + 1
      FROM branch_lock locked
      LEFT JOIN bot_history_messages existing ON existing.branch_id = locked.id
    )
  END,
  sqlc.narg(turn_id)::uuid,
  CASE
    WHEN sqlc.narg(turn_id)::uuid IS NULL THEN NULL
    ELSE (
      SELECT COALESCE(MAX(existing.turn_message_seq), 0) + 1
      FROM turn_lock locked
      LEFT JOIN bot_history_messages existing ON existing.turn_id = locked.id
    )
  END,
  sqlc.narg(sender_channel_identity_id)::uuid,
  sqlc.narg(sender_user_id)::uuid,
  sqlc.narg(external_message_id)::text,
  sqlc.narg(source_reply_to_message_id)::text,
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.narg(model_id)::uuid,
  sqlc.narg(event_id)::uuid,
  sqlc.narg(display_text)::text
)
RETURNING
  id,
  bot_id,
  session_id,
  branch_id,
  branch_seq,
  turn_id,
  turn_message_seq,
  sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  event_id,
  display_text,
  created_at;

-- name: CreateHistoryTurn :one
WITH branch_lock AS (
  SELECT id
  FROM bot_session_branches
  WHERE id = sqlc.arg(branch_id)
  FOR UPDATE
)
INSERT INTO bot_history_turns (
  session_id,
  branch_id,
  turn_seq,
  status
)
SELECT
  sqlc.arg(session_id),
  sqlc.arg(branch_id),
  (
    SELECT COALESCE(MAX(existing.turn_seq), 0) + 1
    FROM branch_lock locked
    LEFT JOIN bot_history_turns existing ON existing.branch_id = locked.id
  ),
  'running'
FROM branch_lock
RETURNING id;

-- name: GetHistoryTurnForMessagePersist :one
SELECT id
FROM bot_history_turns
WHERE id = sqlc.arg(turn_id)
  AND session_id = sqlc.arg(session_id)
  AND branch_id = sqlc.arg(branch_id)
LIMIT 1;

-- name: GetSessionBranchForPersist :one
SELECT id
FROM bot_session_branches
WHERE id = sqlc.arg(branch_id)
  AND session_id = sqlc.arg(session_id)
LIMIT 1;

-- name: CancelEmptyHistoryTurn :execrows
UPDATE bot_history_turns t
SET status = 'failed',
    completed_at = now(),
    updated_at = now()
WHERE t.id = sqlc.arg(turn_id)
  AND t.session_id = sqlc.arg(session_id)
  AND t.branch_id = sqlc.arg(branch_id)
  AND t.status = 'running'
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_messages m
    WHERE m.turn_id = t.id
  );

-- name: GetOpenHistoryTurnForBranch :one
SELECT id
FROM bot_history_turns
WHERE session_id = sqlc.arg(session_id)
  AND branch_id = sqlc.arg(branch_id)
  AND status = 'running'
ORDER BY turn_seq DESC, created_at DESC
LIMIT 1;

-- name: SetHistoryTurnRequestMessage :exec
UPDATE bot_history_turns
SET request_message_id = sqlc.arg(message_id),
    updated_at = now()
WHERE id = sqlc.arg(turn_id);

-- name: CompleteHistoryTurnWithAssistant :exec
UPDATE bot_history_turns
SET final_assistant_message_id = sqlc.arg(message_id),
    status = 'completed',
    completed_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(turn_id);

-- name: ListMessages :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at ASC
LIMIT 10000;

-- name: ListMessagesBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.created_at ASC;

-- name: ListMessagesSinceBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSinceBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSinceBySessionBranch :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = sqlc.arg(branch_id)
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSinceBySessionBranchTurn :many
WITH RECURSIVE pinned_turn AS (
  SELECT
    t.id,
    t.branch_id,
    t.turn_seq,
    (
      SELECT MAX(m.branch_seq)
      FROM bot_history_messages m
      WHERE m.turn_id = t.id
    ) AS max_branch_seq
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.branch_id = sqlc.arg(branch_id)
    AND t.id = sqlc.arg(turn_id)
  LIMIT 1
),
branch_path AS (
  SELECT b.id AS branch_id, b.parent_branch_id, pt.turn_seq AS max_turn_seq, pt.max_branch_seq AS max_branch_seq, 0 AS depth
  FROM pinned_turn pt
  JOIN bot_session_branches b ON b.id = pt.branch_id
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(child.fork_from_turn_seq, boundary_turn.turn_seq), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND EXISTS (SELECT 1 FROM pinned_turn)
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      (bp.depth = 0 AND bp.max_turn_seq IS NULL)
      OR (
        bp.max_turn_seq IS NOT NULL
        AND (
          t.turn_seq < bp.max_turn_seq
          OR (
            t.turn_seq = bp.max_turn_seq
            AND (bp.max_branch_seq IS NULL OR m.branch_seq <= bp.max_branch_seq)
          )
        )
      )
      OR (
        bp.max_turn_seq IS NULL
        AND bp.max_branch_seq IS NOT NULL
        AND m.branch_seq <= bp.max_branch_seq
      )
    )
  )
  )
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    (sqlc.narg(before_id)::uuid IS NULL AND m.created_at < sqlc.arg(created_at))
    OR (
      sqlc.narg(before_id)::uuid IS NOT NULL
      AND (
        bvm.depth > COALESCE((SELECT cursor_bvm.depth FROM bot_history_messages cursor JOIN bot_branch_visible_messages cursor_bvm ON cursor_bvm.message_id = cursor.id AND cursor_bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  )) WHERE cursor.id = sqlc.narg(before_id)::uuid), 2147483647)
        OR (
          bvm.depth = COALESCE((SELECT cursor_bvm.depth FROM bot_history_messages cursor JOIN bot_branch_visible_messages cursor_bvm ON cursor_bvm.message_id = cursor.id AND cursor_bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  )) WHERE cursor.id = sqlc.narg(before_id)::uuid), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) < COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id)::uuid), 9223372036854775807)
        )
        OR (
          bvm.depth = COALESCE((SELECT cursor_bvm.depth FROM bot_history_messages cursor JOIN bot_branch_visible_messages cursor_bvm ON cursor_bvm.message_id = cursor.id AND cursor_bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  )) WHERE cursor.id = sqlc.narg(before_id)::uuid), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) = COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id)::uuid), 9223372036854775807)
          AND (
            m.created_at < (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id)::uuid)
            OR (m.created_at = (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id)::uuid) AND m.id < sqlc.narg(before_id)::uuid)
          )
        )
      )
    )
  )
ORDER BY bvm.depth ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY bvm.depth ASC, COALESCE(m.branch_seq, -1) DESC, m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByExternalIDBySession :one
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC
LIMIT 1;

-- name: ListMessagesAfterBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    (sqlc.narg(after_id)::uuid IS NULL AND m.created_at > sqlc.arg(created_at))
    OR (
      sqlc.narg(after_id)::uuid IS NOT NULL
      AND (
        bvm.depth < COALESCE((SELECT cursor_bvm.depth FROM bot_history_messages cursor JOIN bot_branch_visible_messages cursor_bvm ON cursor_bvm.message_id = cursor.id AND cursor_bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  )) WHERE cursor.id = sqlc.narg(after_id)::uuid), 2147483647)
        OR (
          bvm.depth = COALESCE((SELECT cursor_bvm.depth FROM bot_history_messages cursor JOIN bot_branch_visible_messages cursor_bvm ON cursor_bvm.message_id = cursor.id AND cursor_bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  )) WHERE cursor.id = sqlc.narg(after_id)::uuid), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) > COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id)::uuid), 9223372036854775807)
        )
        OR (
          bvm.depth = COALESCE((SELECT cursor_bvm.depth FROM bot_history_messages cursor JOIN bot_branch_visible_messages cursor_bvm ON cursor_bvm.message_id = cursor.id AND cursor_bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  )) WHERE cursor.id = sqlc.narg(after_id)::uuid), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) = COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id)::uuid), 9223372036854775807)
          AND (
            m.created_at > (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id)::uuid)
            OR (m.created_at = (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id)::uuid) AND m.id > sqlc.narg(after_id)::uuid)
          )
        )
      )
    )
  )
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: CountMessagesByBot :one
SELECT COUNT(*) FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesByBot :exec
DELETE FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesBySession :exec
DELETE FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id);

-- name: ListObservedConversationsByChannelIdentity :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.sender_channel_identity_id = sqlc.arg(channel_identity_id)::uuid
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_name', '')), ''),
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_handle', '')), ''),
    ''
  )::text AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(r.metadata->>'conversation_avatar_url', '')), ''), '')::text AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
GROUP BY
  r.id,
  r.channel_type,
  r.conversation_type,
  r.external_conversation_id,
  r.external_thread_id,
  r.metadata,
  rr.last_observed_at
ORDER BY rr.last_observed_at DESC;

-- name: ListObservedConversationsByChannelType :many
-- Routes on this platform type where the bot has seen at least one message (any sender).
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  JOIN bot_channel_routes r ON r.id = s.route_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND LOWER(TRIM(r.channel_type)) = LOWER(TRIM(sqlc.arg(channel_type)))
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN LOWER(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_name', '')), ''),
    NULLIF(TRIM(COALESCE(r.metadata->>'conversation_handle', '')), ''),
    ''
  )::text AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(r.metadata->>'conversation_avatar_url', '')), ''), '')::text AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
GROUP BY
  r.id,
  r.channel_type,
  r.conversation_type,
  r.external_conversation_id,
  r.external_thread_id,
  r.metadata,
  rr.last_observed_at
ORDER BY rr.last_observed_at DESC;

-- name: SearchMessages :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
  m.sender_channel_identity_id,
  m.role,
  m.content,
  m.created_at,
  ci.display_name AS sender_display_name,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id)::uuid IS NULL OR m.session_id = sqlc.narg(session_id)::uuid)
  AND (sqlc.narg(contact_id)::uuid IS NULL OR m.sender_channel_identity_id = sqlc.narg(contact_id)::uuid)
  AND (sqlc.narg(start_time)::timestamptz IS NULL OR m.created_at >= sqlc.narg(start_time)::timestamptz)
  AND (sqlc.narg(end_time)::timestamptz IS NULL OR m.created_at <= sqlc.narg(end_time)::timestamptz)
  AND (sqlc.narg(role)::text IS NULL OR m.role = sqlc.narg(role)::text)
  AND (sqlc.narg(keyword)::text IS NULL OR (
    CASE
      WHEN jsonb_typeof(m.content->'content') = 'string'
        THEN m.content->>'content'
      WHEN jsonb_typeof(m.content->'content') = 'array'
        THEN (SELECT COALESCE(string_agg(elem->>'text', ' '), '')
              FROM jsonb_array_elements(m.content->'content') AS elem
              WHERE elem->>'type' = 'text')
      ELSE ''
    END
  ) ILIKE '%' || sqlc.narg(keyword)::text || '%')
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = $1
WHERE id = ANY($2::uuid[]);

-- name: ListUncompactedMessagesBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
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
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
JOIN bot_branch_visible_messages bvm ON bvm.message_id = m.id
  AND bvm.branch_id = COALESCE(s.active_branch_id, (
    SELECT rb.id
    FROM bot_session_branches rb
    WHERE rb.session_id = s.id
      AND rb.parent_branch_id IS NULL
      AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC
    LIMIT 1
  ))
WHERE m.session_id = sqlc.arg(session_id)
  AND m.compact_id IS NULL
ORDER BY bvm.depth DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: GetActiveSessionBranch :one
SELECT active_branch_id
FROM bot_sessions
WHERE id = sqlc.arg(session_id);

-- name: SetActiveSessionBranch :execrows
UPDATE bot_sessions
SET active_branch_id = sqlc.arg(branch_id),
    updated_at = now()
WHERE bot_sessions.id = sqlc.arg(session_id)
  AND EXISTS (
    SELECT 1
    FROM bot_session_branches b
    WHERE b.id = sqlc.arg(branch_id)
      AND b.session_id = sqlc.arg(session_id)
  );

-- name: GetRootSessionBranch :one
SELECT id
FROM bot_session_branches
WHERE session_id = sqlc.arg(session_id)
  AND parent_branch_id IS NULL
  AND fork_from_message_id IS NULL
ORDER BY created_at ASC
LIMIT 1;

-- name: CreateRootSessionBranch :one
INSERT INTO bot_session_branches (session_id)
VALUES (sqlc.arg(session_id))
RETURNING id;

-- name: GetMessageForSessionBranchFork :one
SELECT
  m.id,
  m.session_id,
  m.branch_id,
  m.branch_seq,
  m.turn_id,
  t.turn_seq,
  (
    SELECT pt.id
    FROM bot_history_turns pt
    WHERE pt.branch_id = t.branch_id
      AND pt.turn_seq < t.turn_seq
      AND pt.status = 'completed'
    ORDER BY pt.turn_seq DESC
    LIMIT 1
  ) AS previous_turn_id,
  COALESCE((
    SELECT pt.turn_seq
    FROM bot_history_turns pt
    WHERE pt.branch_id = t.branch_id
      AND pt.turn_seq < t.turn_seq
      AND pt.status = 'completed'
    ORDER BY pt.turn_seq DESC
    LIMIT 1
  ), 0)::BIGINT AS previous_turn_seq,
  (
    SELECT pm.branch_seq
    FROM bot_history_turns pt
    JOIN bot_history_messages pm ON pm.id = pt.final_assistant_message_id
    WHERE pt.branch_id = t.branch_id
      AND pt.turn_seq < t.turn_seq
      AND pt.status = 'completed'
    ORDER BY pt.turn_seq DESC
    LIMIT 1
  ) AS previous_branch_seq,
  m.role,
  m.created_at
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.id = sqlc.arg(message_id)
  AND m.session_id = sqlc.arg(session_id)
LIMIT 1;

-- name: CreateSessionBranchFromMessage :one
INSERT INTO bot_session_branches (
  session_id,
  parent_branch_id,
  fork_from_message_id,
  fork_from_seq,
  fork_from_turn_id,
  fork_from_turn_seq,
  title
)
VALUES (
  sqlc.arg(session_id),
  sqlc.arg(parent_branch_id),
  sqlc.arg(fork_from_message_id),
  sqlc.arg(fork_from_seq),
  sqlc.arg(fork_from_turn_id),
  sqlc.arg(fork_from_turn_seq),
  sqlc.narg(title)
)
RETURNING id;

-- name: DeleteSessionBranch :exec
DELETE FROM bot_session_branches
WHERE id = sqlc.arg(branch_id)
  AND session_id = sqlc.arg(session_id);

-- name: ListSessionBranches :many
SELECT
  b.id,
  b.session_id,
  b.parent_branch_id,
  b.fork_from_message_id,
  b.fork_from_seq,
  b.fork_from_turn_id,
  b.fork_from_turn_seq,
  b.title,
  b.created_at,
  b.updated_at,
  s.active_branch_id
FROM bot_session_branches b
JOIN bot_sessions s ON s.id = b.session_id
WHERE b.session_id = sqlc.arg(session_id)
ORDER BY b.created_at ASC, b.id ASC;

-- name: ListSessionBranchPreviewMessages :many
WITH first_turns AS (
  SELECT DISTINCT ON (t.branch_id)
    t.branch_id,
    t.request_message_id,
    t.final_assistant_message_id
  FROM bot_history_turns t
  JOIN bot_session_branches b ON b.id = t.branch_id
  WHERE b.session_id = sqlc.arg(session_id)
    AND t.status = 'completed'
    AND t.request_message_id IS NOT NULL
    AND t.final_assistant_message_id IS NOT NULL
  ORDER BY t.branch_id, t.turn_seq ASC, t.created_at ASC
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  ft.branch_id AS branch_id,
  m.branch_seq,
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
FROM first_turns ft
JOIN bot_history_messages m ON m.id IN (ft.request_message_id, ft.final_assistant_message_id)
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
ORDER BY m.branch_id, m.branch_seq ASC, m.created_at ASC;

-- name: ListSessionBranchTurnMessages :many
SELECT
  t.id AS turn_id,
  t.turn_seq,
  t.title,
  a.id AS assistant_id,
  a.bot_id,
  a.session_id,
  a.branch_id,
  a.branch_seq,
  a.content AS assistant_content,
  a.display_text AS assistant_display_text,
  a.created_at AS assistant_created_at,
  u.id AS user_id,
  u.content AS user_content,
  u.display_text AS user_display_text,
  u.created_at AS user_created_at
FROM bot_history_turns t
JOIN bot_session_branches b ON b.id = t.branch_id
JOIN bot_history_messages a ON a.id = t.final_assistant_message_id
LEFT JOIN bot_history_messages u ON u.id = t.request_message_id
WHERE b.session_id = sqlc.arg(session_id)
  AND t.status = 'completed'
  AND t.final_assistant_message_id IS NOT NULL
ORDER BY b.created_at ASC, t.turn_seq ASC, t.created_at ASC;
