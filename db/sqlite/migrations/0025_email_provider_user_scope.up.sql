-- 0025_email_provider_user_scope
-- Scope email providers to users and move Gmail OAuth client secrets to server config.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  username TEXT,
  role TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bots (
  id TEXT PRIMARY KEY,
  owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS email_providers (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT email_providers_name_unique UNIQUE (name)
);

CREATE TABLE IF NOT EXISTS email_oauth_tokens (
  id TEXT PRIMARY KEY,
  email_provider_id TEXT NOT NULL UNIQUE REFERENCES email_providers(id) ON DELETE CASCADE,
  email_address TEXT NOT NULL DEFAULT '',
  access_token TEXT NOT NULL DEFAULT '',
  refresh_token TEXT NOT NULL DEFAULT '',
  expires_at TEXT,
  scope TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS bot_email_bindings (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  email_provider_id TEXT NOT NULL REFERENCES email_providers(id) ON DELETE CASCADE,
  email_address TEXT NOT NULL,
  can_read INTEGER NOT NULL DEFAULT TRUE,
  can_write INTEGER NOT NULL DEFAULT TRUE,
  can_delete INTEGER NOT NULL DEFAULT FALSE,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_email_bindings_unique UNIQUE (bot_id, email_provider_id)
);

CREATE TEMP TABLE email_provider_bound_owners AS
SELECT DISTINCT
  beb.email_provider_id AS old_provider_id,
  b.owner_user_id
FROM bot_email_bindings AS beb
JOIN bots AS b ON b.id = beb.bot_id;

CREATE TEMP TABLE email_provider_owner_source AS
SELECT old_provider_id, owner_user_id
FROM email_provider_bound_owners
UNION
SELECT ep.id, owner.id
FROM email_providers AS ep
CROSS JOIN (
  SELECT id
  FROM users
  WHERE username IS NOT NULL
  ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, created_at ASC
  LIMIT 1
) AS owner
WHERE NOT EXISTS (
  SELECT 1
  FROM email_provider_bound_owners AS bound
  WHERE bound.old_provider_id = ep.id
);

CREATE TEMP TABLE email_provider_owner_map AS
SELECT
  old_provider_id,
  owner_user_id,
  CASE
    WHEN owner_rank = 1 THEN old_provider_id
    ELSE lower(hex(randomblob(4))) || '-' ||
      lower(hex(randomblob(2))) || '-' ||
      '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
      substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
      lower(hex(randomblob(6)))
  END AS new_provider_id,
  CASE WHEN owner_rank = 1 THEN 1 ELSE 0 END AS is_primary
FROM (
  SELECT
    old_provider_id,
    owner_user_id,
    ROW_NUMBER() OVER (PARTITION BY old_provider_id ORDER BY owner_user_id) AS owner_rank
  FROM email_provider_owner_source
) AS ranked;

CREATE TABLE IF NOT EXISTS email_providers_new (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT email_providers_user_name_unique UNIQUE (user_id, name)
);

INSERT OR IGNORE INTO email_providers_new (id, user_id, name, provider, config, created_at, updated_at)
SELECT
  map.new_provider_id,
  map.owner_user_id,
  ep.name,
  ep.provider,
  CASE
    WHEN ep.provider = 'gmail' THEN json_remove(ep.config, '$.client_id', '$.client_secret')
    ELSE ep.config
  END,
  ep.created_at,
  ep.updated_at
FROM email_provider_owner_map AS map
JOIN email_providers AS ep ON ep.id = map.old_provider_id;

DROP TABLE email_providers;
ALTER TABLE email_providers_new RENAME TO email_providers;

CREATE INDEX IF NOT EXISTS idx_email_providers_user_id ON email_providers(user_id);

INSERT OR IGNORE INTO email_oauth_tokens (
  id,
  email_provider_id,
  email_address,
  access_token,
  refresh_token,
  expires_at,
  scope,
  state,
  created_at,
  updated_at
)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  map.new_provider_id,
  tok.email_address,
  tok.access_token,
  tok.refresh_token,
  tok.expires_at,
  tok.scope,
  tok.state,
  tok.created_at,
  tok.updated_at
FROM email_provider_owner_map AS map
JOIN email_oauth_tokens AS tok ON tok.email_provider_id = map.old_provider_id
WHERE map.is_primary = 0;

UPDATE bot_email_bindings
SET email_provider_id = (
  SELECT map.new_provider_id
  FROM bots AS b
  JOIN email_provider_owner_map AS map
    ON map.old_provider_id = bot_email_bindings.email_provider_id
    AND map.owner_user_id = b.owner_user_id
  WHERE b.id = bot_email_bindings.bot_id
  LIMIT 1
)
WHERE EXISTS (
  SELECT 1
  FROM bots AS b
  JOIN email_provider_owner_map AS map
    ON map.old_provider_id = bot_email_bindings.email_provider_id
    AND map.owner_user_id = b.owner_user_id
  WHERE b.id = bot_email_bindings.bot_id
);

INSERT OR IGNORE INTO email_providers (id, user_id, name, provider, config)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  users.id,
  'Gmail',
  'gmail',
  '{}'
FROM users
WHERE username IS NOT NULL;

DROP TABLE email_provider_owner_map;
DROP TABLE email_provider_owner_source;
DROP TABLE email_provider_bound_owners;

PRAGMA foreign_keys = ON;
