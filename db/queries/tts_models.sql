-- name: CreateTtsModel :one
INSERT INTO tts_models (model_id, name, tts_provider_id, config)
VALUES (
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(tts_provider_id),
  sqlc.arg(config)
)
RETURNING *;

-- name: GetTtsModelByID :one
SELECT * FROM tts_models WHERE id = sqlc.arg(id);

-- name: GetTtsModelWithProvider :one
SELECT
  tm.*,
  tp.provider AS provider_type
FROM tts_models tm
JOIN tts_providers tp ON tp.id = tm.tts_provider_id
WHERE tm.id = sqlc.arg(id);

-- name: ListTtsModels :many
SELECT * FROM tts_models
ORDER BY created_at DESC;

-- name: ListTtsModelsByProviderID :many
SELECT * FROM tts_models
WHERE tts_provider_id = sqlc.arg(tts_provider_id)
ORDER BY created_at DESC;

-- name: UpdateTtsModel :one
UPDATE tts_models
SET
  name = sqlc.arg(name),
  config = sqlc.arg(config),
  updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: DeleteTtsModel :exec
DELETE FROM tts_models WHERE id = sqlc.arg(id);

-- name: DeleteTtsModelsByProviderID :exec
DELETE FROM tts_models WHERE tts_provider_id = sqlc.arg(tts_provider_id);

-- name: UpsertTtsModel :one
INSERT INTO tts_models (model_id, name, tts_provider_id, config)
VALUES (
  sqlc.arg(model_id),
  sqlc.arg(name),
  sqlc.arg(tts_provider_id),
  sqlc.arg(config)
)
ON CONFLICT (tts_provider_id, model_id)
DO UPDATE SET
  name = EXCLUDED.name,
  updated_at = now()
RETURNING *;
