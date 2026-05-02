-- 0003_iam_sso_rbac
-- Roll back IAM SSO RBAC schema to the legacy users/password_hash/role model.

PRAGMA foreign_keys = OFF;

ALTER TABLE iam_users ADD COLUMN password_hash TEXT;
ALTER TABLE iam_users ADD COLUMN role TEXT NOT NULL DEFAULT 'member';

UPDATE iam_users
SET password_hash = (
  SELECT credential_secret
  FROM iam_identities
  WHERE iam_identities.user_id = iam_users.id
    AND iam_identities.provider_type = 'password'
    AND iam_identities.credential_secret IS NOT NULL
  LIMIT 1
);

UPDATE iam_users
SET role = 'admin'
WHERE EXISTS (
  SELECT 1
  FROM iam_principal_roles pr
  JOIN iam_roles r ON r.id = pr.role_id
  WHERE pr.principal_type = 'user'
    AND pr.principal_id = iam_users.id
    AND pr.resource_type = 'system'
    AND pr.resource_id IS NULL
    AND r.key = 'admin'
);

DROP TABLE IF EXISTS iam_sso_group_mappings;
DROP TABLE IF EXISTS iam_principal_roles;
DROP TABLE IF EXISTS iam_role_permissions;
DROP TABLE IF EXISTS iam_group_members;
DROP TABLE IF EXISTS iam_login_codes;
DROP TABLE IF EXISTS iam_sessions;
DROP TABLE IF EXISTS iam_identities;
DROP TABLE IF EXISTS iam_groups;
DROP TABLE IF EXISTS iam_roles;
DROP TABLE IF EXISTS iam_permissions;
DROP TABLE IF EXISTS iam_sso_providers;

ALTER TABLE iam_user_provider_oauth_tokens RENAME TO user_provider_oauth_tokens;
ALTER TABLE iam_channel_identity_bind_codes RENAME TO channel_identity_bind_codes;
ALTER TABLE iam_user_channel_bindings RENAME TO user_channel_bindings;
ALTER TABLE iam_channel_identities RENAME TO channel_identities;
ALTER TABLE iam_users RENAME TO users;

PRAGMA foreign_keys = ON;
