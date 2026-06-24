-- 0024_video_providers
-- Add bot-level video model configuration.

PRAGMA foreign_keys = OFF;

CREATE TABLE providers_new (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  client_type TEXT NOT NULL DEFAULT 'openai-completions',
  icon TEXT,
  enable INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT providers_name_unique UNIQUE (name),
  CONSTRAINT providers_client_type_check CHECK (client_type IN (
    'openai-responses',
    'openai-completions',
    'anthropic-messages',
    'google-generative-ai',
    'openai-codex',
    'github-copilot',
    'edge-speech',
    'openai-speech',
    'openai-transcription',
    'openrouter-speech',
    'openrouter-transcription',
    'elevenlabs-speech',
    'elevenlabs-transcription',
    'deepgram-speech',
    'deepgram-transcription',
    'minimax-speech',
    'volcengine-speech',
    'alibabacloud-speech',
    'microsoft-speech',
    'google-speech',
    'google-transcription',
    'openrouter-video',
    'modelark-video',
    'volcengine-video'
  ))
);

INSERT INTO providers_new (
  id, name, client_type, icon, enable, config, metadata, created_at, updated_at
)
SELECT id, name, client_type, icon, enable, config, metadata, created_at, updated_at
FROM providers;

DROP TABLE providers;
ALTER TABLE providers_new RENAME TO providers;

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
  CONSTRAINT models_type_check CHECK (type IN ('chat', 'embedding', 'speech', 'transcription', 'video'))
);

INSERT INTO models_new (
  id, model_id, name, provider_id, type, enable, config, created_at, updated_at
)
SELECT id, model_id, name, provider_id, type, enable, config, created_at, updated_at
FROM models;

DROP TABLE models;
ALTER TABLE models_new RENAME TO models;

ALTER TABLE bots ADD COLUMN video_model_id TEXT REFERENCES models(id) ON DELETE SET NULL;

PRAGMA foreign_keys = ON;
