-- name: CreateChannelIdentity :one
INSERT INTO iam_channel_identities (user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: GetChannelIdentityByID :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE id = $1;

-- name: GetChannelIdentityByIDForUpdate :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE id = $1
FOR UPDATE;

-- name: GetChannelIdentityByChannelSubject :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE channel_type = $1 AND channel_subject_id = $2;

-- name: UpsertChannelIdentityByChannelSubject :one
INSERT INTO iam_channel_identities (user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (channel_type, channel_subject_id)
DO UPDATE SET
  display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), iam_channel_identities.display_name),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), iam_channel_identities.avatar_url),
  metadata = EXCLUDED.metadata,
  user_id = COALESCE(iam_channel_identities.user_id, EXCLUDED.user_id),
  updated_at = now()
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: ListChannelIdentitiesByUserID :many
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: SearchChannelIdentities :many
SELECT
  ci.id,
  ci.user_id,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name,
  ci.avatar_url,
  ci.metadata,
  ci.created_at,
  ci.updated_at,
  u.username AS linked_username,
  u.display_name AS linked_display_name,
  u.avatar_url AS linked_avatar_url
FROM iam_channel_identities ci
LEFT JOIN iam_users u ON u.id = ci.user_id
WHERE
  sqlc.arg(query)::text = ''
  OR ci.channel_type ILIKE '%' || sqlc.arg(query)::text || '%'
  OR ci.channel_subject_id ILIKE '%' || sqlc.arg(query)::text || '%'
  OR COALESCE(ci.display_name, '') ILIKE '%' || sqlc.arg(query)::text || '%'
  OR COALESCE(u.username, '') ILIKE '%' || sqlc.arg(query)::text || '%'
  OR COALESCE(u.display_name, '') ILIKE '%' || sqlc.arg(query)::text || '%'
ORDER BY ci.updated_at DESC
LIMIT sqlc.arg(limit_count);

-- name: SetChannelIdentityLinkedUser :one
UPDATE iam_channel_identities
SET user_id = $2, updated_at = now()
WHERE id = $1
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

