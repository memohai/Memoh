-- name: CreateChannelIdentity :one
INSERT INTO iam_channel_identities (id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(user_id),
  sqlc.arg(channel_type),
  sqlc.arg(channel_subject_id),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(metadata)
)
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: GetChannelIdentityByID :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE id = sqlc.arg(id);

-- name: GetChannelIdentityByIDForUpdate :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE id = sqlc.arg(id);

-- name: GetChannelIdentityByChannelSubject :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE channel_type = sqlc.arg(channel_type) AND channel_subject_id = sqlc.arg(channel_subject_id);

-- name: UpsertChannelIdentityByChannelSubject :one
INSERT INTO iam_channel_identities (id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(user_id),
  sqlc.arg(channel_type),
  sqlc.arg(channel_subject_id),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(metadata)
)
ON CONFLICT (channel_type, channel_subject_id)
DO UPDATE SET
  display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), iam_channel_identities.display_name),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), iam_channel_identities.avatar_url),
  metadata = EXCLUDED.metadata,
  user_id = COALESCE(iam_channel_identities.user_id, EXCLUDED.user_id),
  updated_at = CURRENT_TIMESTAMP
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: ListChannelIdentitiesByUserID :many
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM iam_channel_identities
WHERE user_id = sqlc.arg(user_id)
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
  sqlc.arg(query) = ''
  OR lower(ci.channel_type) LIKE '%' || lower(sqlc.arg(query)) || '%'
  OR lower(ci.channel_subject_id) LIKE '%' || lower(sqlc.arg(query)) || '%'
  OR lower(COALESCE(ci.display_name, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
  OR lower(COALESCE(u.username, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
  OR lower(COALESCE(u.display_name, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
ORDER BY ci.updated_at DESC
LIMIT sqlc.arg(limit_count);

-- name: SetChannelIdentityLinkedUser :one
UPDATE iam_channel_identities
SET user_id = sqlc.arg(user_id), updated_at = CURRENT_TIMESTAMP
WHERE id = sqlc.arg(id)
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;
