-- name: CreateLlmProvider :one
INSERT INTO llm_providers (name, client_type, base_url, api_key, metadata)
VALUES (
  sqlc.arg(name),
  sqlc.arg(client_type),
  sqlc.arg(base_url),
  sqlc.arg(api_key),
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

-- name: ListLlmProvidersByClientType :many
SELECT * FROM llm_providers
WHERE client_type = sqlc.arg(client_type)
ORDER BY created_at DESC;

-- name: UpdateLlmProvider :one
UPDATE llm_providers
SET
  name = sqlc.arg(name),
  client_type = sqlc.arg(client_type),
  base_url = sqlc.arg(base_url),
  api_key = sqlc.arg(api_key),
  metadata = sqlc.arg(metadata),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteLlmProvider :exec
DELETE FROM llm_providers WHERE id = sqlc.arg(id);

-- name: CountLlmProviders :one
SELECT COUNT(*) FROM llm_providers;

-- name: CountLlmProvidersByClientType :one
SELECT COUNT(*) FROM llm_providers WHERE client_type = sqlc.arg(client_type);

-- name: CreateModel :one
INSERT INTO models (model_id, name, llm_provider_id, dimensions, input_modalities, type)
VALUES (
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(llm_provider_id),
  sqlc.arg(dimensions),
  sqlc.arg(input_modalities),
  sqlc.arg(type)
)
RETURNING *;

-- name: GetModelByID :one
SELECT * FROM models WHERE id = sqlc.arg(id);

-- name: GetModelByModelID :one
SELECT * FROM models WHERE model_id = sqlc.arg(model_id);

-- name: ListModels :many
SELECT * FROM models
ORDER BY created_at DESC;

-- name: ListModelsByType :many
SELECT * FROM models
WHERE type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: ListModelsByClientType :many
SELECT m.* FROM models AS m
JOIN llm_providers AS p ON p.id = m.llm_provider_id
WHERE p.client_type = sqlc.arg(client_type)
ORDER BY m.created_at DESC;

-- name: ListModelsByProviderID :many
SELECT * FROM models
WHERE llm_provider_id = sqlc.arg(llm_provider_id)
ORDER BY created_at DESC;

-- name: ListModelsByProviderIDAndType :many
SELECT * FROM models
WHERE llm_provider_id = sqlc.arg(llm_provider_id)
  AND type = sqlc.arg(type)
ORDER BY created_at DESC;

-- name: UpdateModel :one
UPDATE models
SET
  name = sqlc.arg(name),
  llm_provider_id = sqlc.arg(llm_provider_id),
  dimensions = sqlc.arg(dimensions),
  input_modalities = sqlc.arg(input_modalities),
  type = sqlc.arg(type),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: UpdateModelByModelID :one
UPDATE models
SET
  model_id = sqlc.arg(new_model_id),
  name = sqlc.arg(name),
  llm_provider_id = sqlc.arg(llm_provider_id),
  dimensions = sqlc.arg(dimensions),
  input_modalities = sqlc.arg(input_modalities),
  type = sqlc.arg(type),
  updated_at = now()
WHERE model_id = sqlc.arg(model_id)
RETURNING *;

-- name: DeleteModel :exec
DELETE FROM models WHERE id = sqlc.arg(id);

-- name: DeleteModelByModelID :exec
DELETE FROM models WHERE model_id = sqlc.arg(model_id);

-- name: CountModels :one
SELECT COUNT(*) FROM models;

-- name: CountModelsByType :one
SELECT COUNT(*) FROM models WHERE type = sqlc.arg(type);


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


