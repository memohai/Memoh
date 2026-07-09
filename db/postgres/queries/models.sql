-- name: CreateProvider :one
INSERT INTO providers (team_id, name, client_type, icon, enable, config, metadata)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(name),
  sqlc.arg(client_type),
  sqlc.arg(icon),
  sqlc.arg(enable),
  sqlc.arg(config),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetProviderByID :one
SELECT * FROM providers WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: GetProviderByIDForTeam :one
SELECT * FROM providers
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetProviderByName :one
SELECT * FROM providers WHERE name = sqlc.arg(name);

-- name: GetProviderByNameForTeam :one
SELECT * FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND name = sqlc.arg(name);

-- name: GetProviderByClientType :one
SELECT * FROM providers WHERE client_type = sqlc.arg(client_type);

-- name: GetProviderByClientTypeForTeam :one
SELECT * FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND client_type = sqlc.arg(client_type);

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
  'google-transcription',
  'openrouter-video',
  'modelark-video',
  'volcengine-video'
)
ORDER BY created_at DESC;

-- name: ListProvidersForTeam :many
SELECT * FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND client_type NOT IN (
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
  AND team_id = COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: DeleteProvider :exec
DELETE FROM providers WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: DeleteProviderForTeam :exec
DELETE FROM providers
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

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
  'google-transcription',
  'openrouter-video',
  'modelark-video',
  'volcengine-video'
);

-- name: CountProvidersForTeam :one
SELECT COUNT(*)
FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND client_type NOT IN (
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
  );

-- name: CreateModel :one
INSERT INTO models (team_id, model_id, name, provider_id, type, enable, config)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(provider_id),
  sqlc.arg(type),
  sqlc.arg(enable),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetModelByID :one
SELECT * FROM models WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: GetModelByIDForTeam :one
SELECT * FROM models
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: GetModelByModelID :one
SELECT * FROM models WHERE model_id = sqlc.arg(model_id);

-- name: GetModelByModelIDForTeam :one
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND model_id = sqlc.arg(model_id);

-- name: ListModelsByModelID :many
SELECT * FROM models
WHERE model_id = sqlc.arg(model_id)
ORDER BY created_at DESC;

-- name: ListModelsByModelIDForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND model_id = sqlc.arg(model_id)
ORDER BY created_at DESC;

-- name: ListModels :many
SELECT * FROM models
WHERE type NOT IN ('speech', 'transcription', 'video')
ORDER BY created_at DESC;

-- name: ListModelsForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND type NOT IN ('speech', 'transcription', 'video')
ORDER BY created_at DESC;

-- name: ListModelsByType :many
SELECT * FROM models
WHERE type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByTypeForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type NOT IN ('speech', 'transcription', 'video')
ORDER BY created_at DESC;

-- name: ListModelsByProviderIDForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND type NOT IN ('speech', 'transcription', 'video')
ORDER BY created_at DESC;

-- name: ListModelsByProviderIDAndType :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderIDAndTypeForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderClientType :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: ListModelsByProviderClientTypeForTeam :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: UpdateModel :one
UPDATE models
SET
  model_id = sqlc.arg(model_id),
  name = sqlc.arg(name),
  provider_id = sqlc.arg(provider_id),
  type = sqlc.arg(type),
  enable = sqlc.arg(enable),
  config = sqlc.arg(config),
  updated_at = now()
WHERE id = sqlc.arg(id)
  AND team_id = COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid)
RETURNING *;

-- name: DeleteModel :exec
DELETE FROM models WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id)::uuid;

-- name: DeleteModelForTeam :exec
DELETE FROM models
WHERE id = sqlc.arg(id)
  AND team_id = sqlc.arg(team_id);

-- name: DeleteModelByModelID :exec
DELETE FROM models WHERE model_id = sqlc.arg(model_id);

-- name: DeleteModelByModelIDForTeam :exec
DELETE FROM models
WHERE team_id = sqlc.arg(team_id)
  AND model_id = sqlc.arg(model_id);

-- name: DeleteModelByProviderIDAndModelID :exec
DELETE FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id);

-- name: DeleteModelByProviderIDAndModelIDForTeam :exec
DELETE FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id);

-- name: DeleteModelByProviderAndType :exec
DELETE FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id)
  AND type = sqlc.arg(type);

-- name: DeleteModelByProviderAndTypeForTeam :exec
DELETE FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id)
  AND type = sqlc.arg(type);

-- name: CountModels :one
SELECT COUNT(*) FROM models
WHERE type NOT IN ('speech', 'transcription', 'video');

-- name: CountModelsForTeam :one
SELECT COUNT(*) FROM models
WHERE team_id = sqlc.arg(team_id)
  AND type NOT IN ('speech', 'transcription', 'video');

-- name: CountModelsByType :one
SELECT COUNT(*) FROM models WHERE type = sqlc.arg(type);

-- name: CountModelsByTypeForTeam :one
SELECT COUNT(*) FROM models
WHERE team_id = sqlc.arg(team_id)
  AND type = sqlc.arg(type);


-- name: UpsertRegistryProvider :one
INSERT INTO providers (team_id, name, client_type, icon, enable, config, metadata)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(name),
  sqlc.arg(client_type),
  sqlc.arg(icon),
  false,
  sqlc.arg(config),
  '{}'
)
ON CONFLICT (team_id, name) DO UPDATE SET
  icon = EXCLUDED.icon,
  client_type = EXCLUDED.client_type,
  updated_at = now()
RETURNING *;

-- name: UpsertRegistryModel :one
INSERT INTO models (team_id, model_id, name, provider_id, type, config)
VALUES (
  COALESCE(sqlc.narg(team_id)::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(provider_id),
  sqlc.arg(type),
  sqlc.arg(config)
)
ON CONFLICT (team_id, provider_id, model_id) DO UPDATE SET
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
  AND m.enable = true
  AND m.type NOT IN ('speech', 'transcription', 'video')
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsForTeam :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND p.enable = true
  AND m.enable = true
  AND m.type NOT IN ('speech', 'transcription', 'video')
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByType :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.enable = true
  AND m.enable = true
  AND m.type = sqlc.arg(type)
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByTypeForTeam :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND p.enable = true
  AND m.enable = true
  AND m.type = sqlc.arg(type)
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByProviderClientType :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE p.enable = true
  AND m.enable = true
  AND p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByProviderClientTypeForTeam :many
SELECT m.*
FROM models m
JOIN providers p ON m.provider_id = p.id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND p.enable = true
  AND m.enable = true
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
  AND m.team_id = sqlc.arg(team_id)::uuid
  AND m.type = 'speech';

-- name: GetSpeechModelWithProviderForTeam :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
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

-- name: ListSpeechProvidersForTeam :many
SELECT * FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND client_type IN (
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

-- name: ListTranscriptionProvidersForTeam :many
SELECT * FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND client_type IN (
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
  AND m.enable = true
ORDER BY m.created_at DESC;

-- name: ListSpeechModelsForTeam :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND m.type = 'speech'
  AND m.enable = true
ORDER BY m.created_at DESC;

-- name: ListSpeechModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = 'speech'
  AND enable = true
ORDER BY created_at DESC;

-- name: ListSpeechModelsByProviderIDForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND type = 'speech'
  AND enable = true
ORDER BY created_at DESC;

-- name: GetModelByProviderAndModelID :one
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id)
LIMIT 1;

-- name: GetModelByProviderAndModelIDForTeam :one
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND model_id = sqlc.arg(model_id)
LIMIT 1;

-- name: GetTranscriptionModelWithProvider :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.team_id = sqlc.arg(team_id)::uuid
  AND m.type = 'transcription';

-- name: GetTranscriptionModelWithProviderForTeam :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND m.type = 'transcription';

-- name: ListTranscriptionModels :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.type = 'transcription'
  AND m.enable = true
ORDER BY m.created_at DESC;

-- name: ListTranscriptionModelsForTeam :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND m.type = 'transcription'
  AND m.enable = true
ORDER BY m.created_at DESC;

-- name: ListTranscriptionModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = 'transcription'
  AND enable = true
ORDER BY created_at DESC;

-- name: ListTranscriptionModelsByProviderIDForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND type = 'transcription'
  AND enable = true
ORDER BY created_at DESC;


-- name: GetVideoModelWithProvider :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.team_id = sqlc.arg(team_id)::uuid
  AND m.type = 'video';

-- name: GetVideoModelWithProviderForTeam :one
SELECT
  m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.id = sqlc.arg(id)
  AND m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND m.type = 'video';

-- name: ListVideoProviders :many
SELECT * FROM providers
WHERE client_type IN (
  'openrouter-video',
  'modelark-video',
  'volcengine-video'
)
ORDER BY created_at DESC;

-- name: ListVideoProvidersForTeam :many
SELECT * FROM providers
WHERE team_id = sqlc.arg(team_id)
  AND client_type IN (
    'openrouter-video',
    'modelark-video',
    'volcengine-video'
  )
ORDER BY created_at DESC;

-- name: ListVideoModels :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.type = 'video'
  AND m.enable = true
ORDER BY m.created_at DESC;

-- name: ListVideoModelsForTeam :many
SELECT m.*,
  p.client_type AS provider_type
FROM models m
JOIN providers p ON p.id = m.provider_id
WHERE m.team_id = sqlc.arg(team_id)
  AND p.team_id = sqlc.arg(team_id)
  AND m.type = 'video'
  AND m.enable = true
ORDER BY m.created_at DESC;

-- name: ListVideoModelsByProviderID :many
SELECT * FROM models
WHERE provider_id = sqlc.arg(provider_id)
  AND type = 'video'
  AND enable = true
ORDER BY created_at DESC;

-- name: ListVideoModelsByProviderIDForTeam :many
SELECT * FROM models
WHERE team_id = sqlc.arg(team_id)
  AND provider_id = sqlc.arg(provider_id)
  AND type = 'video'
  AND enable = true
ORDER BY created_at DESC;
