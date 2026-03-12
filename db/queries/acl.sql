-- name: UpsertBotACLGuestAllAllowRule :one
INSERT INTO bot_acl_rules (bot_id, action, effect, subject_kind, created_by_user_id)
VALUES ($1, 'chat.trigger', 'allow', 'guest_all', $2)
ON CONFLICT ON CONSTRAINT bot_acl_rules_unique_user
DO UPDATE SET
  created_by_user_id = COALESCE(EXCLUDED.created_by_user_id, bot_acl_rules.created_by_user_id),
  updated_at = now()
RETURNING id, bot_id, action, effect, subject_kind, user_id, channel_identity_id, created_by_user_id, created_at, updated_at;

-- name: UpsertBotACLUserRule :one
INSERT INTO bot_acl_rules (bot_id, action, effect, subject_kind, user_id, created_by_user_id)
VALUES ($1, 'chat.trigger', $2, 'user', $3, $4)
ON CONFLICT ON CONSTRAINT bot_acl_rules_unique_user
DO UPDATE SET
  created_by_user_id = COALESCE(EXCLUDED.created_by_user_id, bot_acl_rules.created_by_user_id),
  updated_at = now()
RETURNING id, bot_id, action, effect, subject_kind, user_id, channel_identity_id, created_by_user_id, created_at, updated_at;

-- name: UpsertBotACLChannelIdentityRule :one
INSERT INTO bot_acl_rules (bot_id, action, effect, subject_kind, channel_identity_id, created_by_user_id)
VALUES ($1, 'chat.trigger', $2, 'channel_identity', $3, $4)
ON CONFLICT ON CONSTRAINT bot_acl_rules_unique_channel_identity
DO UPDATE SET
  created_by_user_id = COALESCE(EXCLUDED.created_by_user_id, bot_acl_rules.created_by_user_id),
  updated_at = now()
RETURNING id, bot_id, action, effect, subject_kind, user_id, channel_identity_id, created_by_user_id, created_at, updated_at;

-- name: DeleteBotACLGuestAllAllowRule :exec
DELETE FROM bot_acl_rules
WHERE bot_id = $1
  AND action = 'chat.trigger'
  AND effect = 'allow'
  AND subject_kind = 'guest_all';

-- name: DeleteBotACLRuleByID :exec
DELETE FROM bot_acl_rules
WHERE id = $1;

-- name: HasBotACLGuestAllAllowRule :one
SELECT EXISTS (
  SELECT 1
  FROM bot_acl_rules
  WHERE bot_id = $1
    AND action = 'chat.trigger'
    AND effect = 'allow'
    AND subject_kind = 'guest_all'
) AS allowed;

-- name: HasBotACLUserRule :one
SELECT EXISTS (
  SELECT 1
  FROM bot_acl_rules
  WHERE bot_id = $1
    AND action = 'chat.trigger'
    AND effect = $2
    AND subject_kind = 'user'
    AND user_id = $3
) AS matched;

-- name: HasBotACLChannelIdentityRule :one
SELECT EXISTS (
  SELECT 1
  FROM bot_acl_rules
  WHERE bot_id = $1
    AND action = 'chat.trigger'
    AND effect = $2
    AND subject_kind = 'channel_identity'
    AND channel_identity_id = $3
) AS matched;

-- name: ListBotACLSubjectRulesByEffect :many
SELECT
  r.id,
  r.bot_id,
  r.action,
  r.effect,
  r.subject_kind,
  r.user_id,
  r.channel_identity_id,
  r.created_by_user_id,
  r.created_at,
  r.updated_at,
  u.username AS user_username,
  u.display_name AS user_display_name,
  u.avatar_url AS user_avatar_url,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name AS channel_identity_display_name,
  ci.avatar_url AS channel_identity_avatar_url,
  linked.id AS linked_user_id,
  linked.username AS linked_user_username,
  linked.display_name AS linked_user_display_name,
  linked.avatar_url AS linked_user_avatar_url
FROM bot_acl_rules r
LEFT JOIN users u ON u.id = r.user_id
LEFT JOIN channel_identities ci ON ci.id = r.channel_identity_id
LEFT JOIN users linked ON linked.id = ci.user_id
WHERE r.bot_id = $1
  AND r.action = 'chat.trigger'
  AND r.effect = $2
  AND r.subject_kind IN ('user', 'channel_identity')
ORDER BY r.created_at DESC;
