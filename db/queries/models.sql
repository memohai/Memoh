-- name: CreateLlmProvider :one
INSERT INTO llm_providers (name, base_url, api_key, client_type, icon, enable, metadata)
VALUES (
  sqlc.arg(name),
  sqlc.arg(base_url),
  sqlc.arg(api_key),
  sqlc.arg(client_type),
  sqlc.arg(icon),
  sqlc.arg(enable),
  sqlc.arg(metadata)
)
RETURNING *;

-- name: GetLlmProviderByID :one
SELECT * FROM llm_providers WHERE id = sqlc.arg(id);

-- name: GetLlmProviderByName :one
SELECT * FROM llm_providers WHERE name = sqlc.arg(name);

-- name: ListLlmProviders :many
SELECT * FROM llm_providers
ORDER BY created_at DESC;

-- name: UpdateLlmProvider :one
UPDATE llm_providers
SET
  name = sqlc.arg(name),
  base_url = sqlc.arg(base_url),
  api_key = sqlc.arg(api_key),
  client_type = sqlc.arg(client_type),
  icon = sqlc.arg(icon),
  enable = sqlc.arg(enable),
  metadata = sqlc.arg(metadata),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteLlmProvider :exec
DELETE FROM llm_providers WHERE id = sqlc.arg(id);

-- name: CountLlmProviders :one
SELECT COUNT(*) FROM llm_providers;

-- name: CreateModel :one
INSERT INTO models (model_id, name, llm_provider_id, type, config)
VALUES (
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(llm_provider_id),
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
ORDER BY created_at DESC;

-- name: ListModelsByType :many
SELECT * FROM models
WHERE type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderID :many
SELECT * FROM models
WHERE llm_provider_id = sqlc.arg(llm_provider_id)
ORDER BY created_at DESC;

-- name: ListModelsByProviderIDAndType :many
SELECT * FROM models
WHERE llm_provider_id = sqlc.arg(llm_provider_id)
  AND type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByProviderClientType :many
SELECT m.*
FROM models m
JOIN llm_providers p ON m.llm_provider_id = p.id
WHERE p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: UpdateModel :one
UPDATE models
SET
  model_id = sqlc.arg(model_id),
  name = sqlc.arg(name),
  llm_provider_id = sqlc.arg(llm_provider_id),
  type = sqlc.arg(type),
  config = sqlc.arg(config),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteModel :exec
DELETE FROM models WHERE id = sqlc.arg(id);

-- name: DeleteModelByModelID :exec
DELETE FROM models WHERE model_id = sqlc.arg(model_id);

-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByType :one
SELECT COUNT(*) FROM models WHERE type = sqlc.arg(type);


-- name: UpsertRegistryProvider :one
INSERT INTO llm_providers (name, base_url, api_key, client_type, icon, enable, metadata)
VALUES (sqlc.arg(name), sqlc.arg(base_url), '', sqlc.arg(client_type), sqlc.arg(icon), false, '{}')
ON CONFLICT (name) DO UPDATE SET
  icon = EXCLUDED.icon,
  client_type = EXCLUDED.client_type,
  updated_at = now()
RETURNING *;

-- name: UpsertRegistryModel :one
INSERT INTO models (model_id, name, llm_provider_id, type, config)
VALUES (sqlc.arg(model_id), sqlc.arg(name), sqlc.arg(llm_provider_id), sqlc.arg(type), sqlc.arg(config))
ON CONFLICT (llm_provider_id, model_id) DO UPDATE SET
  name = EXCLUDED.name,
  type = EXCLUDED.type,
  config = EXCLUDED.config,
  updated_at = now()
RETURNING *;

-- name: ListEnabledModels :many
SELECT m.*
FROM models m
JOIN llm_providers p ON m.llm_provider_id = p.id
WHERE p.enable = true
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByType :many
SELECT m.*
FROM models m
JOIN llm_providers p ON m.llm_provider_id = p.id
WHERE p.enable = true
  AND m.type = sqlc.arg(type)
ORDER BY m.created_at DESC;

-- name: ListEnabledModelsByProviderClientType :many
SELECT m.*
FROM models m
JOIN llm_providers p ON m.llm_provider_id = p.id
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
