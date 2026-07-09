-- name: EvaluateBotACLRule :one
-- Mode-based ACL: only rules opposite to bots.acl_default_effect can override the default.
-- If no matching override exists, returns bots.acl_default_effect.
SELECT COALESCE((
  SELECT r.effect
  FROM bot_acl_rules r
  WHERE r.bot_id = b.id
    AND r.team_id = sqlc.arg(team_id)::uuid
    AND r.team_id = b.team_id
    AND r.enabled = true
    AND r.action = sqlc.arg(action)
    AND r.effect <> b.acl_default_effect
    AND (r.subject_channel_type IS NULL OR r.subject_channel_type = sqlc.narg(subject_channel_type)::text)
    AND (
      r.channel_identity_id IS NULL
      OR (
        r.channel_identity_id = sqlc.narg(channel_identity_id)::uuid
        AND EXISTS (
          SELECT 1
          FROM channel_identities ci
          WHERE ci.id = r.channel_identity_id
            AND ci.team_id = b.team_id
        )
      )
    )
    AND (r.source_conversation_type IS NULL OR r.source_conversation_type = sqlc.narg(source_conversation_type)::text)
    AND (r.source_conversation_id IS NULL OR r.source_conversation_id = sqlc.narg(source_conversation_id)::text)
    AND (r.source_thread_id IS NULL OR r.source_thread_id = sqlc.narg(source_thread_id)::text)
  LIMIT 1
), b.acl_default_effect) AS effect
FROM bots b
WHERE b.id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)::uuid;

-- name: GetBotACLDefaultEffect :one
SELECT acl_default_effect
FROM bots
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: SetBotACLDefaultEffect :exec
UPDATE bots
SET acl_default_effect = sqlc.arg(acl_default_effect),
    updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: ListBotACLRules :many
SELECT
  r.id,
  r.bot_id,
  r.enabled,
  r.description,
  r.action,
  r.effect,
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
  COALESCE(
    NULLIF(TRIM(COALESCE(source_route.metadata->>'conversation_name', '')), ''),
    NULLIF(TRIM(COALESCE(source_route.metadata->>'conversation_handle', '')), ''),
    ''
  )::text AS source_conversation_name,
  COALESCE(NULLIF(TRIM(COALESCE(source_route.metadata->>'conversation_avatar_url', '')), ''), '')::text AS source_conversation_avatar_url
FROM bot_acl_rules r
INNER JOIN bots bot_scope ON bot_scope.id = r.bot_id AND bot_scope.team_id = r.team_id
LEFT JOIN channel_identities ci ON ci.id = r.channel_identity_id
  AND ci.team_id = r.team_id
LEFT JOIN bot_channel_routes source_route ON source_route.bot_id = r.bot_id
  AND source_route.team_id = r.team_id
  AND r.source_conversation_id IS NOT NULL
  AND source_route.external_conversation_id = r.source_conversation_id
  AND COALESCE(source_route.external_thread_id, '') = COALESCE(r.source_thread_id, '')
  AND (r.source_channel IS NULL OR source_route.channel_type = r.source_channel)
WHERE r.bot_id = sqlc.arg(bot_id)
  AND r.team_id = sqlc.arg(team_id)::uuid
  AND r.action = 'chat.trigger'
ORDER BY r.created_at DESC;

-- name: CreateBotACLRule :one
INSERT INTO bot_acl_rules (
  team_id,
  bot_id,
  enabled,
  description,
  action,
  effect,
  channel_identity_id,
  subject_channel_type,
  source_channel,
  source_conversation_type,
  source_conversation_id,
  source_thread_id,
  created_by_user_id
)
SELECT
  b.team_id,
  b.id,
  sqlc.arg(enabled),
  sqlc.narg(description)::text,
  'chat.trigger',
  sqlc.arg(effect),
  sqlc.narg(channel_identity_id)::uuid,
  sqlc.narg(subject_channel_type)::text,
  sqlc.narg(source_channel)::text,
  sqlc.narg(source_conversation_type)::text,
  sqlc.narg(source_conversation_id)::text,
  sqlc.narg(source_thread_id)::text,
  sqlc.arg(created_by_user_id)
FROM bots b
LEFT JOIN channel_identities ci ON ci.id = sqlc.narg(channel_identity_id)::uuid
WHERE b.id = sqlc.arg(bot_id)
  AND b.team_id = sqlc.arg(team_id)::uuid
  AND (
    sqlc.narg(channel_identity_id)::uuid IS NULL
    OR ci.team_id = b.team_id
  )
RETURNING *;

-- name: UpdateBotACLRule :one
UPDATE bot_acl_rules
SET
  enabled = sqlc.arg(enabled),
  description = sqlc.narg(description)::text,
  effect = sqlc.arg(effect),
  channel_identity_id = sqlc.narg(channel_identity_id)::uuid,
  subject_channel_type = sqlc.narg(subject_channel_type)::text,
  source_channel = sqlc.narg(source_channel)::text,
  source_conversation_type = sqlc.narg(source_conversation_type)::text,
  source_conversation_id = sqlc.narg(source_conversation_id)::text,
  source_thread_id = sqlc.narg(source_thread_id)::text,
  updated_at = now()
WHERE bot_acl_rules.id = sqlc.arg(id)
  AND bot_acl_rules.bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid
  AND (
    sqlc.narg(channel_identity_id)::uuid IS NULL
    OR EXISTS (
      SELECT 1
      FROM channel_identities ci
      WHERE ci.id = sqlc.narg(channel_identity_id)::uuid
        AND ci.team_id = bot_acl_rules.team_id
    )
  )
RETURNING *;

-- name: DeleteBotACLRuleByID :exec
DELETE FROM bot_acl_rules
WHERE id = sqlc.arg(id)
  AND bot_id = sqlc.arg(bot_id)
  AND team_id = sqlc.arg(team_id)::uuid;
