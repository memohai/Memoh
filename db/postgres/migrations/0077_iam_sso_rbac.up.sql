-- 0077_iam_sso_rbac
-- Introduce IAM identities, sessions, SSO providers, groups, and RBAC assignments.

ALTER TABLE IF EXISTS users RENAME TO iam_users;
ALTER TABLE IF EXISTS channel_identities RENAME TO iam_channel_identities;
ALTER TABLE IF EXISTS user_channel_bindings RENAME TO iam_user_channel_bindings;
ALTER TABLE IF EXISTS channel_identity_bind_codes RENAME TO iam_channel_identity_bind_codes;
ALTER TABLE IF EXISTS user_provider_oauth_tokens RENAME TO iam_user_provider_oauth_tokens;

ALTER INDEX IF EXISTS idx_channel_identities_user_id RENAME TO idx_iam_channel_identities_user_id;
ALTER INDEX IF EXISTS idx_user_channel_bindings_user_id RENAME TO idx_iam_user_channel_bindings_user_id;
ALTER INDEX IF EXISTS idx_channel_identity_bind_codes_channel_type RENAME TO idx_iam_channel_identity_bind_codes_channel_type;
ALTER INDEX IF EXISTS idx_user_provider_oauth_tokens_state RENAME TO idx_iam_user_provider_oauth_tokens_state;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_email_unique') THEN
    ALTER TABLE iam_users RENAME CONSTRAINT users_email_unique TO iam_users_email_unique;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'users_username_unique') THEN
    ALTER TABLE iam_users RENAME CONSTRAINT users_username_unique TO iam_users_username_unique;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'channel_identities_channel_type_subject_unique') THEN
    ALTER TABLE iam_channel_identities RENAME CONSTRAINT channel_identities_channel_type_subject_unique TO iam_channel_identities_channel_type_subject_unique;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'user_channel_bindings_unique') THEN
    ALTER TABLE iam_user_channel_bindings RENAME CONSTRAINT user_channel_bindings_unique TO iam_user_channel_bindings_unique;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'channel_identity_bind_codes_token_unique') THEN
    ALTER TABLE iam_channel_identity_bind_codes RENAME CONSTRAINT channel_identity_bind_codes_token_unique TO iam_channel_identity_bind_codes_token_unique;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'user_provider_oauth_tokens_provider_user_unique') THEN
    ALTER TABLE iam_user_provider_oauth_tokens RENAME CONSTRAINT user_provider_oauth_tokens_provider_user_unique TO iam_user_provider_oauth_tokens_provider_user_unique;
  END IF;
END
$$;

ALTER TABLE iam_users DROP CONSTRAINT IF EXISTS iam_users_email_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_users_email_unique
  ON iam_users(email)
  WHERE email IS NOT NULL AND email <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_users_username_unique
  ON iam_users(username)
  WHERE username IS NOT NULL AND username <> '';

CREATE TABLE IF NOT EXISTS iam_sso_providers (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type TEXT NOT NULL,
  key TEXT NOT NULL,
  name TEXT NOT NULL,
  enabled BOOLEAN NOT NULL DEFAULT true,
  config JSONB NOT NULL DEFAULT '{}'::jsonb,
  attribute_mapping JSONB NOT NULL DEFAULT '{}'::jsonb,
  jit_enabled BOOLEAN NOT NULL DEFAULT true,
  email_linking_policy TEXT NOT NULL DEFAULT 'link_existing',
  trust_email BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_sso_providers_type_check CHECK (type IN ('oidc', 'saml')),
  CONSTRAINT iam_sso_providers_key_unique UNIQUE (key),
  CONSTRAINT iam_sso_providers_email_linking_policy_check CHECK (email_linking_policy IN ('link_existing', 'reject_existing'))
);

CREATE TABLE IF NOT EXISTS iam_identities (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  provider_type TEXT NOT NULL,
  provider_id UUID REFERENCES iam_sso_providers(id) ON DELETE CASCADE,
  subject TEXT NOT NULL,
  credential_secret TEXT,
  email TEXT,
  username TEXT,
  display_name TEXT,
  avatar_url TEXT,
  raw_claims JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_login_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_identities_provider_type_check CHECK (provider_type IN ('password', 'oidc', 'saml', 'channel')),
  CONSTRAINT iam_identities_password_provider_check CHECK (
    (provider_type = 'password' AND provider_id IS NULL AND credential_secret IS NOT NULL)
    OR provider_type <> 'password'
  ),
  CONSTRAINT iam_identities_provider_subject_unique UNIQUE NULLS NOT DISTINCT (provider_type, provider_id, subject)
);
CREATE INDEX IF NOT EXISTS idx_iam_identities_user_id ON iam_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_identities_email ON iam_identities(email) WHERE email IS NOT NULL AND email <> '';

CREATE TABLE IF NOT EXISTS iam_sessions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  identity_id UUID REFERENCES iam_identities(id) ON DELETE SET NULL,
  issued_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ,
  ip_address TEXT,
  user_agent TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_iam_sessions_user_id ON iam_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_sessions_active ON iam_sessions(user_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS iam_login_codes (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  code_hash TEXT NOT NULL,
  user_id UUID NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  identity_id UUID REFERENCES iam_identities(id) ON DELETE SET NULL,
  session_id UUID NOT NULL REFERENCES iam_sessions(id) ON DELETE CASCADE,
  expires_at TIMESTAMPTZ NOT NULL,
  used_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_login_codes_code_hash_unique UNIQUE (code_hash)
);
CREATE INDEX IF NOT EXISTS idx_iam_login_codes_expires_at ON iam_login_codes(expires_at);

CREATE TABLE IF NOT EXISTS iam_groups (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'local',
  external_id TEXT,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_groups_key_unique UNIQUE (key),
  CONSTRAINT iam_groups_source_check CHECK (source IN ('local', 'sso', 'scim'))
);

CREATE TABLE IF NOT EXISTS iam_group_members (
  user_id UUID NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  group_id UUID NOT NULL REFERENCES iam_groups(id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT 'manual',
  provider_id UUID REFERENCES iam_sso_providers(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_group_members_source_check CHECK (source IN ('manual', 'sso', 'scim')),
  CONSTRAINT iam_group_members_unique UNIQUE NULLS NOT DISTINCT (user_id, group_id, source, provider_id)
);
CREATE INDEX IF NOT EXISTS idx_iam_group_members_user_id ON iam_group_members(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_group_members_group_id ON iam_group_members(group_id);

CREATE TABLE IF NOT EXISTS iam_permissions (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  is_system BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_permissions_key_unique UNIQUE (key)
);

CREATE TABLE IF NOT EXISTS iam_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  key TEXT NOT NULL,
  scope TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  is_system BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_roles_key_unique UNIQUE (key),
  CONSTRAINT iam_roles_scope_check CHECK (scope IN ('system', 'bot'))
);

CREATE TABLE IF NOT EXISTS iam_role_permissions (
  role_id UUID NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
  permission_id UUID NOT NULL REFERENCES iam_permissions(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS iam_principal_roles (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  principal_type TEXT NOT NULL,
  principal_id UUID NOT NULL,
  role_id UUID NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
  resource_type TEXT NOT NULL,
  resource_id UUID,
  source TEXT NOT NULL DEFAULT 'manual',
  provider_id UUID REFERENCES iam_sso_providers(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_principal_roles_principal_type_check CHECK (principal_type IN ('user', 'group')),
  CONSTRAINT iam_principal_roles_resource_type_check CHECK (resource_type IN ('system', 'bot')),
  CONSTRAINT iam_principal_roles_source_check CHECK (source IN ('system', 'manual', 'sso', 'scim')),
  CONSTRAINT iam_principal_roles_resource_id_check CHECK (
    (resource_type = 'system' AND resource_id IS NULL)
    OR resource_type = 'bot'
  ),
  CONSTRAINT iam_principal_roles_unique UNIQUE NULLS NOT DISTINCT (
    principal_type, principal_id, role_id, resource_type, resource_id, source, provider_id
  )
);
CREATE INDEX IF NOT EXISTS idx_iam_principal_roles_user ON iam_principal_roles(principal_id) WHERE principal_type = 'user';
CREATE INDEX IF NOT EXISTS idx_iam_principal_roles_group ON iam_principal_roles(principal_id) WHERE principal_type = 'group';
CREATE INDEX IF NOT EXISTS idx_iam_principal_roles_resource ON iam_principal_roles(resource_type, resource_id);

CREATE TABLE IF NOT EXISTS iam_sso_group_mappings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  provider_id UUID NOT NULL REFERENCES iam_sso_providers(id) ON DELETE CASCADE,
  external_group TEXT NOT NULL,
  group_id UUID NOT NULL REFERENCES iam_groups(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT iam_sso_group_mappings_unique UNIQUE (provider_id, external_group)
);
CREATE INDEX IF NOT EXISTS idx_iam_sso_group_mappings_group_id ON iam_sso_group_mappings(group_id);

INSERT INTO iam_permissions (key, description, is_system)
VALUES
  ('system.login', 'Log in to the system', true),
  ('system.admin', 'Administer the system', true),
  ('bot.read', 'Read bot data', true),
  ('bot.chat', 'Chat with bot', true),
  ('bot.update', 'Update bot configuration', true),
  ('bot.delete', 'Delete bot', true),
  ('bot.permissions.manage', 'Manage bot permissions', true)
ON CONFLICT (key) DO NOTHING;

INSERT INTO iam_roles (key, scope, description, is_system)
VALUES
  ('member', 'system', 'Default authenticated user', true),
  ('admin', 'system', 'System administrator', true),
  ('bot_viewer', 'bot', 'Bot viewer', true),
  ('bot_operator', 'bot', 'Bot operator', true),
  ('bot_owner', 'bot', 'Bot owner', true)
ON CONFLICT (key) DO NOTHING;

INSERT INTO iam_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM iam_roles r
JOIN iam_permissions p ON (
  (r.key = 'member' AND p.key IN ('system.login')) OR
  (r.key = 'admin' AND p.key IN ('system.login', 'system.admin')) OR
  (r.key = 'bot_viewer' AND p.key IN ('bot.read', 'bot.chat')) OR
  (r.key = 'bot_operator' AND p.key IN ('bot.read', 'bot.chat', 'bot.update')) OR
  (r.key = 'bot_owner' AND p.key IN ('bot.read', 'bot.chat', 'bot.update', 'bot.delete', 'bot.permissions.manage'))
)
ON CONFLICT DO NOTHING;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'iam_users' AND column_name = 'password_hash'
  ) THEN
    EXECUTE $migrate_password$
      INSERT INTO iam_identities (
        user_id, provider_type, provider_id, subject, credential_secret, email, username,
        display_name, avatar_url, raw_claims, last_login_at, created_at, updated_at
      )
      SELECT
        id,
        'password',
        NULL,
        lower(username),
        password_hash,
        email,
        username,
        display_name,
        avatar_url,
        '{}'::jsonb,
        last_login_at,
        created_at,
        updated_at
      FROM iam_users
      WHERE username IS NOT NULL AND username <> ''
        AND password_hash IS NOT NULL AND password_hash <> ''
      ON CONFLICT DO NOTHING
    $migrate_password$;
  END IF;

  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'iam_users' AND column_name = 'role'
  ) THEN
    EXECUTE $migrate_roles$
      INSERT INTO iam_principal_roles (principal_type, principal_id, role_id, resource_type, resource_id, source)
      SELECT 'user', u.id, r.id, 'system', NULL, 'system'
      FROM iam_users u
      JOIN iam_roles r ON r.key = CASE WHEN u.role::text = 'admin' THEN 'admin' ELSE 'member' END
      ON CONFLICT DO NOTHING
    $migrate_roles$;
  ELSE
    INSERT INTO iam_principal_roles (principal_type, principal_id, role_id, resource_type, resource_id, source)
    SELECT 'user', u.id, r.id, 'system', NULL, 'system'
    FROM iam_users u
    JOIN iam_roles r ON r.key = 'member'
    ON CONFLICT DO NOTHING;
  END IF;
END
$$;

INSERT INTO iam_principal_roles (principal_type, principal_id, role_id, resource_type, resource_id, source)
SELECT 'user', b.owner_user_id, r.id, 'bot', b.id, 'system'
FROM bots b
JOIN iam_roles r ON r.key = 'bot_owner'
WHERE b.owner_user_id IS NOT NULL
ON CONFLICT DO NOTHING;

ALTER TABLE iam_users DROP COLUMN IF EXISTS password_hash;
ALTER TABLE iam_users DROP COLUMN IF EXISTS role;
DROP TYPE IF EXISTS user_role;
