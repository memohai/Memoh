-- 0003_iam_sso_rbac
-- Introduce IAM identities, sessions, SSO providers, groups, and RBAC assignments.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT,
  email TEXT,
  password_hash TEXT,
  role TEXT NOT NULL DEFAULT 'member',
  display_name TEXT,
  avatar_url TEXT,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  data_root TEXT,
  last_login_at TEXT,
  is_active INTEGER NOT NULL DEFAULT 1,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (email),
  UNIQUE (username)
);

CREATE TABLE IF NOT EXISTS channel_identities (
  id TEXT PRIMARY KEY,
  user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  channel_type TEXT NOT NULL,
  channel_subject_id TEXT NOT NULL,
  display_name TEXT,
  avatar_url TEXT,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (channel_type, channel_subject_id)
);

CREATE TABLE IF NOT EXISTS user_channel_bindings (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  channel_type TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (user_id, channel_type)
);

CREATE TABLE IF NOT EXISTS channel_identity_bind_codes (
  id TEXT PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  issued_by_user_id TEXT NOT NULL REFERENCES users(id),
  channel_type TEXT,
  expires_at TEXT,
  used_at TEXT,
  used_by_channel_identity_id TEXT REFERENCES channel_identities(id),
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_provider_oauth_tokens (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  scope TEXT NOT NULL DEFAULT '',
  token_type TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  pkce_code_verifier TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (provider_id, user_id)
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
DROP TABLE IF EXISTS iam_user_provider_oauth_tokens;
DROP TABLE IF EXISTS iam_channel_identity_bind_codes;
DROP TABLE IF EXISTS iam_user_channel_bindings;
DROP TABLE IF EXISTS iam_channel_identities;
DROP TABLE IF EXISTS iam_users;

ALTER TABLE users RENAME TO iam_users;
ALTER TABLE channel_identities RENAME TO iam_channel_identities;
ALTER TABLE user_channel_bindings RENAME TO iam_user_channel_bindings;
ALTER TABLE channel_identity_bind_codes RENAME TO iam_channel_identity_bind_codes;
ALTER TABLE user_provider_oauth_tokens RENAME TO iam_user_provider_oauth_tokens;

CREATE TABLE IF NOT EXISTS iam_sso_providers (
  id TEXT PRIMARY KEY,
  type TEXT NOT NULL CHECK (type IN ('oidc', 'saml')),
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  attribute_mapping TEXT NOT NULL DEFAULT '{}',
  jit_enabled INTEGER NOT NULL DEFAULT 1,
  email_linking_policy TEXT NOT NULL DEFAULT 'link_existing' CHECK (email_linking_policy IN ('link_existing', 'reject_existing')),
  trust_email INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS iam_identities (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  provider_type TEXT NOT NULL CHECK (provider_type IN ('password', 'oidc', 'saml', 'channel')),
  provider_id TEXT REFERENCES iam_sso_providers(id) ON DELETE CASCADE,
  subject TEXT NOT NULL,
  credential_secret TEXT,
  email TEXT,
  username TEXT,
  display_name TEXT,
  avatar_url TEXT,
  raw_claims TEXT NOT NULL DEFAULT '{}',
  last_login_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT iam_identities_password_provider_check CHECK (
    (provider_type = 'password' AND provider_id IS NULL AND credential_secret IS NOT NULL)
    OR provider_type <> 'password'
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_identities_provider_subject_unique
  ON iam_identities(provider_type, COALESCE(provider_id, ''), subject);
CREATE INDEX IF NOT EXISTS idx_iam_identities_user_id ON iam_identities(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_identities_email ON iam_identities(email) WHERE email IS NOT NULL AND email != '';

CREATE TABLE IF NOT EXISTS iam_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  identity_id TEXT REFERENCES iam_identities(id) ON DELETE SET NULL,
  issued_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at TEXT NOT NULL,
  revoked_at TEXT,
  ip_address TEXT,
  user_agent TEXT,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_iam_sessions_user_id ON iam_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_sessions_active ON iam_sessions(user_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS iam_login_codes (
  id TEXT PRIMARY KEY,
  code_hash TEXT NOT NULL UNIQUE,
  user_id TEXT NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  identity_id TEXT REFERENCES iam_identities(id) ON DELETE SET NULL,
  session_id TEXT NOT NULL REFERENCES iam_sessions(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  used_at TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_iam_login_codes_expires_at ON iam_login_codes(expires_at);

CREATE TABLE IF NOT EXISTS iam_groups (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'local' CHECK (source IN ('local', 'sso', 'scim')),
  external_id TEXT,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS iam_group_members (
  user_id TEXT NOT NULL REFERENCES iam_users(id) ON DELETE CASCADE,
  group_id TEXT NOT NULL REFERENCES iam_groups(id) ON DELETE CASCADE,
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'sso', 'scim')),
  provider_id TEXT REFERENCES iam_sso_providers(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_group_members_unique
  ON iam_group_members(user_id, group_id, source, COALESCE(provider_id, ''));
CREATE INDEX IF NOT EXISTS idx_iam_group_members_user_id ON iam_group_members(user_id);
CREATE INDEX IF NOT EXISTS idx_iam_group_members_group_id ON iam_group_members(group_id);

CREATE TABLE IF NOT EXISTS iam_permissions (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  description TEXT NOT NULL DEFAULT '',
  is_system INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS iam_roles (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  scope TEXT NOT NULL CHECK (scope IN ('system', 'bot')),
  description TEXT NOT NULL DEFAULT '',
  is_system INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS iam_role_permissions (
  role_id TEXT NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
  permission_id TEXT NOT NULL REFERENCES iam_permissions(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE IF NOT EXISTS iam_principal_roles (
  id TEXT PRIMARY KEY,
  principal_type TEXT NOT NULL CHECK (principal_type IN ('user', 'group')),
  principal_id TEXT NOT NULL,
  role_id TEXT NOT NULL REFERENCES iam_roles(id) ON DELETE CASCADE,
  resource_type TEXT NOT NULL CHECK (resource_type IN ('system', 'bot')),
  resource_id TEXT,
  source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('system', 'manual', 'sso', 'scim')),
  provider_id TEXT REFERENCES iam_sso_providers(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT iam_principal_roles_resource_id_check CHECK (
    (resource_type = 'system' AND resource_id IS NULL)
    OR resource_type = 'bot'
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_principal_roles_unique
  ON iam_principal_roles(principal_type, principal_id, role_id, resource_type, COALESCE(resource_id, ''), source, COALESCE(provider_id, ''));
CREATE INDEX IF NOT EXISTS idx_iam_principal_roles_user ON iam_principal_roles(principal_id) WHERE principal_type = 'user';
CREATE INDEX IF NOT EXISTS idx_iam_principal_roles_group ON iam_principal_roles(principal_id) WHERE principal_type = 'group';
CREATE INDEX IF NOT EXISTS idx_iam_principal_roles_resource ON iam_principal_roles(resource_type, resource_id);

CREATE TABLE IF NOT EXISTS iam_sso_group_mappings (
  id TEXT PRIMARY KEY,
  provider_id TEXT NOT NULL REFERENCES iam_sso_providers(id) ON DELETE CASCADE,
  external_group TEXT NOT NULL,
  group_id TEXT NOT NULL REFERENCES iam_groups(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE (provider_id, external_group)
);
CREATE INDEX IF NOT EXISTS idx_iam_sso_group_mappings_group_id ON iam_sso_group_mappings(group_id);

INSERT OR IGNORE INTO iam_permissions (id, key, description, is_system) VALUES
  ('00000000-0000-0000-0000-000000000001', 'system.login', 'Log in to the system', 1),
  ('00000000-0000-0000-0000-000000000002', 'system.admin', 'Administer the system', 1),
  ('00000000-0000-0000-0000-000000000003', 'bot.read', 'Read bot data', 1),
  ('00000000-0000-0000-0000-000000000004', 'bot.chat', 'Chat with bot', 1),
  ('00000000-0000-0000-0000-000000000005', 'bot.update', 'Update bot configuration', 1),
  ('00000000-0000-0000-0000-000000000006', 'bot.delete', 'Delete bot', 1),
  ('00000000-0000-0000-0000-000000000007', 'bot.permissions.manage', 'Manage bot permissions', 1);

INSERT OR IGNORE INTO iam_roles (id, key, scope, description, is_system) VALUES
  ('00000000-0000-0000-0000-000000000101', 'member', 'system', 'Default authenticated user', 1),
  ('00000000-0000-0000-0000-000000000102', 'admin', 'system', 'System administrator', 1),
  ('00000000-0000-0000-0000-000000000103', 'bot_viewer', 'bot', 'Bot viewer', 1),
  ('00000000-0000-0000-0000-000000000104', 'bot_operator', 'bot', 'Bot operator', 1),
  ('00000000-0000-0000-0000-000000000105', 'bot_owner', 'bot', 'Bot owner', 1);

INSERT OR IGNORE INTO iam_role_permissions (role_id, permission_id) VALUES
  ('00000000-0000-0000-0000-000000000101', '00000000-0000-0000-0000-000000000001'),
  ('00000000-0000-0000-0000-000000000102', '00000000-0000-0000-0000-000000000001'),
  ('00000000-0000-0000-0000-000000000102', '00000000-0000-0000-0000-000000000002'),
  ('00000000-0000-0000-0000-000000000103', '00000000-0000-0000-0000-000000000003'),
  ('00000000-0000-0000-0000-000000000103', '00000000-0000-0000-0000-000000000004'),
  ('00000000-0000-0000-0000-000000000104', '00000000-0000-0000-0000-000000000003'),
  ('00000000-0000-0000-0000-000000000104', '00000000-0000-0000-0000-000000000004'),
  ('00000000-0000-0000-0000-000000000104', '00000000-0000-0000-0000-000000000005'),
  ('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000003'),
  ('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000004'),
  ('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000005'),
  ('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000006'),
  ('00000000-0000-0000-0000-000000000105', '00000000-0000-0000-0000-000000000007');

INSERT OR IGNORE INTO iam_identities (
  id, user_id, provider_type, provider_id, subject, credential_secret, email, username,
  display_name, avatar_url, raw_claims, last_login_at, created_at, updated_at
)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || '4' || substr(lower(hex(randomblob(2))), 2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
  id, 'password', NULL, lower(username), password_hash, email, username, display_name, avatar_url, '{}', last_login_at, created_at, updated_at
FROM iam_users
WHERE username IS NOT NULL AND username != ''
  AND password_hash IS NOT NULL AND password_hash != '';

INSERT OR IGNORE INTO iam_principal_roles (id, principal_type, principal_id, role_id, resource_type, resource_id, source)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || '4' || substr(lower(hex(randomblob(2))), 2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
  'user',
  id,
  CASE WHEN role = 'admin' THEN '00000000-0000-0000-0000-000000000102' ELSE '00000000-0000-0000-0000-000000000101' END,
  'system',
  NULL,
  'system'
FROM iam_users;

INSERT OR IGNORE INTO iam_principal_roles (id, principal_type, principal_id, role_id, resource_type, resource_id, source)
SELECT
  lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-' || '4' || substr(lower(hex(randomblob(2))), 2) || '-' || substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' || lower(hex(randomblob(6))),
  'user',
  owner_user_id,
  '00000000-0000-0000-0000-000000000105',
  'bot',
  id,
  'system'
FROM bots
WHERE owner_user_id IS NOT NULL;

CREATE TABLE iam_users_new (
  id TEXT PRIMARY KEY,
  username TEXT,
  email TEXT,
  display_name TEXT,
  avatar_url TEXT,
  timezone TEXT NOT NULL DEFAULT 'UTC',
  data_root TEXT,
  last_login_at TEXT,
  is_active INTEGER NOT NULL DEFAULT 1,
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT iam_users_username_unique UNIQUE (username)
);
INSERT INTO iam_users_new (
  id, username, email, display_name, avatar_url, timezone, data_root, last_login_at,
  is_active, metadata, created_at, updated_at
)
SELECT
  id, username, email, display_name, avatar_url, timezone, data_root, last_login_at,
  is_active, metadata, created_at, updated_at
FROM iam_users;
DROP TABLE iam_users;
ALTER TABLE iam_users_new RENAME TO iam_users;
CREATE UNIQUE INDEX IF NOT EXISTS idx_iam_users_email_unique ON iam_users(email) WHERE email IS NOT NULL AND email != '';

PRAGMA foreign_keys = ON;
