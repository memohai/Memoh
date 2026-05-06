-- name: EvaluateBotACLRule :one
-- First-match-wins: returns the effect of the highest-priority matching enabled rule.
-- If no row is returned, the caller falls back to bots.acl_default_effect.
SELECT effect
FROM bot_acl_rules
WHERE bot_id = $1
  AND enabled = true
  AND action = $2
  AND (
    subject_kind = 'all'
    OR (subject_kind = 'channel_identity' AND channel_identity_id = sqlc.narg(channel_identity_id)::uuid)
    OR (subject_kind = 'channel_type' AND subject_channel_type = sqlc.narg(subject_channel_type)::text)
  )
  AND (source_conversation_type IS NULL OR source_conversation_type = sqlc.narg(source_conversation_type)::text)
  AND (source_conversation_id IS NULL OR source_conversation_id = sqlc.narg(source_conversation_id)::text)
  AND (source_thread_id IS NULL OR source_thread_id = sqlc.narg(source_thread_id)::text)
ORDER BY priority ASC, created_at ASC
LIMIT 1;

-- name: GetBotACLDefaultEffect :one
SELECT acl_default_effect FROM bots WHERE id = $1;

-- name: SetBotACLDefaultEffect :exec
UPDATE bots SET acl_default_effect = $2, updated_at = now() WHERE id = $1;

-- name: ListBotACLRules :many
SELECT
  r.id,
  r.bot_id,
  r.priority,
  r.enabled,
  r.description,
  r.action,
  r.effect,
  r.subject_kind,
  r.channel_identity_id,
  r.subject_channel_type,
  r.source_conversation_type,
  r.source_conversation_id,
  r.source_thread_id,
  r.created_by_user_id,
  r.created_at,
  r.updated_at,
  ci.channel_type,
  ci.channel_subject_id,
  ci.display_name AS channel_identity_display_name,
  ci.avatar_url AS channel_identity_avatar_url,
  linked.id AS linked_user_id,
  linked.username AS linked_user_username,
  linked.display_name AS linked_user_display_name,
  linked.avatar_url AS linked_user_avatar_url
FROM bot_acl_rules r
LEFT JOIN iam_channel_identities ci ON ci.id = r.channel_identity_id
LEFT JOIN iam_users linked ON linked.id = ci.user_id
WHERE r.bot_id = $1
  AND r.action = 'chat.trigger'
ORDER BY r.priority ASC, r.created_at ASC;

-- name: CreateBotACLRule :one
INSERT INTO bot_acl_rules (
  bot_id,
  priority,
  enabled,
  description,
  action,
  effect,
  subject_kind,
  channel_identity_id,
  subject_channel_type,
  source_channel,
  source_conversation_type,
  source_conversation_id,
  source_thread_id,
  created_by_user_id
)
VALUES (
  $1,
  $2,
  $3,
  sqlc.narg(description)::text,
  'chat.trigger',
  $4,
  $5,
  sqlc.narg(channel_identity_id)::uuid,
  sqlc.narg(subject_channel_type)::text,
  sqlc.narg(source_channel)::text,
  sqlc.narg(source_conversation_type)::text,
  sqlc.narg(source_conversation_id)::text,
  sqlc.narg(source_thread_id)::text,
  $6
)
RETURNING id, bot_id, priority, enabled, description, action, effect, subject_kind, channel_identity_id, subject_channel_type, source_channel, source_conversation_type, source_conversation_id, source_thread_id, created_by_user_id, created_at, updated_at;

-- name: UpdateBotACLRule :one
UPDATE bot_acl_rules
SET
  priority = $2,
  enabled = $3,
  description = sqlc.narg(description)::text,
  effect = $4,
  subject_kind = $5,
  channel_identity_id = sqlc.narg(channel_identity_id)::uuid,
  subject_channel_type = sqlc.narg(subject_channel_type)::text,
  source_channel = sqlc.narg(source_channel)::text,
  source_conversation_type = sqlc.narg(source_conversation_type)::text,
  source_conversation_id = sqlc.narg(source_conversation_id)::text,
  source_thread_id = sqlc.narg(source_thread_id)::text,
  updated_at = now()
WHERE id = $1
RETURNING id, bot_id, priority, enabled, description, action, effect, subject_kind, channel_identity_id, subject_channel_type, source_channel, source_conversation_type, source_conversation_id, source_thread_id, created_by_user_id, created_at, updated_at;

-- name: UpdateBotACLRulePriority :exec
UPDATE bot_acl_rules SET priority = $2, updated_at = now() WHERE id = $1;

-- name: DeleteBotACLRuleByID :exec
DELETE FROM bot_acl_rules WHERE id = $1;
