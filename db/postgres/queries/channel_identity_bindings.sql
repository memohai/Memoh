-- name: CreateChannelLinkCode :one
INSERT INTO channel_link_codes (token, team_id, user_id, channel_type, expires_at)
VALUES (sqlc.arg(token), sqlc.arg(team_id), sqlc.arg(user_id), sqlc.narg(channel_type)::text, sqlc.arg(expires_at))
RETURNING *;

-- name: GetChannelLinkCodeByToken :one
SELECT *
FROM channel_link_codes
WHERE token = sqlc.arg(token)
  AND team_id = sqlc.arg(team_id);

-- name: RedeemChannelLinkCode :one
WITH identity_scope AS (
  SELECT ci.id, ci.team_id, ci.channel_type
  FROM channel_identities ci
  WHERE ci.id = sqlc.arg(channel_identity_id)
    AND ci.team_id = sqlc.arg(team_id)
),
claimed AS (
  UPDATE channel_link_codes c
  SET consumed_at = now(),
      consumed_channel_identity_id = (SELECT id FROM identity_scope)
  WHERE c.token = sqlc.arg(token)
    AND c.team_id = sqlc.arg(team_id)
    AND c.consumed_at IS NULL
    AND c.expires_at > now()
    AND EXISTS (
      SELECT 1
      FROM identity_scope i
      WHERE c.channel_type = '' OR c.channel_type = i.channel_type
    )
  RETURNING c.team_id, c.user_id, (SELECT id FROM identity_scope) AS channel_identity_id
)
INSERT INTO user_channel_identity_bindings (team_id, user_id, channel_identity_id)
SELECT team_id, user_id, channel_identity_id
FROM claimed
ON CONFLICT (team_id, user_id, channel_identity_id) DO UPDATE
  SET updated_at = now()
RETURNING *;

-- name: MarkChannelLinkCodeConsumed :one
UPDATE channel_link_codes
SET consumed_at = now(),
    consumed_channel_identity_id = sqlc.arg(channel_identity_id)
WHERE token = sqlc.arg(token)
  AND team_id = sqlc.arg(team_id)
  AND consumed_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: UpsertUserChannelIdentityBinding :one
INSERT INTO user_channel_identity_bindings (team_id, user_id, channel_identity_id)
SELECT sqlc.arg(team_id), sqlc.arg(user_id), ci.id
FROM channel_identities ci
WHERE ci.id = sqlc.arg(channel_identity_id)
  AND ci.team_id = sqlc.arg(team_id)
ON CONFLICT (team_id, user_id, channel_identity_id) DO UPDATE
  SET updated_at = now()
RETURNING *;

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
LEFT JOIN channel_identities ci ON ci.id = b.channel_identity_id AND ci.team_id = b.team_id
WHERE b.team_id = sqlc.arg(team_id)
  AND b.user_id = sqlc.arg(user_id)
ORDER BY b.created_at DESC;

-- name: ListChannelIdentityBindingsForBot :many
SELECT DISTINCT
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
INNER JOIN bots bot_scope ON bot_scope.id = sqlc.arg(bot_id)
INNER JOIN bot_user_grants g ON g.user_id = b.user_id AND g.bot_id = bot_scope.id AND g.subject_type = 'user'
LEFT JOIN channel_identities ci ON ci.id = b.channel_identity_id AND ci.team_id = b.team_id
WHERE b.team_id = bot_scope.team_id
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
LEFT JOIN channel_identities ci ON ci.id = b.channel_identity_id AND ci.team_id = b.team_id
WHERE b.team_id = sqlc.arg(team_id)
ORDER BY b.created_at DESC;

-- name: DeleteUserChannelIdentityBinding :exec
DELETE FROM user_channel_identity_bindings
WHERE team_id = sqlc.arg(team_id)
  AND user_id = sqlc.arg(user_id)
  AND channel_identity_id = sqlc.arg(channel_identity_id);

-- name: ListUserIDsByChannelIdentity :many
SELECT user_id
FROM user_channel_identity_bindings
WHERE team_id = sqlc.arg(team_id)
  AND channel_identity_id = sqlc.arg(channel_identity_id);
