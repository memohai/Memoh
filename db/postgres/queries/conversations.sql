-- name: CreateChat :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  (COALESCE(NULLIF(sqlc.arg(kind)::text, ''), 'direct'))::text AS kind,
  CASE WHEN sqlc.arg(kind) = 'thread' THEN sqlc.arg(parent_chat_id)::uuid ELSE NULL::uuid END AS parent_chat_id,
  COALESCE(NULLIF(sqlc.arg(title)::text, ''), b.display_name) AS title,
  COALESCE(sqlc.arg(created_by_user_id)::uuid, b.owner_user_id) AS created_by_user_id,
  COALESCE(sqlc.arg(metadata)::jsonb, b.metadata) AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id AND chat_models.team_id = public.memoh_current_team_id()
WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(bot_id)
LIMIT 1;

-- name: GetChatByID :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id AND chat_models.team_id = public.memoh_current_team_id()
WHERE b.team_id = public.memoh_current_team_id() AND b.id = $1;

-- name: ListChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id AND chat_models.team_id = public.memoh_current_team_id()
WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(bot_id)
  AND b.owner_user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: ListVisibleChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at,
  'participant'::text AS access_mode,
  (CASE
    WHEN b.owner_user_id = sqlc.arg(user_id) THEN 'owner'
    ELSE ''::text
  END)::text AS participant_role,
  NULL::timestamptz AS last_observed_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id AND chat_models.team_id = public.memoh_current_team_id()
WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(bot_id)
  AND b.owner_user_id = sqlc.arg(user_id)
ORDER BY b.updated_at DESC;

-- name: GetChatReadAccessByUser :one
SELECT
  'participant'::text AS access_mode,
  'owner'::text AS participant_role,
  NULL::timestamptz AS last_observed_at
FROM bots b
WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(chat_id)
  AND b.owner_user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListThreadsByParent :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id AND chat_models.team_id = public.memoh_current_team_id()
WHERE b.team_id = public.memoh_current_team_id() AND b.id = $1
ORDER BY b.created_at DESC;

-- name: UpdateChatTitle :one
WITH updated AS (
  UPDATE bots
  SET display_name = sqlc.arg(title),
      updated_at = now()
  WHERE bots.team_id = public.memoh_current_team_id() AND bots.id = sqlc.arg(bot_id)
  RETURNING *
)
SELECT
  updated.id AS id,
  updated.id AS bot_id,
  'direct'::text AS kind,
  NULL::uuid AS parent_chat_id,
  updated.display_name AS title,
  updated.owner_user_id AS created_by_user_id,
  updated.metadata,
  chat_models.model_id AS model_id,
  updated.created_at,
  updated.updated_at
FROM updated
LEFT JOIN models chat_models ON chat_models.id = updated.chat_model_id AND chat_models.team_id = public.memoh_current_team_id();

-- name: TouchChat :exec
UPDATE bots
SET updated_at = now()
WHERE team_id = public.memoh_current_team_id() AND id = sqlc.arg(chat_id);

-- name: DeleteChat :exec
WITH target_sessions AS MATERIALIZED (
  SELECT session.id
  FROM bot_sessions session
  WHERE session.team_id = public.memoh_current_team_id()
    AND session.bot_id = sqlc.arg(chat_id)
  ORDER BY session.id
  FOR UPDATE
),
target_compaction_artifacts AS MATERIALIZED (
  SELECT compact.id
  FROM bot_history_message_compacts compact
  WHERE compact.team_id = public.memoh_current_team_id()
    AND compact.bot_id = sqlc.arg(chat_id)
    AND (SELECT count(*) FROM target_sessions) >= 0
  ORDER BY compact.id
  FOR UPDATE
),
deleted_compaction_artifacts AS (
  DELETE FROM bot_history_message_compacts compact
  USING target_compaction_artifacts target
  WHERE compact.team_id = public.memoh_current_team_id()
    AND compact.id = target.id
  RETURNING compact.id
),
target_messages AS MATERIALIZED (
  SELECT message.id
  FROM bot_history_messages message
  WHERE message.team_id = public.memoh_current_team_id()
    AND message.bot_id = sqlc.arg(chat_id)
    AND (SELECT count(*) FROM target_sessions) >= 0
    AND (SELECT count(*) FROM deleted_compaction_artifacts) >= 0
  ORDER BY message.id
  FOR UPDATE
),
deleted_messages AS (
  DELETE FROM bot_history_messages message
  USING target_messages target
  WHERE message.team_id = public.memoh_current_team_id()
    AND message.id = target.id
  RETURNING message.id
),
deleted_sessions AS (
  DELETE FROM bot_sessions session
  USING target_sessions target
  WHERE session.team_id = public.memoh_current_team_id()
    AND session.id = target.id
    AND (SELECT count(*) FROM deleted_messages) >= 0
  RETURNING session.id
)
DELETE FROM bot_channel_routes bcr
WHERE bcr.team_id = public.memoh_current_team_id()
  AND bcr.bot_id = sqlc.arg(chat_id)
  AND (SELECT count(*) FROM deleted_sessions) >= 0;

-- name: GetChatParticipant :one
SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner'::text AS role, b.created_at AS joined_at
FROM bots b
WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(chat_id) AND b.owner_user_id = sqlc.arg(user_id)
LIMIT 1;

-- name: ListChatParticipants :many
SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner'::text AS role, b.created_at AS joined_at
FROM bots b
WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(chat_id)
ORDER BY joined_at ASC;

-- name: RemoveChatParticipant :exec
SELECT 1
WHERE EXISTS (
  SELECT 1
  FROM bots b
  WHERE b.team_id = public.memoh_current_team_id() AND b.id = sqlc.arg(chat_id)
    AND b.owner_user_id = sqlc.arg(user_id)
);

-- chat_settings

-- name: UpsertChatSettings :one
WITH
updated AS (
  UPDATE bots
  SET chat_model_id = COALESCE(sqlc.narg(chat_model_id)::uuid, bots.chat_model_id),
      updated_at = now()
  WHERE bots.team_id = public.memoh_current_team_id() AND bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.chat_model_id, bots.updated_at
)
SELECT
  updated.id AS chat_id,
  chat_models.id AS model_id,
  updated.updated_at
FROM updated
LEFT JOIN models chat_models ON chat_models.id = updated.chat_model_id AND chat_models.team_id = public.memoh_current_team_id();

-- name: GetChatSettings :one
SELECT
  b.id AS chat_id,
  chat_models.id AS model_id,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id AND chat_models.team_id = public.memoh_current_team_id()
WHERE b.team_id = public.memoh_current_team_id() AND b.id = $1;
