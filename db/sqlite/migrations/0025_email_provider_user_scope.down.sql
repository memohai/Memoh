-- 0025_email_provider_user_scope
-- Restore global email provider names.

PRAGMA foreign_keys = OFF;

CREATE TEMP TABLE email_provider_global_map AS
SELECT
  ep.id AS old_provider_id,
  (
    SELECT ep2.id
    FROM email_providers AS ep2
    WHERE ep2.name = ep.name
    ORDER BY
      (
        SELECT COUNT(*)
        FROM email_oauth_tokens AS tok
        WHERE tok.email_provider_id = ep2.id
      ) DESC,
      ep2.created_at ASC,
      ep2.id ASC
    LIMIT 1
  ) AS survivor_provider_id
FROM email_providers AS ep;

UPDATE bot_email_bindings
SET email_provider_id = (
  SELECT survivor_provider_id
  FROM email_provider_global_map
  WHERE old_provider_id = bot_email_bindings.email_provider_id
)
WHERE EXISTS (
  SELECT 1
  FROM email_provider_global_map
  WHERE old_provider_id = bot_email_bindings.email_provider_id
    AND old_provider_id <> survivor_provider_id
);

DELETE FROM email_oauth_tokens
WHERE email_provider_id IN (
  SELECT old_provider_id
  FROM email_provider_global_map
  WHERE old_provider_id <> survivor_provider_id
);

CREATE TABLE IF NOT EXISTS email_providers_old (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  provider TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT email_providers_name_unique UNIQUE (name)
);

INSERT OR IGNORE INTO email_providers_old (id, name, provider, config, created_at, updated_at)
SELECT id, name, provider, config, created_at, updated_at
FROM email_providers
WHERE id IN (
  SELECT survivor_provider_id
  FROM email_provider_global_map
)
ORDER BY created_at ASC, id ASC;

DROP TABLE email_providers;
ALTER TABLE email_providers_old RENAME TO email_providers;

DROP TABLE email_provider_global_map;

PRAGMA foreign_keys = ON;
