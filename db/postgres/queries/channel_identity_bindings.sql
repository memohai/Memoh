-- name: CreateChannelLinkCode :one
INSERT INTO channel_link_codes (token, user_id, channel_type, expires_at)
VALUES ($1, $2, sqlc.narg(channel_type)::text, $3)
RETURNING token, user_id, channel_type, expires_at, consumed_at, consumed_channel_identity_id, created_at;

-- name: GetChannelLinkCodeByToken :one
SELECT token, user_id, channel_type, expires_at, consumed_at, consumed_channel_identity_id, created_at
FROM channel_link_codes
WHERE token = $1;

-- name: RedeemChannelLinkCode :one
WITH claimed AS (
  UPDATE channel_link_codes
  SET consumed_at = now(),
      consumed_channel_identity_id = $2
  WHERE token = $1
    AND consumed_at IS NULL
    AND expires_at > now()
  RETURNING user_id
)
INSERT INTO user_channel_identity_bindings (user_id, channel_identity_id)
SELECT user_id, $2
FROM claimed
ON CONFLICT (user_id, channel_identity_id) DO UPDATE
  SET updated_at = now()
RETURNING id, user_id, channel_identity_id, created_at, updated_at;

-- name: MarkChannelLinkCodeConsumed :one
UPDATE channel_link_codes
SET consumed_at = now(),
    consumed_channel_identity_id = $2
WHERE token = $1 AND consumed_at IS NULL AND expires_at > now()
RETURNING token, user_id, channel_type, expires_at, consumed_at, consumed_channel_identity_id, created_at;

-- name: UpsertUserChannelIdentityBinding :one
INSERT INTO user_channel_identity_bindings (user_id, channel_identity_id)
VALUES ($1, $2)
ON CONFLICT (user_id, channel_identity_id) DO UPDATE
  SET updated_at = now()
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
WHERE b.user_id = $1
ORDER BY b.created_at DESC;

-- name: DeleteUserChannelIdentityBinding :exec
DELETE FROM user_channel_identity_bindings
WHERE user_id = $1 AND channel_identity_id = $2;

-- name: ListUserIDsByChannelIdentity :many
SELECT user_id
FROM user_channel_identity_bindings
WHERE channel_identity_id = $1;
