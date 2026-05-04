-- name: CreateProvider :one
INSERT INTO providers (name, client_type, icon, enable, config, metadata)
VALUES (
  sqlc.arg(name),
  sqlc.arg(client_type),
  sqlc.arg(icon),
  sqlc.arg(enable),
  sqlc.arg(config),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetProviderByID :one
SELECT * FROM providers WHERE id = sqlc.arg(id);

-- name: GetProviderByName :one
SELECT * FROM providers WHERE name = sqlc.arg(name);

-- name: GetProviderByClientType :one
SELECT * FROM providers WHERE client_type = sqlc.arg(client_type);

-- name: ListProviders :many
SELECT * FROM providers
WHERE client_type NOT IN (
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
  'google-transcription'
)
ORDER BY created_at DESC;

-- name: UpdateProvider :one
UPDATE providers
SET
  name = sqlc.arg(name),
  client_type = sqlc.arg(client_type),
  icon = sqlc.arg(icon),
  enable = sqlc.arg(enable),
  config = sqlc.arg(config),
  metadata = sqlc.arg(metadata),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteProvider :exec
DELETE FROM providers WHERE id = sqlc.arg(id);

-- name: CountProviders :one
SELECT COUNT(*)
FROM providers
WHERE client_type NOT IN (
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
  'google-transcription'
);

-- name: CreateModel :one
INSERT INTO models (model_id, name, provider_id, type, config)
VALUES (
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(provider_id),
  sqlc.arg(type),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetModelByID :one
SELECT * FROM models WHERE id = sqlc.arg(id);

-- name: GetModelByModelID :one
SELECT * FROM models WHERE model_id = sqlc.arg(model_id);

-- name: ListModelsByModelID :many
SELECT * FROM models
WHERE model_id = sqlc.arg(model_id)
ORDER BY created_at DESC;

-- name: ListModels :many
SELECT * FROM models
WHERE type NOT IN ('speech', 'transcription')
ORDER BY created_at DESC;

-- name: ListModelsByType :many
SELECT * FROM models
WHERE type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type NOT IN ('speech', 'transcription')
ORDER BY created_at DESC;

-- name: ListModelsByProviderIDAndType :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderClientType :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: UpdateModel :one
UPDATE models
SET
  model_id = sqlc.arg(model_id),
  name = sqlc.arg(name),
  provider_id = sqlc.arg(provider_id),
  type = sqlc.arg(type),
  config = sqlc.arg(config),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteModel :exec
DELETE FROM models WHERE id = sqlc.arg(id);

-- name: DeleteModelByModelID :exec
DELETE FROM models WHERE model_id = sqlc.arg(model_id);

-- name: DeleteModelByProviderIDAndModelID :exec
DELETE FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id);

-- name: DeleteModelByProviderAndType :exec
DELETE FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id)
  AND type = sqlc.arg(type);

-- name: CountModels :one
SELECT COUNT(*) FROM models
WHERE type NOT IN ('speech', 'transcription');

-- name: CountModelsByType :one
SELECT COUNT(*) FROM models WHERE type = sqlc.arg(type);


-- name: UpsertRegistryProvider :one
INSERT INTO providers (name, client_type, icon, enable, config, metadata)
VALUES (sqlc.arg(name), sqlc.arg(client_type), sqlc.arg(icon), false, sqlc.arg(config), '{}')
ON CONFLICT (name) DO UPDATE SET
  icon = EXCLUDED.icon,
  client_type = EXCLUDED.client_type,
  updated_at = now()
RETURNING *;

-- name: UpsertRegistryModel :one
INSERT INTO models (model_id, name, provider_id, type, config)
VALUES (sqlc.arg(model_id), sqlc.arg(name), sqlc.arg(provider_id), sqlc.arg(type), sqlc.arg(config))
ON CONFLICT (provider_id, model_id) DO UPDATE SET
  name = EXCLUDED.name,
  type = EXCLUDED.type,
  config = EXCLUDED.config,
  updated_at = now()
RETURNING *;

-- name: ListEnabledModels :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.enable = true
  AND m.type NOT IN ('speech', 'transcription')
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByType :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.enable = true
  AND m.type = sqlc.arg(type)
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByProviderClientType :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.enable = true
  AND p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: CreateModelVariant :one
INSERT INTO model_variants (model_uuid, variant_id, weight, metadata)
VALUES (
  sqlc.arg(model_uuid),
  sqlc.arg(variant_id),
  sqlc.arg(weight),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: ListModelVariantsByModelUUID :many
SELECT * FROM model_variants
WHERE model_uuid = sqlc.arg(model_uuid)
ORDER BY weight DESC, created_at DESC;

-- name: GetSpeechModelWithProvider :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.type = 'speech';

-- name: ListSpeechProviders :many
SELECT * FROM providers
WHERE client_type IN (
  'edge-speech',
  'openai-speech',
  'openrouter-speech',
  'elevenlabs-speech',
  'deepgram-speech',
  'minimax-speech',
  'volcengine-speech',
  'alibabacloud-speech',
  'microsoft-speech'
)
ORDER BY created_at DESC;

-- name: ListTranscriptionProviders :many
SELECT * FROM providers
WHERE client_type IN (
  'openai-transcription',
  'openrouter-transcription',
  'elevenlabs-transcription',
  'deepgram-transcription',
  'google-transcription'
)
ORDER BY created_at DESC;

-- name: ListSpeechModels :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.type = 'speech'
ORDER BY m.created_at DESC;

-- name: ListSpeechModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = 'speech'
ORDER BY created_at DESC;

-- name: GetModelByProviderAndModelID :one
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id)
LIMIT 1;

-- name: GetTranscriptionModelWithProvider :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.type = 'transcription';

-- name: ListTranscriptionModels :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.type = 'transcription'
ORDER BY m.created_at DESC;

-- name: ListTranscriptionModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = 'transcription'
ORDER BY created_at DESC;
