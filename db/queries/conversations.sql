-- name: CreateChat :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  (COALESCE(NULLIF(sqlc.arg(kind)::text, ''), CASE WHEN b.type = 'public' THEN 'group' ELSE 'direct' END))::text AS kind,
  CASE WHEN sqlc.arg(kind) = 'thread' THEN sqlc.arg(parent_chat_id)::uuid ELSE NULL::uuid END AS parent_chat_id,
  COALESCE(NULLIF(sqlc.arg(title)::text, ''), b.display_name) AS title,
  COALESCE(sqlc.arg(created_by_user_id)::uuid, b.owner_user_id) AS created_by_user_id,
  COALESCE(sqlc.arg(metadata)::jsonb, b.metadata) AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
LIMIT 1;

-- name: GetChatByID :one
SELECT
  b.id AS id,
  b.id AS bot_id,
  CASE WHEN b.type = 'public' THEN 'group' ELSE 'direct' END AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = $1;

-- name: ListChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  CASE WHEN b.type = 'public' THEN 'group' ELSE 'direct' END AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN bot_members bm ON bm.bot_id = b.id AND bm.user_id = sqlc.arg(user_id)
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
  AND (b.owner_user_id = sqlc.arg(user_id) OR bm.user_id IS NOT NULL)
ORDER BY b.updated_at DESC;

-- name: ListVisibleChatsByBotAndUser :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  CASE WHEN b.type = 'public' THEN 'group' ELSE 'direct' END AS kind,
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
    ELSE COALESCE(bm.role, ''::text)
  END)::text AS participant_role,
  NULL::timestamptz AS last_observed_at
FROM bots b
LEFT JOIN bot_members bm ON bm.bot_id = b.id AND bm.user_id = sqlc.arg(user_id)
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = sqlc.arg(bot_id)
  AND (b.owner_user_id = sqlc.arg(user_id) OR bm.user_id IS NOT NULL)
ORDER BY b.updated_at DESC;

-- name: GetChatReadAccessByUser :one
SELECT
  'participant'::text AS access_mode,
  (CASE
    WHEN b.owner_user_id = sqlc.arg(user_id) THEN 'owner'
    ELSE COALESCE(bm.role, ''::text)
  END)::text AS participant_role,
  NULL::timestamptz AS last_observed_at
FROM bots b
LEFT JOIN bot_members bm ON bm.bot_id = b.id AND bm.user_id = sqlc.arg(user_id)
WHERE b.id = sqlc.arg(chat_id)
  AND (b.owner_user_id = sqlc.arg(user_id) OR bm.user_id IS NOT NULL)
LIMIT 1;

-- name: ListThreadsByParent :many
SELECT
  b.id AS id,
  b.id AS bot_id,
  CASE WHEN b.type = 'public' THEN 'group' ELSE 'direct' END AS kind,
  NULL::uuid AS parent_chat_id,
  b.display_name AS title,
  b.owner_user_id AS created_by_user_id,
  b.metadata AS metadata,
  chat_models.model_id AS model_id,
  b.created_at,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = $1
ORDER BY b.created_at DESC;

-- name: UpdateChatTitle :one
WITH updated AS (
  UPDATE bots
  SET display_name = sqlc.arg(title),
      updated_at = now()
  WHERE bots.id = sqlc.arg(bot_id)
  RETURNING *
)
SELECT
  updated.id AS id,
  updated.id AS bot_id,
  CASE WHEN updated.type = 'public' THEN 'group' ELSE 'direct' END AS kind,
  NULL::uuid AS parent_chat_id,
  updated.display_name AS title,
  updated.owner_user_id AS created_by_user_id,
  updated.metadata,
  chat_models.model_id AS model_id,
  updated.created_at,
  updated.updated_at
FROM updated
LEFT JOIN models chat_models ON chat_models.id = updated.chat_model_id;

-- name: TouchChat :exec
UPDATE bots
SET updated_at = now()
WHERE id = sqlc.arg(chat_id);

-- name: DeleteChat :exec
WITH deleted_messages AS (
  DELETE FROM bot_history_messages
  WHERE bot_id = sqlc.arg(chat_id)
)
DELETE FROM bot_channel_routes bcr
WHERE bcr.bot_id = sqlc.arg(chat_id);

-- chat_participants

-- name: AddChatParticipant :one
INSERT INTO bot_members (bot_id, user_id, role)
VALUES (sqlc.arg(chat_id), sqlc.arg(user_id), sqlc.arg(role))
ON CONFLICT (bot_id, user_id) DO UPDATE SET role = EXCLUDED.role
RETURNING bot_id AS chat_id, user_id, role, created_at AS joined_at;

-- name: GetChatParticipant :one
WITH owner_participant AS (
  SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner'::text AS role, b.created_at AS joined_at
  FROM bots b
  WHERE b.id = sqlc.arg(chat_id) AND b.owner_user_id = sqlc.arg(user_id)
),
member_participant AS (
  SELECT bm.bot_id AS chat_id, bm.user_id, bm.role, bm.created_at AS joined_at
  FROM bot_members bm
  WHERE bm.bot_id = sqlc.arg(chat_id) AND bm.user_id = sqlc.arg(user_id)
)
SELECT chat_id, user_id, role, joined_at
FROM (
  SELECT * FROM owner_participant
  UNION ALL
  SELECT * FROM member_participant
) p
ORDER BY CASE WHEN role = 'owner' THEN 0 ELSE 1 END
LIMIT 1;

-- name: ListChatParticipants :many
WITH owner_participant AS (
  SELECT b.id AS chat_id, b.owner_user_id AS user_id, 'owner'::text AS role, b.created_at AS joined_at
  FROM bots b
  WHERE b.id = sqlc.arg(chat_id)
),
member_participant AS (
  SELECT bm.bot_id AS chat_id, bm.user_id, bm.role, bm.created_at AS joined_at
  FROM bot_members bm
  WHERE bm.bot_id = sqlc.arg(chat_id)
    AND bm.user_id <> (SELECT owner_user_id FROM bots WHERE id = sqlc.arg(chat_id))
)
SELECT chat_id, user_id, role, joined_at
FROM (
  SELECT * FROM owner_participant
  UNION ALL
  SELECT * FROM member_participant
) p
ORDER BY joined_at ASC;

-- name: RemoveChatParticipant :exec
DELETE FROM bot_members
WHERE bot_id = sqlc.arg(chat_id)
  AND user_id = sqlc.arg(user_id)
  AND user_id <> (SELECT owner_user_id FROM bots WHERE id = sqlc.arg(chat_id));

-- name: CopyParticipantsToChat :exec
INSERT INTO bot_members (bot_id, user_id, role)
SELECT sqlc.arg(chat_id_2), bm.user_id, bm.role
FROM bot_members bm
WHERE bm.bot_id = sqlc.arg(chat_id)
ON CONFLICT (bot_id, user_id) DO NOTHING;

-- chat_settings

-- name: UpsertChatSettings :one
WITH resolved_model AS (
  SELECT id
  FROM models
  WHERE model_id = NULLIF(sqlc.narg(model_id)::text, '')
  LIMIT 1
),
updated AS (
  UPDATE bots
  SET chat_model_id = COALESCE((SELECT id FROM resolved_model), bots.chat_model_id),
      updated_at = now()
  WHERE bots.id = sqlc.arg(id)
  RETURNING bots.id, bots.chat_model_id, bots.updated_at
)
SELECT
  updated.id AS chat_id,
  chat_models.model_id AS model_id,
  updated.updated_at
FROM updated
LEFT JOIN models chat_models ON chat_models.id = updated.chat_model_id;

-- name: GetChatSettings :one
SELECT
  b.id AS chat_id,
  chat_models.model_id AS model_id,
  b.updated_at
FROM bots b
LEFT JOIN models chat_models ON chat_models.id = b.chat_model_id
WHERE b.id = $1;
