-- name: CreateChannelIdentity :one
INSERT INTO channel_identities (user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: GetChannelIdentityByID :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE id = $1;

-- name: GetChannelIdentityByIDForUpdate :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE id = $1
FOR UPDATE;

-- name: GetChannelIdentityByChannelSubject :one
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE channel_type = $1 AND channel_subject_id = $2;

-- name: UpsertChannelIdentityByChannelSubject :one
INSERT INTO channel_identities (user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (channel_type, channel_subject_id)
DO UPDATE SET
  display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), channel_identities.display_name),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), channel_identities.avatar_url),
  metadata = EXCLUDED.metadata,
  user_id = COALESCE(channel_identities.user_id, EXCLUDED.user_id),
  updated_at = now()
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: ListChannelIdentitiesByUserID :many
SELECT id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: SetChannelIdentityLinkedUser :one
UPDATE channel_identities
SET user_id = $2, updated_at = now()
WHERE id = $1
RETURNING id, user_id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

