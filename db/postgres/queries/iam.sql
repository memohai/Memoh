-- name: CreatePasswordIdentity :one
INSERT INTO iam_identities (
  user_id, provider_type, provider_id, subject, credential_secret, email, username,
  display_name, avatar_url, raw_claims
)
VALUES (
  sqlc.arg(user_id), 'password', NULL, lower(sqlc.arg(subject)::text), sqlc.arg(credential_secret),
  sqlc.narg(email), sqlc.narg(username), sqlc.narg(display_name), sqlc.narg(avatar_url), '{}'::jsonb
)
ON CONFLICT (provider_type, provider_id, subject) DO UPDATE SET
  credential_secret = EXCLUDED.credential_secret,
  email = EXCLUDED.email,
  username = EXCLUDED.username,
  display_name = EXCLUDED.display_name,
  avatar_url = EXCLUDED.avatar_url,
  updated_at = now()
RETURNING *;

-- name: GetIdentityByProviderSubject :one
SELECT *
FROM iam_identities
WHERE provider_type = sqlc.arg(provider_type)
  AND provider_id IS NOT DISTINCT FROM sqlc.narg(provider_id)::uuid
  AND subject = sqlc.arg(subject);

-- name: GetPasswordIdentityBySubject :one
SELECT *
FROM iam_identities
WHERE provider_type = 'password'
  AND subject = lower(sqlc.arg(subject)::text);

-- name: UpsertExternalIdentity :one
INSERT INTO iam_identities (
  user_id, provider_type, provider_id, subject, email, username, display_name, avatar_url, raw_claims
)
VALUES (
  sqlc.arg(user_id), sqlc.arg(provider_type), sqlc.arg(provider_id), sqlc.arg(subject),
  sqlc.narg(email), sqlc.narg(username), sqlc.narg(display_name), sqlc.narg(avatar_url), sqlc.arg(raw_claims)
)
ON CONFLICT (provider_type, provider_id, subject) DO UPDATE SET
  user_id = EXCLUDED.user_id,
  email = EXCLUDED.email,
  username = EXCLUDED.username,
  display_name = EXCLUDED.display_name,
  avatar_url = EXCLUDED.avatar_url,
  raw_claims = EXCLUDED.raw_claims,
  updated_at = now()
RETURNING *;

-- name: UpdateIdentityLastLogin :exec
UPDATE iam_identities
SET last_login_at = now(), updated_at = now()
WHERE id = sqlc.arg(id);

-- name: CreateIAMSession :one
INSERT INTO iam_sessions (user_id, identity_id, expires_at, ip_address, user_agent, metadata)
VALUES (sqlc.arg(user_id), sqlc.narg(identity_id), sqlc.arg(expires_at), sqlc.narg(ip_address), sqlc.narg(user_agent), sqlc.arg(metadata))
RETURNING *;

-- name: GetIAMSessionByID :one
SELECT s.*, u.is_active AS user_is_active
FROM iam_sessions s
JOIN iam_users u ON u.id = s.user_id
WHERE s.id = sqlc.arg(id);

-- name: RevokeIAMSession :exec
UPDATE iam_sessions
SET revoked_at = now(), updated_at = now()
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL;

-- name: ExtendIAMSession :one
UPDATE iam_sessions
SET expires_at = sqlc.arg(expires_at), updated_at = now()
WHERE id = sqlc.arg(id)
  AND revoked_at IS NULL
RETURNING *;

-- name: CreateIAMLoginCode :one
INSERT INTO iam_login_codes (code_hash, user_id, identity_id, session_id, expires_at)
VALUES (sqlc.arg(code_hash), sqlc.arg(user_id), sqlc.narg(identity_id), sqlc.arg(session_id), sqlc.arg(expires_at))
RETURNING *;

-- name: UseIAMLoginCode :one
UPDATE iam_login_codes
SET used_at = now()
WHERE code_hash = sqlc.arg(code_hash)
  AND used_at IS NULL
  AND expires_at > now()
RETURNING *;

-- name: ListSSOProviders :many
SELECT *
FROM iam_sso_providers
ORDER BY created_at DESC;

-- name: ListEnabledSSOProviders :many
SELECT *
FROM iam_sso_providers
WHERE enabled = true
ORDER BY name ASC;

-- name: GetSSOProviderByID :one
SELECT *
FROM iam_sso_providers
WHERE id = sqlc.arg(id);

-- name: GetSSOProviderByKey :one
SELECT *
FROM iam_sso_providers
WHERE key = sqlc.arg(key);

-- name: UpsertSSOProvider :one
INSERT INTO iam_sso_providers (
  id, type, key, name, enabled, config, attribute_mapping, jit_enabled, email_linking_policy, trust_email
)
VALUES (
  sqlc.arg(id), sqlc.arg(type), sqlc.arg(key), sqlc.arg(name), sqlc.arg(enabled), sqlc.arg(config),
  sqlc.arg(attribute_mapping), sqlc.arg(jit_enabled), sqlc.arg(email_linking_policy), sqlc.arg(trust_email)
)
ON CONFLICT (key) DO UPDATE SET
  type = EXCLUDED.type,
  name = EXCLUDED.name,
  enabled = EXCLUDED.enabled,
  config = EXCLUDED.config,
  attribute_mapping = EXCLUDED.attribute_mapping,
  jit_enabled = EXCLUDED.jit_enabled,
  email_linking_policy = EXCLUDED.email_linking_policy,
  trust_email = EXCLUDED.trust_email,
  updated_at = now()
RETURNING *;

-- name: DeleteSSOProvider :exec
DELETE FROM iam_sso_providers WHERE id = sqlc.arg(id);

-- name: ListIAMGroups :many
SELECT *
FROM iam_groups
ORDER BY key ASC;

-- name: GetIAMGroupByID :one
SELECT *
FROM iam_groups
WHERE id = sqlc.arg(id);

-- name: UpsertIAMGroup :one
INSERT INTO iam_groups (id, key, display_name, source, external_id, metadata)
VALUES (sqlc.arg(id), sqlc.arg(key), sqlc.arg(display_name), sqlc.arg(source), sqlc.narg(external_id), sqlc.arg(metadata))
ON CONFLICT (key) DO UPDATE SET
  display_name = EXCLUDED.display_name,
  source = EXCLUDED.source,
  external_id = EXCLUDED.external_id,
  metadata = EXCLUDED.metadata,
  updated_at = now()
RETURNING *;

-- name: DeleteIAMGroup :exec
DELETE FROM iam_groups
WHERE id = sqlc.arg(id);

-- name: ListIAMGroupMembers :many
SELECT gm.*, u.username, u.email, u.display_name
FROM iam_group_members gm
JOIN iam_users u ON u.id = gm.user_id
WHERE gm.group_id = sqlc.arg(group_id)
ORDER BY u.username ASC, u.email ASC;

-- name: UpsertIAMGroupMember :one
INSERT INTO iam_group_members (user_id, group_id, source, provider_id)
VALUES (sqlc.arg(user_id), sqlc.arg(group_id), sqlc.arg(source), sqlc.narg(provider_id))
ON CONFLICT (user_id, group_id, source, provider_id) DO UPDATE SET
  updated_at = now()
RETURNING *;

-- name: DeleteIAMGroupMember :exec
DELETE FROM iam_group_members
WHERE user_id = sqlc.arg(user_id)
  AND group_id = sqlc.arg(group_id);

-- name: ListSSOGroupMappingsByProvider :many
SELECT m.*, g.key AS group_key, g.display_name AS group_display_name
FROM iam_sso_group_mappings m
JOIN iam_groups g ON g.id = m.group_id
WHERE m.provider_id = sqlc.arg(provider_id)
ORDER BY m.external_group ASC;

-- name: UpsertSSOGroupMapping :one
INSERT INTO iam_sso_group_mappings (provider_id, external_group, group_id)
VALUES (sqlc.arg(provider_id), sqlc.arg(external_group), sqlc.arg(group_id))
ON CONFLICT (provider_id, external_group) DO UPDATE SET
  group_id = EXCLUDED.group_id,
  updated_at = now()
RETURNING *;

-- name: DeleteSSOGroupMapping :exec
DELETE FROM iam_sso_group_mappings
WHERE provider_id = sqlc.arg(provider_id)
  AND external_group = sqlc.arg(external_group);

-- name: ReplaceSSOGroupMemberships :exec
WITH deleted AS (
  DELETE FROM iam_group_members
  WHERE user_id = sqlc.arg(user_id)
    AND source = 'sso'
    AND provider_id = sqlc.arg(provider_id)
)
INSERT INTO iam_group_members (user_id, group_id, source, provider_id)
SELECT sqlc.arg(user_id), unnest(sqlc.arg(group_ids)::uuid[]), 'sso', sqlc.arg(provider_id);

-- name: AssignPrincipalRole :one
INSERT INTO iam_principal_roles (
  principal_type, principal_id, role_id, resource_type, resource_id, source, provider_id
)
VALUES (
  sqlc.arg(principal_type), sqlc.arg(principal_id), sqlc.arg(role_id), sqlc.arg(resource_type),
  sqlc.narg(resource_id), sqlc.arg(source), sqlc.narg(provider_id)
)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: DeletePrincipalRole :exec
DELETE FROM iam_principal_roles
WHERE id = sqlc.arg(id);

-- name: ListPrincipalRoles :many
SELECT
  pr.*,
  r.key AS role_key,
  r.scope AS role_scope,
  u.username AS user_username,
  u.email AS user_email,
  u.display_name AS user_display_name,
  g.key AS group_key,
  g.display_name AS group_display_name
FROM iam_principal_roles pr
JOIN iam_roles r ON r.id = pr.role_id
LEFT JOIN iam_users u ON pr.principal_type = 'user' AND u.id = pr.principal_id
LEFT JOIN iam_groups g ON pr.principal_type = 'group' AND g.id = pr.principal_id
WHERE pr.resource_type = sqlc.arg(resource_type)
  AND pr.resource_id IS NOT DISTINCT FROM sqlc.narg(resource_id)::uuid
ORDER BY r.key ASC, pr.principal_type ASC, user_username ASC, group_key ASC;

-- name: DeletePrincipalRoleAssignment :exec
DELETE FROM iam_principal_roles pr
USING iam_roles r
WHERE pr.role_id = r.id
  AND pr.principal_type = sqlc.arg(principal_type)
  AND pr.principal_id = sqlc.arg(principal_id)
  AND pr.resource_type = sqlc.arg(resource_type)
  AND pr.resource_id = sqlc.narg(resource_id)
  AND r.key = sqlc.arg(role_key);

-- name: HasPermission :one
SELECT EXISTS (
  SELECT 1
  FROM iam_principal_roles pr
  JOIN iam_roles r ON r.id = pr.role_id
  JOIN iam_role_permissions rp ON rp.role_id = r.id
  JOIN iam_permissions p ON p.id = rp.permission_id
  WHERE p.key = sqlc.arg(permission_key)
    AND pr.resource_type = sqlc.arg(resource_type)
    AND (pr.resource_id = sqlc.narg(resource_id)::uuid OR pr.resource_id IS NULL)
    AND (
      (pr.principal_type = 'user' AND pr.principal_id = sqlc.arg(user_id))
      OR (
        pr.principal_type = 'group'
        AND pr.principal_id IN (
          SELECT group_id FROM iam_group_members WHERE user_id = sqlc.arg(user_id)
        )
      )
    )
) AS allowed;

-- name: GetRoleByKey :one
SELECT *
FROM iam_roles
WHERE key = sqlc.arg(key);

-- name: ListRoles :many
SELECT *
FROM iam_roles
ORDER BY scope ASC, key ASC;
