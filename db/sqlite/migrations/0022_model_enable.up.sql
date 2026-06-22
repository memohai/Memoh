-- 0022_model_enable
-- Add a per-model enable flag so users can disable specific models within a
-- provider. Defaults to 1 (true) so existing rows stay listed in chat pickers.

PRAGMA foreign_keys = OFF;

BEGIN;

-- SQLite has no ADD COLUMN IF NOT EXISTS. Patch legacy schemas by appending a
-- trailing enable column first, preserving existing row storage order. Then
-- rebuild into the canonical column order used by the baseline schema.
PRAGMA writable_schema = ON;

UPDATE sqlite_schema
SET sql = replace(
  sql,
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT models_provider_id_model_id_unique',
  'updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  enable INTEGER NOT NULL DEFAULT 1,
  CONSTRAINT models_provider_id_model_id_unique'
)
WHERE type = 'table'
  AND name = 'models'
  AND sql NOT LIKE '%enable INTEGER%';

PRAGMA writable_schema = OFF;
PRAGMA schema_version = 1000022;

CREATE TABLE models_new (
  id TEXT PRIMARY KEY,
  model_id TEXT NOT NULL,
  name TEXT,
  provider_id TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'chat',
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT models_provider_id_model_id_unique UNIQUE (provider_id, model_id),
  CONSTRAINT models_type_check CHECK (type IN ('chat', 'embedding', 'speech', 'transcription'))
);

INSERT INTO models_new (
  id,
  model_id,
  name,
  provider_id,
  type,
  enable,
  config,
  created_at,
  updated_at
)
SELECT
  id,
  model_id,
  name,
  provider_id,
  type,
  enable,
  config,
  created_at,
  updated_at
FROM models;

DROP TABLE models;
ALTER TABLE models_new RENAME TO models;

COMMIT;

PRAGMA foreign_keys = ON;
