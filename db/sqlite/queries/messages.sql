-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, sender_channel_identity_id, sender_account_user_id,
  source_message_id, source_reply_to_message_id, role, content, metadata,
  usage, model_id, event_id, display_text
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(bot_id),
  sqlc.narg(session_id),
  sqlc.narg(branch_id),
  CASE
    WHEN sqlc.narg(branch_id) IS NULL THEN NULL
    ELSE (
      SELECT COALESCE(MAX(existing.branch_seq), 0) + 1
      FROM bot_history_messages existing
      WHERE existing.branch_id = sqlc.narg(branch_id)
    )
  END,
  sqlc.narg(turn_id),
  CASE
    WHEN sqlc.narg(turn_id) IS NULL THEN NULL
    ELSE (
      SELECT COALESCE(MAX(existing.turn_message_seq), 0) + 1
      FROM bot_history_messages existing
      WHERE existing.turn_id = sqlc.narg(turn_id)
    )
  END,
  sqlc.narg(sender_channel_identity_id),
  sqlc.narg(sender_user_id),
  sqlc.narg(external_message_id),
  sqlc.narg(source_reply_to_message_id),
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.narg(model_id),
  sqlc.narg(event_id),
  sqlc.narg(display_text)
)
RETURNING
  id, bot_id, session_id, branch_id, branch_seq, turn_id, turn_message_seq, sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id, role, content, metadata, usage,
  event_id, display_text, created_at;

-- name: CreateHistoryTurn :one
INSERT INTO bot_history_turns (
  id,
  session_id,
  branch_id,
  turn_seq,
  status
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(session_id),
  sqlc.arg(branch_id),
  (
    SELECT COALESCE(MAX(existing.turn_seq), 0) + 1
    FROM bot_history_turns existing
    WHERE existing.branch_id = sqlc.arg(branch_id)
  ),
  'running'
)
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
UPDATE bot_history_turns
SET status = 'failed',
    completed_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE bot_history_turns.id = sqlc.arg(turn_id)
  AND bot_history_turns.session_id = sqlc.arg(session_id)
  AND bot_history_turns.branch_id = sqlc.arg(branch_id)
  AND bot_history_turns.status = 'running'
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_messages m
    WHERE m.turn_id = bot_history_turns.id
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
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(turn_id);

-- name: CompleteHistoryTurnWithAssistant :exec
UPDATE bot_history_turns
SET final_assistant_message_id = sqlc.arg(message_id),
    status = 'completed',
    completed_at = CURRENT_TIMESTAMP,
    updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(turn_id);

-- name: ListMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
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
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT
    b.id,
    b.parent_branch_id,
    NULL,
    NULL,
    0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(
    s.active_branch_id,
    (
      SELECT rb.id
      FROM bot_session_branches rb
      WHERE rb.session_id = s.id
        AND rb.parent_branch_id IS NULL
        AND rb.fork_from_message_id IS NULL
      ORDER BY rb.created_at ASC
      LIMIT 1
    )
  )
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
ORDER BY m.created_at ASC;

-- name: ListMessagesSinceBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND COALESCE(
    CASE WHEN json_valid(m.metadata) THEN json_extract(m.metadata, '$.trigger_mode') END,
    ''
  ) != 'passive_sync'
ORDER BY m.created_at ASC;

-- name: ListActiveMessagesSinceBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
  AND COALESCE(
    CASE WHEN json_valid(m.metadata) THEN json_extract(m.metadata, '$.trigger_mode') END,
    ''
  ) != 'passive_sync'
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSinceBySessionBranch :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_session_branches b
  WHERE b.session_id = sqlc.arg(session_id)
    AND b.id = sqlc.arg(branch_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
  AND COALESCE(
    CASE WHEN json_valid(m.metadata) THEN json_extract(m.metadata, '$.trigger_mode') END,
    ''
  ) != 'passive_sync'
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListActiveMessagesSinceBySessionBranchTurn :many
WITH RECURSIVE pinned_turn(id, branch_id, turn_seq) AS (
  SELECT
    t.id,
    t.branch_id,
    t.turn_seq
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.branch_id = sqlc.arg(branch_id)
    AND t.id = sqlc.arg(turn_id)
  LIMIT 1
),
branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, pt.turn_seq, (
    SELECT MAX(m.branch_seq)
    FROM bot_history_messages m
    WHERE m.turn_id = pt.id
  ), 0
  FROM pinned_turn pt
  JOIN bot_session_branches b ON b.id = pt.branch_id
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.compact_id, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
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
  AND COALESCE(
    CASE WHEN json_valid(m.metadata) THEN json_extract(m.metadata, '$.trigger_mode') END,
    ''
  ) != 'passive_sync'
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at))
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    (sqlc.narg(before_id) IS NULL AND m.created_at < strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at)))
    OR (
      sqlc.narg(before_id) IS NOT NULL
      AND (
        COALESCE(bp.depth, 2147483647) > COALESCE((SELECT cursor_bp.depth FROM bot_history_messages cursor LEFT JOIN branch_path cursor_bp ON cursor_bp.branch_id = cursor.branch_id WHERE cursor.id = sqlc.narg(before_id)), 2147483647)
        OR (
          COALESCE(bp.depth, 2147483647) = COALESCE((SELECT cursor_bp.depth FROM bot_history_messages cursor LEFT JOIN branch_path cursor_bp ON cursor_bp.branch_id = cursor.branch_id WHERE cursor.id = sqlc.narg(before_id)), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) < COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id)), 9223372036854775807)
        )
        OR (
          COALESCE(bp.depth, 2147483647) = COALESCE((SELECT cursor_bp.depth FROM bot_history_messages cursor LEFT JOIN branch_path cursor_bp ON cursor_bp.branch_id = cursor.branch_id WHERE cursor.id = sqlc.narg(before_id)), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) = COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id)), 9223372036854775807)
          AND (
            m.created_at < (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id))
            OR (
              m.created_at = (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(before_id))
              AND m.id < sqlc.narg(before_id)
            )
          )
        )
      )
    )
  )
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, -1) ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
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
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, -1) ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByExternalIDBySession :one
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, -1) ASC, COALESCE(m.branch_seq, 0) DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: ListMessagesAfterBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  m.id, m.bot_id, m.session_id, m.branch_id, m.branch_seq, m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id, m.role, m.content, m.metadata, m.usage,
  m.event_id, m.display_text, m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (
    (sqlc.narg(after_id) IS NULL AND m.created_at > strftime('%Y-%m-%d %H:%M:%S', sqlc.arg(created_at)))
    OR (
      sqlc.narg(after_id) IS NOT NULL
      AND (
        COALESCE(bp.depth, 2147483647) < COALESCE((SELECT cursor_bp.depth FROM bot_history_messages cursor LEFT JOIN branch_path cursor_bp ON cursor_bp.branch_id = cursor.branch_id WHERE cursor.id = sqlc.narg(after_id)), 2147483647)
        OR (
          COALESCE(bp.depth, 2147483647) = COALESCE((SELECT cursor_bp.depth FROM bot_history_messages cursor LEFT JOIN branch_path cursor_bp ON cursor_bp.branch_id = cursor.branch_id WHERE cursor.id = sqlc.narg(after_id)), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) > COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id)), 9223372036854775807)
        )
        OR (
          COALESCE(bp.depth, 2147483647) = COALESCE((SELECT cursor_bp.depth FROM bot_history_messages cursor LEFT JOIN branch_path cursor_bp ON cursor_bp.branch_id = cursor.branch_id WHERE cursor.id = sqlc.narg(after_id)), 2147483647)
          AND COALESCE(m.branch_seq, 9223372036854775807) = COALESCE((SELECT cursor.branch_seq FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id)), 9223372036854775807)
          AND (
            m.created_at > (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id))
            OR (
              m.created_at = (SELECT cursor.created_at FROM bot_history_messages cursor WHERE cursor.id = sqlc.narg(after_id))
              AND m.id > sqlc.narg(after_id)
            )
          )
        )
      )
    )
  )
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC, m.id ASC
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
    MAX(m.created_at) AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND m.sender_channel_identity_id = sqlc.arg(channel_identity_id)
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(CASE WHEN json_valid(r.metadata) THEN json_extract(r.metadata, '$.conversation_name') END, '')), ''),
    NULLIF(TRIM(COALESCE(CASE WHEN json_valid(r.metadata) THEN json_extract(r.metadata, '$.conversation_handle') END, '')), ''),
    ''
  ) AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(CASE WHEN json_valid(r.metadata) THEN json_extract(r.metadata, '$.conversation_avatar_url') END, '')), ''), '') AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
ORDER BY rr.last_observed_at DESC;

-- name: ListObservedConversationsByChannelType :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at) AS last_observed_at
  FROM bot_history_messages m
  JOIN bot_sessions s ON s.id = m.session_id
  JOIN bot_channel_routes r ON r.id = s.route_id
  WHERE m.bot_id = sqlc.arg(bot_id)
    AND lower(TRIM(r.channel_type)) = lower(TRIM(sqlc.arg(channel_type)))
    AND s.route_id IS NOT NULL
  GROUP BY s.route_id
)
SELECT
  r.id AS route_id,
  r.channel_type AS channel,
  CASE
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('thread', 'topic') THEN 'thread'
    WHEN lower(COALESCE(r.conversation_type, '')) IN ('p2p', 'private', 'direct', 'dm') THEN 'private'
    ELSE 'group'
  END AS conversation_type,
  r.external_conversation_id AS conversation_id,
  COALESCE(r.external_thread_id, '') AS thread_id,
  COALESCE(
    NULLIF(TRIM(COALESCE(CASE WHEN json_valid(r.metadata) THEN json_extract(r.metadata, '$.conversation_name') END, '')), ''),
    NULLIF(TRIM(COALESCE(CASE WHEN json_valid(r.metadata) THEN json_extract(r.metadata, '$.conversation_handle') END, '')), ''),
    ''
  ) AS conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(CASE WHEN json_valid(r.metadata) THEN json_extract(r.metadata, '$.conversation_avatar_url') END, '')), ''), '') AS conversation_avatar_url,
  rr.last_observed_at
FROM observed_routes rr
JOIN bot_channel_routes r ON r.id = rr.route_id
ORDER BY rr.last_observed_at DESC;

-- name: SearchMessages :many
SELECT
  m.id, m.bot_id, m.session_id, m.sender_channel_identity_id,
  m.branch_id, m.branch_seq,
  m.role, m.content, m.created_at,
  ci.display_name AS sender_display_name,
  s.channel_type AS platform
FROM bot_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND (sqlc.narg(session_id) IS NULL OR m.session_id = sqlc.narg(session_id))
  AND (sqlc.narg(contact_id) IS NULL OR m.sender_channel_identity_id = sqlc.narg(contact_id))
  AND (sqlc.narg(start_time) IS NULL OR m.created_at >= strftime('%Y-%m-%d %H:%M:%S', sqlc.narg(start_time)))
  AND (sqlc.narg(end_time) IS NULL OR m.created_at <= strftime('%Y-%m-%d %H:%M:%S', sqlc.narg(end_time)))
  AND (sqlc.narg(role) IS NULL OR m.role = sqlc.narg(role))
  AND (sqlc.narg(keyword) IS NULL OR (
    CASE
      WHEN NOT json_valid(m.content)
        THEN CASE WHEN m.content LIKE '%' || sqlc.narg(keyword) || '%' THEN m.content ELSE '' END
      WHEN json_valid(m.content) AND json_type(m.content, '$.content') = 'text'
        THEN json_extract(m.content, '$.content')
      WHEN json_valid(m.content) AND json_type(m.content, '$.content') = 'array'
        THEN (SELECT COALESCE(group_concat(json_extract(j.value, '$.text'), ' '), '')
              FROM json_each(
                CASE
                  WHEN json_valid(m.content) AND json_type(m.content, '$.content') = 'array'
                    THEN json_extract(m.content, '$.content')
                  ELSE '[]'
                END
              ) AS j
              WHERE json_valid(j.value) AND json_extract(j.value, '$.type') = 'text')
      ELSE ''
    END
  ) LIKE '%' || sqlc.narg(keyword) || '%')
ORDER BY m.created_at DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = sqlc.arg(compact_id)
WHERE id IN (sqlc.slice(ids));

-- name: ListUncompactedMessagesBySession :many
WITH RECURSIVE branch_path(branch_id, parent_branch_id, max_turn_seq, max_branch_seq, depth) AS (
  SELECT b.id, b.parent_branch_id, NULL, NULL, 0
  FROM bot_sessions s
  JOIN bot_session_branches b ON b.id = COALESCE(s.active_branch_id, (
    SELECT rb.id FROM bot_session_branches rb
    WHERE rb.session_id = s.id AND rb.parent_branch_id IS NULL AND rb.fork_from_message_id IS NULL
    ORDER BY rb.created_at ASC LIMIT 1
  ))
  WHERE s.id = sqlc.arg(session_id)
  UNION ALL
  SELECT parent.id, parent.parent_branch_id, COALESCE(
    CASE
      WHEN typeof(child.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(child.fork_from_turn_seq AS INTEGER)
      WHEN typeof(child.fork_from_turn_seq) = 'text'
        AND child.fork_from_turn_seq != ''
        AND child.fork_from_turn_seq NOT GLOB '*[^0-9]*'
        THEN CAST(child.fork_from_turn_seq AS INTEGER)
      ELSE NULL
    END,
    boundary_turn.turn_seq
  ), child.fork_from_seq, bp.depth + 1
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT m.id, m.bot_id, m.session_id, m.role, m.content, m.usage, m.sender_channel_identity_id, m.compact_id, m.created_at
FROM bot_history_messages m
LEFT JOIN branch_path bp ON bp.branch_id = m.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.compact_id IS NULL
  AND (
    m.branch_id IS NULL
    OR (
    bp.branch_id IS NOT NULL
    AND (
      bp.depth = 0
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
ORDER BY COALESCE(bp.depth, 2147483647) DESC, COALESCE(m.branch_seq, 9223372036854775807) ASC, m.created_at ASC;


-- name: GetActiveSessionBranch :one
SELECT active_branch_id
FROM bot_sessions
WHERE id = sqlc.arg(session_id);

-- name: SetActiveSessionBranch :execrows
UPDATE bot_sessions
SET active_branch_id = sqlc.arg(branch_id),
    updated_at = CURRENT_TIMESTAMP
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
INSERT INTO bot_session_branches (id, session_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(session_id)
)
RETURNING id;

-- name: CreateSessionBranchFromMessage :one
INSERT INTO bot_session_branches (
  id,
  session_id,
  parent_branch_id,
  fork_from_message_id,
  fork_from_seq,
  fork_from_turn_id,
  fork_from_turn_seq,
  title
)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
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
  CASE
    WHEN b.fork_from_turn_id LIKE '____-__-__ __:__:__' THEN NULL
    ELSE b.fork_from_turn_id
  END AS fork_from_turn_id,
  CASE
    WHEN typeof(b.fork_from_turn_seq) IN ('integer', 'real') THEN CAST(b.fork_from_turn_seq AS INTEGER)
    WHEN typeof(b.fork_from_turn_seq) = 'text'
      AND b.fork_from_turn_seq != ''
      AND b.fork_from_turn_seq NOT GLOB '*[^0-9]*'
      THEN CAST(b.fork_from_turn_seq AS INTEGER)
    ELSE NULL
  END AS fork_from_turn_seq,
  b.title,
  COALESCE(NULLIF(b.created_at, ''), CURRENT_TIMESTAMP) AS created_at,
  COALESCE(NULLIF(b.updated_at, ''), NULLIF(b.created_at, ''), CURRENT_TIMESTAMP) AS updated_at,
  s.active_branch_id
FROM bot_session_branches b
JOIN bot_sessions s ON s.id = b.session_id
WHERE b.session_id = sqlc.arg(session_id)
ORDER BY b.created_at ASC, b.id ASC;

-- name: ListSessionBranchPreviewMessages :many
WITH first_turns AS (
  SELECT
    t.branch_id,
    t.request_message_id,
    t.final_assistant_message_id
  FROM bot_history_turns t
  JOIN bot_session_branches b ON b.id = t.branch_id
  WHERE b.session_id = sqlc.arg(session_id)
    AND t.status = 'completed'
    AND t.request_message_id IS NOT NULL
    AND t.final_assistant_message_id IS NOT NULL
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns earlier
      WHERE earlier.branch_id = t.branch_id
        AND earlier.status = 'completed'
        AND earlier.request_message_id IS NOT NULL
        AND earlier.final_assistant_message_id IS NOT NULL
        AND (earlier.turn_seq < t.turn_seq OR (earlier.turn_seq = t.turn_seq AND earlier.created_at < t.created_at))
    )
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
