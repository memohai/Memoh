-- name: CreateMessage :one
INSERT INTO bot_history_messages (
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  event_id,
  display_text
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.narg(session_id)::uuid,
  sqlc.narg(sender_channel_identity_id)::uuid,
  sqlc.narg(sender_user_id)::uuid,
  sqlc.narg(external_message_id)::text,
  sqlc.narg(source_reply_to_message_id)::text,
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.narg(model_id)::uuid,
  sqlc.narg(event_id)::uuid,
  sqlc.narg(display_text)::text
)
RETURNING
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  event_id,
  display_text,
  created_at;

-- name: CreateMessageWithTurn :one
INSERT INTO bot_history_messages (
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id,
  source_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  event_id,
  display_text,
  turn_id,
  turn_message_seq
)
VALUES (
  sqlc.arg(message_id),
  sqlc.arg(bot_id),
  sqlc.narg(session_id)::uuid,
  sqlc.narg(sender_channel_identity_id)::uuid,
  sqlc.narg(sender_user_id)::uuid,
  sqlc.narg(external_message_id)::text,
  sqlc.narg(source_reply_to_message_id)::text,
  sqlc.arg(role),
  sqlc.arg(content),
  sqlc.arg(metadata),
  sqlc.arg(usage),
  sqlc.arg(session_mode),
  sqlc.arg(runtime_type),
  sqlc.narg(model_id)::uuid,
  sqlc.narg(event_id)::uuid,
  sqlc.narg(display_text)::text,
  sqlc.arg(turn_id),
  sqlc.arg(turn_message_seq)
)
RETURNING
  id,
  bot_id,
  session_id,
  sender_channel_identity_id,
  sender_account_user_id AS sender_user_id,
  source_message_id AS external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  event_id,
  display_text,
  created_at;

-- name: CreateMessageWithHistoryTurn :one
WITH inserted_message AS (
  INSERT INTO bot_history_messages (
    id,
    bot_id,
    session_id,
    sender_channel_identity_id,
    sender_account_user_id,
    source_message_id,
    source_reply_to_message_id,
    role,
    content,
    metadata,
    usage,
    session_mode,
    runtime_type,
    model_id,
    event_id,
    display_text,
    turn_id,
    turn_message_seq
  )
  VALUES (
    sqlc.arg(message_id),
    sqlc.arg(bot_id),
    sqlc.arg(session_id),
    sqlc.narg(sender_channel_identity_id)::uuid,
    sqlc.narg(sender_user_id)::uuid,
    sqlc.narg(external_message_id)::text,
    sqlc.narg(source_reply_to_message_id)::text,
    sqlc.arg(role),
    sqlc.arg(content),
    sqlc.arg(metadata),
    sqlc.arg(usage),
    sqlc.arg(session_mode),
    sqlc.arg(runtime_type),
    sqlc.narg(model_id)::uuid,
    sqlc.narg(event_id)::uuid,
    sqlc.narg(display_text)::text,
    sqlc.arg(turn_id),
    sqlc.arg(turn_message_seq)
  )
  RETURNING
    id,
    created_at
),
inserted_turn AS (
  INSERT INTO bot_history_turns (
    id,
    bot_id,
    session_id,
    position,
    request_message_id,
    assistant_message_id
  )
  SELECT
    sqlc.arg(turn_id),
    sqlc.arg(bot_id),
    sqlc.arg(session_id),
    COALESCE((
      SELECT position + 1
      FROM bot_history_turns
      WHERE session_id = sqlc.arg(session_id)
      ORDER BY position DESC
      LIMIT 1
    ), 1),
    inserted_message.id,
    NULL::uuid
  FROM inserted_message
  RETURNING id
)
SELECT
  inserted_message.id,
  inserted_message.created_at
FROM inserted_message
JOIN inserted_turn ON true;

-- name: CreateMessageInHistoryTurnByRequest :one
WITH target AS (
  SELECT
    turns.id,
    turns.session_id,
    turns.assistant_message_id,
    CASE
      WHEN sqlc.arg(role)::text = 'assistant' AND turns.assistant_message_id IS NULL THEN 2
      ELSE COALESCE((
        SELECT existing.turn_message_seq + 1
        FROM bot_history_messages existing
        WHERE existing.turn_id = turns.id
        ORDER BY existing.turn_message_seq DESC
        LIMIT 1
      ), 1)
    END AS turn_message_seq
  FROM bot_history_turns turns
  WHERE turns.session_id = sqlc.arg(session_id)
    AND turns.request_message_id = sqlc.arg(request_message_id)
    AND turns.superseded_at IS NULL
  LIMIT 1
),
inserted AS (
  INSERT INTO bot_history_messages (
    bot_id,
    session_id,
    sender_channel_identity_id,
    sender_account_user_id,
    source_message_id,
    source_reply_to_message_id,
    role,
    content,
    metadata,
    usage,
    session_mode,
    runtime_type,
    model_id,
    event_id,
    display_text,
    turn_id,
    turn_message_seq
  )
  SELECT
    sqlc.arg(bot_id),
    target.session_id,
    sqlc.narg(sender_channel_identity_id)::uuid,
    sqlc.narg(sender_user_id)::uuid,
    sqlc.narg(external_message_id)::text,
    sqlc.narg(source_reply_to_message_id)::text,
    sqlc.arg(role)::text,
    sqlc.arg(content),
    sqlc.arg(metadata),
    sqlc.arg(usage),
    sqlc.arg(session_mode),
    sqlc.arg(runtime_type),
    sqlc.narg(model_id)::uuid,
    sqlc.narg(event_id)::uuid,
    sqlc.narg(display_text)::text,
    target.id,
    target.turn_message_seq
  FROM target
  RETURNING
    id,
    bot_id,
    session_id,
    sender_channel_identity_id,
    sender_account_user_id AS sender_user_id,
    source_message_id AS external_message_id,
    source_reply_to_message_id,
    role,
    content,
    metadata,
    usage,
    session_mode,
    runtime_type,
    event_id,
    display_text,
    created_at
)
SELECT
  inserted.id,
  inserted.bot_id,
  inserted.session_id,
  inserted.sender_channel_identity_id,
  inserted.sender_user_id,
  inserted.external_message_id,
  inserted.source_reply_to_message_id,
  inserted.role,
  inserted.content,
  inserted.metadata,
  inserted.usage,
  inserted.session_mode,
  inserted.runtime_type,
  inserted.event_id,
  inserted.display_text,
  inserted.created_at
FROM inserted;

-- name: CreateMessageInHistoryTurnByRequestAndBind :one
WITH target AS (
  SELECT
    turns.id,
    turns.session_id,
    turns.assistant_message_id,
    CASE
      WHEN sqlc.arg(role)::text = 'assistant' AND turns.assistant_message_id IS NULL THEN 2
      ELSE COALESCE((
        SELECT existing.turn_message_seq + 1
        FROM bot_history_messages existing
        WHERE existing.turn_id = turns.id
        ORDER BY existing.turn_message_seq DESC
        LIMIT 1
      ), 1)
    END AS turn_message_seq
  FROM bot_history_turns turns
  WHERE turns.session_id = sqlc.arg(session_id)
    AND turns.request_message_id = sqlc.arg(request_message_id)
    AND turns.superseded_at IS NULL
  LIMIT 1
),
inserted AS (
  INSERT INTO bot_history_messages (
    bot_id,
    session_id,
    sender_channel_identity_id,
    sender_account_user_id,
    source_message_id,
    source_reply_to_message_id,
    role,
    content,
    metadata,
    usage,
    session_mode,
    runtime_type,
    model_id,
    event_id,
    display_text,
    turn_id,
    turn_message_seq
  )
  SELECT
    sqlc.arg(bot_id),
    target.session_id,
    sqlc.narg(sender_channel_identity_id)::uuid,
    sqlc.narg(sender_user_id)::uuid,
    sqlc.narg(external_message_id)::text,
    sqlc.narg(source_reply_to_message_id)::text,
    sqlc.arg(role)::text,
    sqlc.arg(content),
    sqlc.arg(metadata),
    sqlc.arg(usage),
    sqlc.arg(session_mode),
    sqlc.arg(runtime_type),
    sqlc.narg(model_id)::uuid,
    sqlc.narg(event_id)::uuid,
    sqlc.narg(display_text)::text,
    target.id,
    target.turn_message_seq
  FROM target
  RETURNING
    id,
    role,
    created_at
),
bound AS (
  UPDATE bot_history_turns turns
  SET assistant_message_id = inserted.id,
      updated_at = now()
  FROM inserted, target
  WHERE turns.id = target.id
    AND inserted.role = 'assistant'
    AND target.assistant_message_id IS NULL
    AND turns.assistant_message_id IS NULL
    AND turns.superseded_at IS NULL
  RETURNING turns.id
)
SELECT
  inserted.id,
  inserted.created_at
FROM inserted
LEFT JOIN bound ON true;

-- name: CreateToolTailRound :many
WITH input_rows(
  turn_message_seq,
  message_id,
  sender_channel_identity_id,
  sender_user_id,
  external_message_id,
  source_reply_to_message_id,
  role,
  content,
  metadata,
  usage,
  session_mode,
  runtime_type,
  model_id,
  event_id,
  display_text
) AS (
  VALUES
      (
        1::bigint,
        sqlc.arg(user_message_id)::uuid,
        sqlc.narg(user_sender_channel_identity_id)::uuid,
        sqlc.narg(user_sender_user_id)::uuid,
        sqlc.narg(user_external_message_id)::text,
        sqlc.narg(user_source_reply_to_message_id)::text,
        'user'::text,
        sqlc.arg(user_content)::jsonb,
        sqlc.arg(user_metadata)::jsonb,
        sqlc.arg(user_usage)::jsonb,
        sqlc.arg(user_session_mode)::text,
        sqlc.arg(user_runtime_type)::text,
        sqlc.narg(user_model_id)::uuid,
        sqlc.narg(user_event_id)::uuid,
        sqlc.narg(user_display_text)::text
      ),
      (
        2::bigint,
        sqlc.arg(tool_call_assistant_message_id)::uuid,
        sqlc.narg(tool_call_assistant_sender_channel_identity_id)::uuid,
        sqlc.narg(tool_call_assistant_sender_user_id)::uuid,
        sqlc.narg(tool_call_assistant_external_message_id)::text,
        sqlc.narg(tool_call_assistant_source_reply_to_message_id)::text,
        'assistant'::text,
        sqlc.arg(tool_call_assistant_content)::jsonb,
        sqlc.arg(tool_call_assistant_metadata)::jsonb,
        sqlc.arg(tool_call_assistant_usage)::jsonb,
        sqlc.arg(tool_call_assistant_session_mode)::text,
        sqlc.arg(tool_call_assistant_runtime_type)::text,
        sqlc.narg(tool_call_assistant_model_id)::uuid,
        sqlc.narg(tool_call_assistant_event_id)::uuid,
        sqlc.narg(tool_call_assistant_display_text)::text
      ),
      (
        3::bigint,
        sqlc.arg(tool_message_id)::uuid,
        sqlc.narg(tool_sender_channel_identity_id)::uuid,
        sqlc.narg(tool_sender_user_id)::uuid,
        sqlc.narg(tool_external_message_id)::text,
        sqlc.narg(tool_source_reply_to_message_id)::text,
        'tool'::text,
        sqlc.arg(tool_content)::jsonb,
        sqlc.arg(tool_metadata)::jsonb,
        sqlc.arg(tool_usage)::jsonb,
        sqlc.arg(tool_session_mode)::text,
        sqlc.arg(tool_runtime_type)::text,
        sqlc.narg(tool_model_id)::uuid,
        sqlc.narg(tool_event_id)::uuid,
        sqlc.narg(tool_display_text)::text
      ),
      (
        4::bigint,
        sqlc.arg(final_assistant_message_id)::uuid,
        sqlc.narg(final_assistant_sender_channel_identity_id)::uuid,
        sqlc.narg(final_assistant_sender_user_id)::uuid,
        sqlc.narg(final_assistant_external_message_id)::text,
        sqlc.narg(final_assistant_source_reply_to_message_id)::text,
        'assistant'::text,
        sqlc.arg(final_assistant_content)::jsonb,
        sqlc.arg(final_assistant_metadata)::jsonb,
        sqlc.arg(final_assistant_usage)::jsonb,
        sqlc.arg(final_assistant_session_mode)::text,
        sqlc.arg(final_assistant_runtime_type)::text,
        sqlc.narg(final_assistant_model_id)::uuid,
        sqlc.narg(final_assistant_event_id)::uuid,
        sqlc.narg(final_assistant_display_text)::text
      )
),
inserted_messages AS (
  INSERT INTO bot_history_messages (
    id,
    bot_id,
    session_id,
    sender_channel_identity_id,
    sender_account_user_id,
    source_message_id,
    source_reply_to_message_id,
    role,
    content,
    metadata,
    usage,
    session_mode,
    runtime_type,
    model_id,
    event_id,
    display_text,
    turn_id,
    turn_message_seq
  )
  SELECT
    input.message_id,
    sqlc.arg(bot_id),
    sqlc.arg(session_id),
    input.sender_channel_identity_id,
    input.sender_user_id,
    input.external_message_id,
    input.source_reply_to_message_id,
    input.role,
    input.content,
    input.metadata,
    input.usage,
    input.session_mode,
    input.runtime_type,
    input.model_id,
    input.event_id,
    input.display_text,
    sqlc.arg(turn_id),
    input.turn_message_seq
  FROM input_rows input
  RETURNING id, created_at, turn_message_seq
),
inserted_turn AS (
  INSERT INTO bot_history_turns (
    id,
    bot_id,
    session_id,
    position,
    request_message_id,
    assistant_message_id
  )
  SELECT
    sqlc.arg(turn_id),
    sqlc.arg(bot_id),
    sqlc.arg(session_id),
    COALESCE((
      SELECT position + 1
      FROM bot_history_turns
      WHERE session_id = sqlc.arg(session_id)
      ORDER BY position DESC
      LIMIT 1
    ), 1),
    user_message.id,
    tool_call_assistant.id
  FROM inserted_messages user_message
  JOIN inserted_messages tool_call_assistant ON tool_call_assistant.turn_message_seq = 2
  WHERE user_message.turn_message_seq = 1
  RETURNING id
)
SELECT inserted_messages.id, inserted_messages.created_at
FROM inserted_messages
JOIN inserted_turn ON true
ORDER BY inserted_messages.turn_message_seq;

-- name: CreateHistoryTurn :one
INSERT INTO bot_history_turns (
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id
)
VALUES (
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  COALESCE((
    SELECT position + 1
    FROM bot_history_turns
    WHERE session_id = sqlc.arg(session_id)
    ORDER BY position DESC
    LIMIT 1
  ), 1),
  sqlc.narg(request_message_id)::uuid,
  sqlc.narg(assistant_message_id)::uuid
)
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: CreateHistoryTurnWithID :one
INSERT INTO bot_history_turns (
  id,
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id
)
VALUES (
  sqlc.arg(turn_id),
  sqlc.arg(bot_id),
  sqlc.arg(session_id),
  COALESCE((
    SELECT position + 1
    FROM bot_history_turns
    WHERE session_id = sqlc.arg(session_id)
    ORDER BY position DESC
    LIMIT 1
  ), 1),
  sqlc.narg(request_message_id)::uuid,
  sqlc.narg(assistant_message_id)::uuid
)
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: BindHistoryTurnAssistantByRequest :one
UPDATE bot_history_turns
SET assistant_message_id = sqlc.arg(assistant_message_id),
    updated_at = now()
WHERE session_id = sqlc.arg(session_id)
  AND request_message_id = sqlc.arg(request_message_id)
  AND assistant_message_id IS NULL
  AND superseded_at IS NULL
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: BindLatestHistoryTurnAssistant :one
UPDATE bot_history_turns
SET assistant_message_id = sqlc.arg(assistant_message_id),
    updated_at = now()
WHERE id = (
  SELECT pending.id
  FROM bot_history_turns pending
  WHERE pending.session_id = sqlc.arg(session_id)
    AND pending.request_message_id IS NOT NULL
    AND pending.assistant_message_id IS NULL
    AND pending.superseded_at IS NULL
  ORDER BY pending.position DESC
  LIMIT 1
)
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: LinkMessageToHistoryTurn :exec
UPDATE bot_history_messages
SET turn_id = sqlc.arg(turn_id),
    turn_message_seq = sqlc.arg(turn_message_seq)
WHERE id = sqlc.arg(message_id);

-- name: AppendMessageToHistoryTurnByRequest :one
WITH target AS (
  SELECT turns.id, turns.session_id
  FROM bot_history_turns turns
  WHERE turns.session_id = sqlc.arg(session_id)
    AND turns.request_message_id = sqlc.arg(request_message_id)
    AND turns.superseded_at IS NULL
  LIMIT 1
),
next_seq AS (
  SELECT
    target.id,
    target.session_id,
    COALESCE((
      SELECT existing.turn_message_seq + 1
      FROM bot_history_messages existing
      WHERE existing.turn_id = target.id
      ORDER BY existing.turn_message_seq DESC
      LIMIT 1
    ), 1) AS turn_message_seq
  FROM target
)
UPDATE bot_history_messages m
SET turn_id = next_seq.id,
    turn_message_seq = next_seq.turn_message_seq
FROM next_seq
WHERE m.id = sqlc.arg(message_id)
  AND m.session_id = next_seq.session_id
  AND m.turn_id IS NULL
RETURNING m.id;

-- name: AppendMessageToLatestHistoryTurn :exec
WITH latest AS (
  SELECT turns.id, turns.session_id
  FROM bot_history_turns turns
  WHERE turns.session_id = sqlc.arg(session_id)
    AND turns.superseded_at IS NULL
  ORDER BY turns.position DESC
  LIMIT 1
)
UPDATE bot_history_messages m
SET turn_id = latest.id,
    turn_message_seq = COALESCE((
      SELECT existing.turn_message_seq + 1
      FROM bot_history_messages existing
      WHERE existing.turn_id = latest.id
      ORDER BY existing.turn_message_seq DESC
      LIMIT 1
    ), 1)
FROM latest
WHERE m.id = sqlc.arg(message_id)
  AND m.session_id = latest.session_id
  AND m.turn_id IS NULL;

-- name: LinkUnassignedMessagesAfterHistoryTurnAssistant :exec
WITH target AS (
  SELECT t.id AS turn_id, t.session_id, assistant.created_at AS assistant_created_at, assistant.id AS assistant_id
  FROM bot_history_turns t
  JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
  WHERE t.id = sqlc.arg(turn_id)
),
tail AS (
  SELECT
    m.id AS message_id,
    target.turn_id,
    2 + ROW_NUMBER() OVER (ORDER BY m.created_at, m.id) AS turn_message_seq
  FROM target
  JOIN bot_history_messages m
    ON m.session_id = target.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE m.id <> target.assistant_id
    AND m.turn_id IS NULL
    AND (m.created_at, m.id) > (target.assistant_created_at, target.assistant_id)
)
UPDATE bot_history_messages m
SET turn_id = tail.turn_id,
    turn_message_seq = tail.turn_message_seq
FROM tail
WHERE m.id = tail.message_id;

-- name: GetVisibleHistoryTurnByMessage :one
SELECT t.id, t.bot_id, t.session_id, t.position, t.request_message_id, t.assistant_message_id,
  t.superseded_by_turn_id, t.superseded_at, t.superseded_reason, t.created_at, t.updated_at
FROM bot_history_turns t
WHERE t.session_id = sqlc.arg(session_id)
  AND t.superseded_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM bot_history_messages m
    WHERE m.turn_id = t.id
      AND m.id = sqlc.arg(message_id)
)
LIMIT 1;

-- name: GetLatestVisibleHistoryTurnBySession :one
SELECT id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at
FROM bot_history_turns
WHERE session_id = sqlc.arg(session_id)
  AND superseded_at IS NULL
ORDER BY position DESC
LIMIT 1;

-- name: GetHistoryTurnByID :one
SELECT id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at
FROM bot_history_turns
WHERE id = sqlc.arg(old_turn_id)
  AND session_id = sqlc.arg(session_id)
  AND superseded_at IS NULL
LIMIT 1;

-- name: ReplaceHistoryTurn :one
WITH old_turn AS (
  SELECT t.*
  FROM bot_history_turns t
  WHERE t.id = sqlc.arg(old_turn_id)
    AND t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
  FOR UPDATE
),
replacement AS (
  INSERT INTO bot_history_turns (
    bot_id,
    session_id,
    position,
    request_message_id,
    assistant_message_id
  )
  SELECT
    old_turn.bot_id,
    old_turn.session_id,
    COALESCE((
      SELECT position + 1
      FROM bot_history_turns
      WHERE session_id = old_turn.session_id
      ORDER BY position DESC
      LIMIT 1
    ), 1),
    sqlc.narg(request_message_id)::uuid,
    sqlc.narg(assistant_message_id)::uuid
  FROM old_turn
  RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
    superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at
),
updated AS (
  UPDATE bot_history_turns old
  SET superseded_by_turn_id = replacement.id,
      superseded_at = sqlc.arg(superseded_at),
      superseded_reason = sqlc.arg(superseded_reason),
      updated_at = now()
  FROM replacement
  WHERE old.id = sqlc.arg(old_turn_id)
    AND old.session_id = sqlc.arg(session_id)
    AND old.superseded_at IS NULL
  RETURNING old.id
),
linked_anchors AS (
  UPDATE bot_history_messages m
  SET turn_id = replacement.id,
      turn_message_seq = CASE
        WHEN m.id = replacement.request_message_id THEN 1
        WHEN m.id = replacement.assistant_message_id THEN 2
        ELSE m.turn_message_seq
      END
  FROM replacement
  WHERE m.id = replacement.request_message_id
     OR m.id = replacement.assistant_message_id
),
linked_tail AS (
  UPDATE bot_history_messages m
  SET turn_id = tail.turn_id,
      turn_message_seq = tail.turn_message_seq
  FROM (
    SELECT
      m.id AS message_id,
      replacement.id AS turn_id,
      2 + ROW_NUMBER() OVER (ORDER BY m.created_at, m.id) AS turn_message_seq
    FROM replacement
    JOIN bot_history_messages assistant ON assistant.id = replacement.assistant_message_id
    JOIN bot_history_messages m
      ON m.session_id = replacement.session_id
     AND m.role IN ('assistant', 'tool')
    WHERE m.id <> replacement.assistant_message_id
      AND m.turn_id IS NULL
      AND (m.created_at, m.id) > (assistant.created_at, assistant.id)
  ) tail
  WHERE m.id = tail.message_id
)
SELECT replacement.id, replacement.bot_id, replacement.session_id, replacement.position,
  replacement.request_message_id, replacement.assistant_message_id,
  replacement.superseded_by_turn_id, replacement.superseded_at,
  replacement.superseded_reason, replacement.created_at, replacement.updated_at
FROM replacement
JOIN updated ON true;

-- name: SupersedeHistoryTurn :one
UPDATE bot_history_turns
SET superseded_by_turn_id = sqlc.arg(superseded_by_turn_id),
    superseded_at = sqlc.arg(superseded_at),
    superseded_reason = sqlc.arg(superseded_reason),
    updated_at = now()
WHERE id = sqlc.arg(old_turn_id)
  AND session_id = sqlc.arg(session_id)
  AND superseded_at IS NULL
RETURNING id, bot_id, session_id, position, request_message_id, assistant_message_id,
  superseded_by_turn_id, superseded_at, superseded_reason, created_at, updated_at;

-- name: ListMessages :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ListMessagesBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT 10000;

-- name: ListMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesSinceBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSince :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListActiveMessagesSinceBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.compact_id,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND m.created_at >= sqlc.arg(created_at)
  AND (m.metadata->>'trigger_mode' IS NULL OR m.metadata->>'trigger_mode' != 'passive_sync')
ORDER BY m.turn_position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: ListMessagesBefore :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND m.created_at < sqlc.arg(created_at)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeMessageBySession :many
WITH cursor_message AS (
  SELECT t.position AS turn_position, m.turn_message_seq, m.created_at, m.id
  FROM bot_history_messages m
  JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
  WHERE m.session_id = sqlc.arg(session_id)
    AND t.session_id = sqlc.arg(session_id)
    AND m.id = sqlc.arg(before_message_id)
  LIMIT 1
),
candidate_turns AS (
  SELECT t.id, t.position
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND t.position <= (SELECT cursor_message.turn_position FROM cursor_message)
  ORDER BY t.position DESC
  LIMIT sqlc.arg(max_count) + 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN candidate_turns t ON t.id = m.turn_id
CROSS JOIN cursor_message cursor
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (t.position, m.turn_message_seq, m.created_at, m.id)
    < (cursor.turn_position, cursor.turn_message_seq, cursor.created_at, cursor.id)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatest :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_visible_history_messages m
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.bot_id = sqlc.arg(bot_id)
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesLatestUIBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: GetMessageByIDBySession :one
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND m.id = sqlc.arg(message_id)
LIMIT 1;

-- name: GetMessageByExternalIDBySession :one
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: GetVisibleMessageCursorByExternalIDBySession :one
SELECT
  m.id,
  t.position AS turn_position,
  m.turn_message_seq,
  m.created_at
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: GetLocatedMessageByExternalIDBySession :one
SELECT
  m.id,
  t.position AS turn_position,
  m.turn_message_seq,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND m.source_message_id = sqlc.arg(external_message_id)
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT 1;

-- name: ListMessagesAfterBySession :many
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND m.created_at > sqlc.arg(created_at)
ORDER BY t.position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesAfterMessageBySession :many
WITH cursor_message AS (
  SELECT t.position AS turn_position, m.turn_message_seq, m.created_at, m.id
  FROM bot_history_messages m
  JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
  WHERE m.session_id = sqlc.arg(session_id)
    AND t.session_id = sqlc.arg(session_id)
    AND m.id = sqlc.arg(after_message_id)
  LIMIT 1
),
candidate_turns AS (
  SELECT t.id, t.position
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND t.position >= (SELECT cursor_message.turn_position FROM cursor_message)
  ORDER BY t.position ASC
  LIMIT sqlc.arg(max_count) + 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN candidate_turns t ON t.id = m.turn_id
CROSS JOIN cursor_message cursor
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (t.position, m.turn_message_seq, m.created_at, m.id)
    > (cursor.turn_position, cursor.turn_message_seq, cursor.created_at, cursor.id)
ORDER BY t.position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesBeforeCursorBySession :many
WITH candidate_turns AS (
  SELECT t.id, t.position
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND t.position <= sqlc.arg(cursor_turn_position)
  ORDER BY t.position DESC
  LIMIT sqlc.arg(max_count) + 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN candidate_turns t ON t.id = m.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (t.position, m.turn_message_seq, m.created_at, m.id)
    < (
      sqlc.arg(cursor_turn_position)::bigint,
      sqlc.arg(cursor_turn_message_seq)::bigint,
      sqlc.arg(cursor_created_at)::timestamptz,
      sqlc.arg(cursor_message_id)::uuid
    )
ORDER BY t.position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: ListMessagesAfterCursorBySession :many
WITH candidate_turns AS (
  SELECT t.id, t.position
  FROM bot_history_turns t
  WHERE t.session_id = sqlc.arg(session_id)
    AND t.superseded_at IS NULL
    AND t.position >= sqlc.arg(cursor_turn_position)
  ORDER BY t.position ASC
  LIMIT sqlc.arg(max_count) + 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN candidate_turns t ON t.id = m.turn_id
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND (t.position, m.turn_message_seq, m.created_at, m.id)
    > (
      sqlc.arg(cursor_turn_position)::bigint,
      sqlc.arg(cursor_turn_message_seq)::bigint,
      sqlc.arg(cursor_created_at)::timestamptz,
      sqlc.arg(cursor_message_id)::uuid
    )
ORDER BY t.position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC
LIMIT sqlc.arg(max_count);

-- name: ListVisibleMessagesFromBySession :many
WITH cursor_message AS (
  SELECT t.position AS turn_position
  FROM bot_history_messages m
  JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
  WHERE m.session_id = sqlc.arg(session_id)
    AND t.session_id = sqlc.arg(session_id)
    AND m.id = sqlc.arg(message_id)
  LIMIT 1
)
SELECT
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id AS sender_user_id,
  m.source_message_id AS external_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.session_mode,
  m.runtime_type,
  m.event_id,
  m.display_text,
  m.created_at,
  ci.display_name AS sender_display_name,
  ci.avatar_url AS sender_avatar_url,
  s.channel_type AS platform
FROM bot_history_messages m
JOIN bot_history_turns t ON t.id = m.turn_id AND t.superseded_at IS NULL
CROSS JOIN cursor_message cursor
LEFT JOIN channel_identities ci ON ci.id = m.sender_channel_identity_id
LEFT JOIN bot_sessions s ON s.id = m.session_id
WHERE m.session_id = sqlc.arg(session_id)
  AND t.session_id = sqlc.arg(session_id)
  AND t.position >= cursor.turn_position
ORDER BY t.position ASC, m.turn_message_seq ASC, m.created_at ASC, m.id ASC;

-- name: CountMessagesByBot :one
SELECT COUNT(*) FROM bot_visible_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesByBot :exec
DELETE FROM bot_history_messages
WHERE bot_id = sqlc.arg(bot_id);

-- name: DeleteMessagesBySession :exec
DELETE FROM bot_history_messages
WHERE session_id = sqlc.arg(session_id);

-- name: DeleteMessagesByIDs :exec
DELETE FROM bot_history_messages
WHERE id = ANY(sqlc.arg(ids)::uuid[]);

-- name: ListObservedConversationsByChannelIdentity :many
WITH observed_routes AS (
  SELECT
    s.route_id,
    MAX(m.created_at)::timestamptz AS last_observed_at
  FROM bot_visible_history_messages m
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
  FROM bot_visible_history_messages m
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
  m.sender_channel_identity_id,
  m.role,
  m.content,
  m.created_at,
  ci.display_name AS sender_display_name,
  s.channel_type AS platform
FROM bot_visible_history_messages m
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
ORDER BY m.turn_position DESC, m.turn_message_seq DESC, m.created_at DESC, m.id DESC
LIMIT sqlc.arg(max_count);

-- name: MarkMessagesCompacted :exec
UPDATE bot_history_messages
SET compact_id = $1
WHERE id = ANY($2::uuid[]);

-- name: ListUncompactedMessagesBySession :many
SELECT id, bot_id, session_id, role, content, usage, sender_channel_identity_id, compact_id, created_at
FROM bot_visible_history_messages
WHERE session_id = $1
  AND compact_id IS NULL
ORDER BY turn_position ASC, turn_message_seq ASC, created_at ASC, id ASC;
