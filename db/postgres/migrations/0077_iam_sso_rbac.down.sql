-- 0077_iam_sso_rbac
-- Roll back IAM SSO RBAC schema to the legacy users/password_hash/role model.

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_role') THEN
    CREATE TYPE user_role AS ENUM ('member', 'admin');
  END IF;
END
$$;

ALTER TABLE iam_users ADD COLUMN IF NOT EXISTS password_hash TEXT;
ALTER TABLE iam_users ADD COLUMN IF NOT EXISTS role user_role NOT NULL DEFAULT 'member';

UPDATE iam_users u
SET password_hash = i.credential_secret
FROM iam_identities i
WHERE i.user_id = u.id
  AND i.provider_type = 'password'
  AND i.credential_secret IS NOT NULL;

UPDATE iam_users u
SET role = 'admin'::user_role
FROM iam_principal_roles pr
JOIN iam_roles r ON r.id = pr.role_id
WHERE pr.principal_type = 'user'
  AND pr.principal_id = u.id
  AND pr.resource_type = 'system'
  AND pr.resource_id IS NULL
  AND r.key = 'admin';

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

DROP INDEX IF EXISTS idx_iam_users_email_unique;
DROP INDEX IF EXISTS idx_iam_users_username_unique;

ALTER TABLE iam_users ADD CONSTRAINT users_email_unique UNIQUE (email);
ALTER TABLE iam_users RENAME CONSTRAINT iam_users_username_unique TO users_username_unique;

ALTER TABLE iam_user_provider_oauth_tokens RENAME CONSTRAINT iam_user_provider_oauth_tokens_provider_user_unique TO user_provider_oauth_tokens_provider_user_unique;
ALTER TABLE iam_channel_identity_bind_codes RENAME CONSTRAINT iam_channel_identity_bind_codes_token_unique TO channel_identity_bind_codes_token_unique;
ALTER TABLE iam_user_channel_bindings RENAME CONSTRAINT iam_user_channel_bindings_unique TO user_channel_bindings_unique;
ALTER TABLE iam_channel_identities RENAME CONSTRAINT iam_channel_identities_channel_type_subject_unique TO channel_identities_channel_type_subject_unique;

ALTER INDEX IF EXISTS idx_iam_user_provider_oauth_tokens_state RENAME TO idx_user_provider_oauth_tokens_state;
ALTER INDEX IF EXISTS idx_iam_channel_identity_bind_codes_channel_type RENAME TO idx_channel_identity_bind_codes_channel_type;
ALTER INDEX IF EXISTS idx_iam_user_channel_bindings_user_id RENAME TO idx_user_channel_bindings_user_id;
ALTER INDEX IF EXISTS idx_iam_channel_identities_user_id RENAME TO idx_channel_identities_user_id;

ALTER TABLE iam_user_provider_oauth_tokens RENAME TO user_provider_oauth_tokens;
ALTER TABLE iam_channel_identity_bind_codes RENAME TO channel_identity_bind_codes;
ALTER TABLE iam_user_channel_bindings RENAME TO user_channel_bindings;
ALTER TABLE iam_channel_identities RENAME TO channel_identities;
ALTER TABLE iam_users RENAME TO users;
