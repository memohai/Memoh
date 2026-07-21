-- name: CreateSubagentConfig :one
INSERT INTO subagent_configs (
  session_id,
  model_uuid,
  model_id,
  provider_name,
  forked,
  parent_messages
) VALUES (
  $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetSubagentConfig :one
SELECT *
FROM subagent_configs
WHERE session_id = $1;

-- name: UpsertSubagentConfig :one
INSERT INTO subagent_configs (
  session_id,
  model_uuid,
  model_id,
  provider_name,
  forked,
  parent_messages
) VALUES (
  $1, $2, $3, $4, $5, $6
)
ON CONFLICT (session_id) DO UPDATE SET
  model_uuid = EXCLUDED.model_uuid,
  model_id = EXCLUDED.model_id,
  provider_name = EXCLUDED.provider_name,
  forked = EXCLUDED.forked,
  parent_messages = EXCLUDED.parent_messages,
  updated_at = now()
RETURNING *;
