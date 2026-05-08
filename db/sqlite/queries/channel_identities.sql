-- name: CreateChannelIdentity :one
INSERT INTO channel_identities (id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(channel_type),
  sqlc.arg(channel_subject_id),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(metadata)
)
RETURNING id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: GetChannelIdentityByID :one
SELECT id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE id = sqlc.arg(id);

-- name: GetChannelIdentityByIDForUpdate :one
SELECT id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE id = sqlc.arg(id);

-- name: GetChannelIdentityByChannelSubject :one
SELECT id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at
FROM channel_identities
WHERE channel_type = sqlc.arg(channel_type) AND channel_subject_id = sqlc.arg(channel_subject_id);

-- name: UpsertChannelIdentityByChannelSubject :one
INSERT INTO channel_identities (id, channel_type, channel_subject_id, display_name, avatar_url, metadata)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(channel_type),
  sqlc.arg(channel_subject_id),
  sqlc.arg(display_name),
  sqlc.arg(avatar_url),
  sqlc.arg(metadata)
)
ON CONFLICT (channel_type, channel_subject_id)
DO UPDATE SET
  display_name = COALESCE(NULLIF(EXCLUDED.display_name, ''), channel_identities.display_name),
  avatar_url = COALESCE(NULLIF(EXCLUDED.avatar_url, ''), channel_identities.avatar_url),
  metadata = EXCLUDED.metadata,
  updated_at = CURRENT_TIMESTAMP
RETURNING id, channel_type, channel_subject_id, display_name, avatar_url, metadata, created_at, updated_at;

-- name: SearchChannelIdentities :many
SELECT
  ci.id,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name,
  ci.avatar_url,
  ci.metadata,
  ci.created_at,
  ci.updated_at
FROM channel_identities ci
WHERE
  sqlc.arg(query) = ''
  OR lower(ci.channel_type) LIKE '%' || lower(sqlc.arg(query)) || '%'
  OR lower(ci.channel_subject_id) LIKE '%' || lower(sqlc.arg(query)) || '%'
  OR lower(COALESCE(ci.display_name, '')) LIKE '%' || lower(sqlc.arg(query)) || '%'
ORDER BY ci.updated_at DESC
LIMIT sqlc.arg(limit_count);
