-- name: CreateChannelLinkCode :one
INSERT INTO channel_link_codes (token, user_id, channel_type, expires_at)
VALUES (sqlc.arg(token), sqlc.arg(user_id), sqlc.arg(channel_type), sqlc.arg(expires_at))
RETURNING token, user_id, channel_type, expires_at, consumed_at, consumed_channel_identity_id, created_at;

-- name: GetChannelLinkCodeByToken :one
SELECT token, user_id, channel_type, expires_at, consumed_at, consumed_channel_identity_id, created_at
FROM channel_link_codes
WHERE token = sqlc.arg(token);

-- name: MarkChannelLinkCodeConsumed :one
UPDATE channel_link_codes
SET consumed_at = CURRENT_TIMESTAMP,
    consumed_channel_identity_id = sqlc.arg(consumed_channel_identity_id)
WHERE token = sqlc.arg(token) AND consumed_at IS NULL AND datetime(expires_at) > CURRENT_TIMESTAMP
RETURNING token, user_id, channel_type, expires_at, consumed_at, consumed_channel_identity_id, created_at;

-- name: UpsertUserChannelIdentityBinding :one
INSERT INTO user_channel_identity_bindings (id, user_id, channel_identity_id)
VALUES (
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  sqlc.arg(user_id),
  sqlc.arg(channel_identity_id)
)
ON CONFLICT (user_id, channel_identity_id) DO UPDATE
  SET updated_at = CURRENT_TIMESTAMP
RETURNING id, user_id, channel_identity_id, created_at, updated_at;

-- name: ListChannelIdentityBindingsForUser :many
SELECT
  b.id,
  b.user_id,
  b.channel_identity_id,
  b.created_at,
  b.updated_at,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name AS channel_identity_display_name,
  ci.avatar_url AS channel_identity_avatar_url
FROM user_channel_identity_bindings b
LEFT JOIN channel_identities ci ON ci.id = b.channel_identity_id
WHERE b.user_id = sqlc.arg(user_id)
ORDER BY b.created_at DESC;

-- name: ListChannelIdentityBindings :many
SELECT
  b.id,
  b.user_id,
  b.channel_identity_id,
  b.created_at,
  b.updated_at,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name AS channel_identity_display_name,
  ci.avatar_url AS channel_identity_avatar_url
FROM user_channel_identity_bindings b
LEFT JOIN channel_identities ci ON ci.id = b.channel_identity_id
ORDER BY b.created_at DESC;

-- name: DeleteUserChannelIdentityBinding :exec
DELETE FROM user_channel_identity_bindings
WHERE user_id = sqlc.arg(user_id) AND channel_identity_id = sqlc.arg(channel_identity_id);

-- name: ListUserIDsByChannelIdentity :many
SELECT user_id
FROM user_channel_identity_bindings
WHERE channel_identity_id = sqlc.arg(channel_identity_id);
