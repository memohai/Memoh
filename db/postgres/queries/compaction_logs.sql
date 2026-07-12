-- name: CreateCompactionLog :one
INSERT INTO bot_history_message_compacts (id, bot_id, session_id)
VALUES (sqlc.arg(id), sqlc.arg(bot_id), sqlc.arg(session_id))
ON CONFLICT (id) DO NOTHING
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
          artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
          superseded_by, superseded_at, started_at, completed_at;

-- name: CompleteCompactionLog :one
UPDATE bot_history_message_compacts
SET status = $2,
    summary = $3,
    message_count = $4,
    error_message = $5,
    usage = $6,
    model_id = $7,
    coverage = $8,
    anchor_start_ms = $9,
    anchor_end_ms = $10,
    completed_at = now()
WHERE id = $1
  AND status = 'pending'
RETURNING id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
          artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
          superseded_by, superseded_at, started_at, completed_at;

-- name: FinalizeCompactionArtifact :one
WITH requested_sources AS MATERIALIZED (
  SELECT
    requested_ids.message_id,
    requested_versions.source_version,
    requested_compacts.expected_compact_id,
    COALESCE(requested_ids.ordinal, requested_versions.ordinal, requested_compacts.ordinal)::bigint AS ordinal
  FROM unnest(sqlc.arg(message_ids)::uuid[])
    WITH ORDINALITY AS requested_ids(message_id, ordinal)
  FULL JOIN unnest(sqlc.arg(source_versions)::text[])
    WITH ORDINALITY AS requested_versions(source_version, ordinal)
    ON requested_versions.ordinal = requested_ids.ordinal
  FULL JOIN unnest(sqlc.arg(expected_compact_ids)::text[])
    WITH ORDINALITY AS requested_compacts(expected_compact_id, ordinal)
    ON requested_compacts.ordinal = COALESCE(requested_ids.ordinal, requested_versions.ordinal)
),
coverage_sources AS MATERIALIZED (
  SELECT
    covered.source->'ref'->>'id' AS source_id,
    covered.ordinal::bigint AS ordinal
  FROM jsonb_array_elements(sqlc.arg(coverage)::jsonb)
    WITH ORDINALITY AS covered(source, ordinal)
),
request_shape AS MATERIALIZED (
  SELECT
    COUNT(*)::integer AS requested_count,
    COUNT(DISTINCT message_id)::integer AS unique_count,
    COALESCE(BOOL_AND(
      message_id IS NOT NULL
      AND COALESCE(BTRIM(source_version), '') <> ''
      AND expected_compact_id IS NOT NULL
      AND expected_compact_id <> (sqlc.arg(compact_id)::uuid)::text
    ), false) AS valid
  FROM requested_sources
),
locked_compacts AS MATERIALIZED (
  SELECT compact.id, compact.bot_id, compact.session_id, compact.status
  FROM bot_history_message_compacts compact
  WHERE compact.id = sqlc.arg(compact_id)
     OR EXISTS (
       SELECT 1
       FROM requested_sources requested
       WHERE requested.expected_compact_id = compact.id::text
     )
  ORDER BY compact.id
  FOR UPDATE OF compact
),
locked_log AS MATERIALIZED (
  SELECT compact.id
  FROM locked_compacts compact
  WHERE compact.id = sqlc.arg(compact_id)
    AND compact.bot_id = sqlc.arg(bot_id)
    AND compact.session_id = sqlc.arg(session_id)
    AND compact.status = 'pending'
),
eligible_request AS MATERIALIZED (
  SELECT shape.requested_count
  FROM request_shape shape
  WHERE shape.requested_count > 0
    AND shape.requested_count = shape.unique_count
    AND shape.valid
    AND cardinality(sqlc.arg(message_ids)::uuid[]) = cardinality(sqlc.arg(source_versions)::text[])
    AND cardinality(sqlc.arg(message_ids)::uuid[]) = cardinality(sqlc.arg(expected_compact_ids)::text[])
    AND COALESCE(
      (SELECT ARRAY_AGG(source_id ORDER BY ordinal) FROM coverage_sources),
      '{}'::text[]
    ) = COALESCE(
      (SELECT ARRAY_AGG(message_id::text ORDER BY ordinal) FROM requested_sources),
      '{}'::text[]
    )
    AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements(sqlc.arg(coverage)::jsonb) AS covered(source)
      WHERE jsonb_typeof(covered.source) IS DISTINCT FROM 'object'
        OR jsonb_typeof(covered.source->'ref') IS DISTINCT FROM 'object'
        OR jsonb_typeof(covered.source->'ref'->'namespace') IS DISTINCT FROM 'string'
        OR COALESCE(BTRIM(covered.source->'ref'->>'namespace'), '') <> 'bot_history_message'
        OR jsonb_typeof(covered.source->'ref'->'id') IS DISTINCT FROM 'string'
        OR COALESCE(BTRIM(covered.source->'ref'->>'id'), '') = ''
        OR jsonb_typeof(covered.source->'ref'->'version') IS DISTINCT FROM 'number'
        OR COALESCE(covered.source->'ref'->>'version', '') <> '1'
        OR jsonb_typeof(covered.source->'ref'->'schema') IS DISTINCT FROM 'string'
        OR COALESCE(BTRIM(covered.source->'ref'->>'schema'), '') <> 'context_ref'
        OR jsonb_typeof(covered.source->'ref'->'durability') IS DISTINCT FROM 'string'
        OR COALESCE(BTRIM(covered.source->'ref'->>'durability'), '') <> 'durable'
        OR jsonb_typeof(covered.source->'ref'->'hash_algo') IS DISTINCT FROM 'string'
        OR COALESCE(BTRIM(covered.source->'ref'->>'hash_algo'), '') <> 'sha256'
        OR jsonb_typeof(covered.source->'ref'->'hash_scope') IS DISTINCT FROM 'string'
        OR COALESCE(BTRIM(covered.source->'ref'->>'hash_scope'), '') <> 'source_payload'
        OR jsonb_typeof(covered.source->'ref'->'content_hash') IS DISTINCT FROM 'string'
        OR COALESCE(covered.source->'ref'->>'content_hash', '') !~ '^[0-9a-f]{64}$'
        OR covered.source->'ref'->'range' IS NOT NULL
        OR jsonb_typeof(covered.source->'created_at_ms') IS DISTINCT FROM 'number'
        OR COALESCE(covered.source->>'created_at_ms', '') !~ '^[0-9]+$'
        OR (
          covered.source ? 'external_message_id'
          AND jsonb_typeof(covered.source->'external_message_id') IS DISTINCT FROM 'string'
        )
        OR (
          covered.source ? 'source_reply_to_message_id'
          AND jsonb_typeof(covered.source->'source_reply_to_message_id') IS DISTINCT FROM 'string'
        )
    )
    AND NOT EXISTS (
      SELECT 1
      FROM coverage_sources covered
      WHERE covered.ordinal > 1
        AND (
          sqlc.arg(coverage)::jsonb->((covered.ordinal - 1)::integer)->>'created_at_ms'
        )::bigint < (
          sqlc.arg(coverage)::jsonb->((covered.ordinal - 2)::integer)->>'created_at_ms'
        )::bigint
    )
    AND sqlc.arg(anchor_start_ms)::bigint = (
      sqlc.arg(coverage)::jsonb->0->>'created_at_ms'
    )::bigint
    AND sqlc.arg(anchor_end_ms)::bigint = (
      sqlc.arg(coverage)::jsonb->(jsonb_array_length(sqlc.arg(coverage)::jsonb) - 1)->>'created_at_ms'
    )::bigint
),
locked_sources AS MATERIALIZED (
  SELECT message.id
  FROM bot_history_messages message
  JOIN requested_sources requested
    ON requested.message_id = message.id
   AND requested.source_version = COALESCE(to_jsonb(message)->>'source_revision', message.xmin::text)
   AND COALESCE(message.compact_id::text, '') = requested.expected_compact_id
  JOIN coverage_sources covered
    ON covered.ordinal = requested.ordinal
   AND covered.source_id = message.id::text
   AND (
     sqlc.arg(coverage)::jsonb->((requested.ordinal - 1)::integer)->>'created_at_ms'
   )::bigint = FLOOR(EXTRACT(EPOCH FROM message.created_at) * 1000)::bigint
  CROSS JOIN locked_log
  CROSS JOIN eligible_request
  WHERE message.bot_id = sqlc.arg(bot_id)
    AND message.session_id = sqlc.arg(session_id)
    AND message.turn_visible = true
    AND message.turn_id IS NOT NULL
    AND message.turn_position IS NOT NULL
    AND message.turn_message_seq IS NOT NULL
    AND (message.metadata->>'trigger_mode' IS NULL OR message.metadata->>'trigger_mode' != 'passive_sync')
    AND (
      requested.expected_compact_id = ''
      OR EXISTS (
        SELECT 1
        FROM locked_compacts existing_compact
        WHERE existing_compact.id::text = requested.expected_compact_id
          AND existing_compact.bot_id = sqlc.arg(bot_id)
          AND existing_compact.session_id = sqlc.arg(session_id)
          AND (
            existing_compact.status <> 'ok'
            OR EXISTS (
              SELECT 1
              FROM bot_history_message_compact_claim_validity validity
              WHERE validity.compact_id = existing_compact.id
                AND NOT validity.sources_current
            )
          )
      )
    )
  ORDER BY message.id
  FOR UPDATE OF message
),
claimed_sources AS (
  UPDATE bot_history_messages message
  SET compact_id = sqlc.arg(compact_id),
      compact_claim_finalized = false,
      compact_claim_invalidated = false
  FROM locked_log, eligible_request request
  WHERE message.id IN (SELECT id FROM locked_sources)
    AND (SELECT COUNT(*) FROM locked_sources) = request.requested_count
  RETURNING message.id
),
finalization_stats AS MATERIALIZED (
  SELECT
    shape.requested_count,
    (SELECT COUNT(*) FROM locked_sources)::integer AS matched_count,
    (SELECT COUNT(*) FROM claimed_sources)::integer AS claimed_count
  FROM request_shape shape
),
finalized_state AS MATERIALIZED (
  SELECT (
    EXISTS (SELECT 1 FROM eligible_request)
    AND stats.matched_count = stats.requested_count
    AND stats.claimed_count = stats.requested_count
  ) AS finalized
  FROM finalization_stats stats
),
retired_pending_logs AS (
  UPDATE bot_history_message_compacts prior
  SET status = 'error',
      summary = '',
      message_count = 0,
      error_message = 'compaction source reclaimed by newer attempt',
      usage = NULL,
      model_id = NULL,
      coverage = '[]'::jsonb,
      anchor_start_ms = 0,
      anchor_end_ms = 0,
      completed_at = now()
  FROM locked_compacts locked, finalized_state state
  WHERE state.finalized
    AND prior.id = locked.id
    AND prior.id <> sqlc.arg(compact_id)
    AND locked.status = 'pending'
  RETURNING prior.id
),
retirement_guard AS MATERIALIZED (
  SELECT COUNT(*)::integer AS retired_count FROM retired_pending_logs
),
completed_log AS (
  UPDATE bot_history_message_compacts compact
  SET status = CASE WHEN state.finalized THEN 'ok' ELSE 'error' END,
      summary = CASE WHEN state.finalized THEN sqlc.arg(summary) ELSE '' END,
      message_count = CASE WHEN state.finalized THEN shape.requested_count ELSE 0 END,
      error_message = CASE
        WHEN state.finalized THEN ''
        ELSE 'compaction source changed before finalization'
      END,
      usage = CASE WHEN state.finalized THEN sqlc.narg(usage)::jsonb ELSE NULL::jsonb END,
      model_id = CASE WHEN state.finalized THEN sqlc.narg(model_id)::uuid ELSE NULL::uuid END,
      artifact_version = 1,
      coverage = CASE WHEN state.finalized THEN sqlc.arg(coverage)::jsonb ELSE '[]'::jsonb END,
      anchor_start_ms = CASE WHEN state.finalized THEN sqlc.arg(anchor_start_ms)::bigint ELSE 0 END,
      anchor_end_ms = CASE WHEN state.finalized THEN sqlc.arg(anchor_end_ms)::bigint ELSE 0 END,
      artifact_level = 0,
      parent_ids = '{}'::uuid[],
      completed_at = now()
  FROM locked_log, request_shape shape, finalized_state state, retirement_guard
  WHERE compact.id = locked_log.id
  RETURNING compact.status, state.finalized
)
SELECT
  COALESCE(completed.finalized, false)::boolean AS finalized,
  COALESCE(completed.status, '')::text AS status,
  stats.requested_count,
  stats.matched_count,
  stats.claimed_count
FROM finalization_stats stats
LEFT JOIN completed_log completed ON true;

-- name: GetCompactionLogByID :one
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
       artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
       superseded_by, superseded_at, started_at, completed_at
FROM bot_history_message_compacts
WHERE id = $1;

-- name: ListCompactionArtifactParentIDsBySuccessor :many
SELECT id
FROM bot_history_message_compacts
WHERE superseded_by = sqlc.arg(successor_id)
  AND bot_id = sqlc.arg(bot_id)
  AND session_id IS NOT DISTINCT FROM sqlc.narg(session_id)::uuid
  AND status = 'ok'
ORDER BY id ASC;

-- name: ListCompactionArtifactParentEdges :many
SELECT parent_id, ordinal
FROM bot_history_message_compact_parent_edges
WHERE artifact_id = $1
ORDER BY ordinal ASC;

-- name: ListCompactionLogsByBot :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
       artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
       superseded_by, superseded_at, started_at, completed_at
FROM bot_history_message_compacts
WHERE bot_id = $1
ORDER BY started_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCompactionLogsByBot :one
SELECT count(*) FROM bot_history_message_compacts WHERE bot_id = $1;

-- name: ListCompactionArtifactLineageBySession :many
SELECT id, bot_id, session_id, status, summary, message_count, error_message, usage, model_id,
       artifact_version, coverage, anchor_start_ms, anchor_end_ms, artifact_level, parent_ids,
       superseded_by, superseded_at, started_at, completed_at
FROM bot_history_message_compacts c
WHERE c.session_id = $1
  AND (
    c.status = 'ok'
    OR EXISTS (
      SELECT 1
      FROM bot_history_message_compacts parent
      WHERE parent.session_id = $1
        AND parent.status = 'ok'
        AND parent.superseded_by = c.id
    )
  )
ORDER BY c.anchor_start_ms ASC, c.started_at ASC, c.id ASC;

-- name: ListInvalidCompactionArtifactIDsBySession :many
SELECT compact.id
FROM bot_history_message_compacts compact
JOIN bot_history_message_compact_claim_validity validity
  ON validity.compact_id = compact.id
WHERE compact.session_id = $1
  AND compact.status = 'ok'
  AND compact.artifact_level = 0
  AND NOT validity.sources_current
ORDER BY compact.id ASC;

-- name: DeleteCompactionLogsByBot :exec
DELETE FROM bot_history_message_compacts WHERE bot_id = $1;
