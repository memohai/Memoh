-- name: FinalizeCompactionRollup :one
WITH RECURSIVE requested_parents AS MATERIALIZED (
  SELECT requested.parent_id, requested.ordinality::bigint AS ordinal
  FROM unnest(sqlc.arg(parent_ids)::uuid[])
    WITH ORDINALITY AS requested(parent_id, ordinality)
),
request_shape AS MATERIALIZED (
  SELECT
    COUNT(*)::integer AS requested_count,
    COUNT(DISTINCT parent_id)::integer AS unique_count,
    COALESCE(BOOL_AND(parent_id <> sqlc.arg(rollup_id)), false) AS excludes_target
  FROM requested_parents
),
locked_compacts AS MATERIALIZED (
  SELECT
    compact.id,
    compact.bot_id,
    compact.session_id,
    compact.status,
    compact.summary,
    compact.message_count,
    compact.error_message,
    compact.usage,
    compact.model_id,
    compact.artifact_version,
    compact.coverage,
    compact.anchor_start_ms,
    compact.anchor_end_ms,
    compact.artifact_level,
    compact.parent_ids,
    compact.superseded_by,
    compact.superseded_at,
    compact.completed_at
  FROM bot_history_message_compacts compact
  WHERE compact.id = sqlc.arg(rollup_id)
     OR compact.id IN (SELECT parent_id FROM requested_parents)
  ORDER BY compact.id
  FOR UPDATE OF compact
),
locked_target AS MATERIALIZED (
  SELECT compact.id
  FROM locked_compacts compact
  WHERE compact.id = sqlc.arg(rollup_id)
    AND compact.bot_id = sqlc.arg(scope_bot_id)
    AND sqlc.narg(scope_session_id)::uuid IS NOT NULL
    AND compact.session_id IS NOT DISTINCT FROM sqlc.narg(scope_session_id)::uuid
    AND compact.status = 'pending'
    AND compact.summary = ''
    AND compact.message_count = 0
    AND compact.error_message = ''
    AND compact.usage IS NULL
    AND compact.model_id IS NULL
    AND compact.artifact_version = 1
    AND compact.coverage = '[]'::jsonb
    AND compact.anchor_start_ms = 0
    AND compact.anchor_end_ms = 0
    AND compact.artifact_level = 0
    AND cardinality(compact.parent_ids) = 0
    AND compact.superseded_by IS NULL
    AND compact.superseded_at IS NULL
    AND compact.completed_at IS NULL
),
locked_parents AS MATERIALIZED (
  SELECT parent.*, requested.ordinal
  FROM requested_parents requested
  JOIN locked_compacts parent ON parent.id = requested.parent_id
),
parent_stats AS MATERIALIZED (
  SELECT
    COUNT(*)::integer AS matched_count,
    COALESCE(BOOL_AND(
      parent.bot_id = sqlc.arg(scope_bot_id)
      AND parent.session_id IS NOT DISTINCT FROM sqlc.narg(scope_session_id)::uuid
      AND parent.status = 'ok'
      AND BTRIM(parent.summary) <> ''
      AND parent.message_count > 0
      AND parent.artifact_version = 1
      AND parent.artifact_level >= 0
      AND parent.artifact_level < 2147483647
      AND parent.superseded_by IS NULL
      AND parent.superseded_at IS NULL
      AND parent.completed_at IS NOT NULL
      AND jsonb_typeof(parent.coverage) = 'array'
      AND jsonb_array_length(
        CASE WHEN jsonb_typeof(parent.coverage) = 'array' THEN parent.coverage ELSE '[]'::jsonb END
      ) > 0
      AND jsonb_array_length(
        CASE WHEN jsonb_typeof(parent.coverage) = 'array' THEN parent.coverage ELSE '[]'::jsonb END
      ) = parent.message_count
    ), false) AS valid
  FROM locked_parents parent
),
ancestry(artifact_id) AS (
  SELECT parent.id
  FROM locked_parents parent
  UNION
  SELECT edge.parent_id
  FROM bot_history_message_compact_parent_edges edge
  JOIN ancestry descendant ON descendant.artifact_id = edge.artifact_id
),
direct_ancestors AS MATERIALIZED (
  SELECT compact.id, compact.message_count
  FROM bot_history_message_compacts compact
  JOIN ancestry ON ancestry.artifact_id = compact.id
  WHERE compact.status = 'ok'
    AND compact.artifact_level = 0
),
locked_sources AS MATERIALIZED (
  SELECT
    message.id,
    message.compact_id,
    message.bot_id,
    message.session_id,
    message.turn_visible,
    message.turn_id,
    message.turn_position,
    message.turn_message_seq,
    message.metadata,
    message.compact_claim_finalized,
    message.compact_claim_invalidated
  FROM bot_history_messages message
  WHERE message.compact_id IN (SELECT id FROM direct_ancestors)
  ORDER BY message.id
  FOR SHARE OF message SKIP LOCKED
),
source_lock_barrier AS MATERIALIZED (
  SELECT COUNT(*)::integer AS locked_count
  FROM locked_sources
),
direct_source_stats AS MATERIALIZED (
  SELECT
    ancestor.id,
    ancestor.message_count,
    COUNT(source.id)::integer AS claimed_count,
    COUNT(source.id) FILTER (WHERE
      source.bot_id = sqlc.arg(scope_bot_id)
      AND source.session_id IS NOT DISTINCT FROM sqlc.narg(scope_session_id)::uuid
      AND source.turn_visible = true
      AND source.turn_id IS NOT NULL
      AND source.turn_position IS NOT NULL
      AND source.turn_message_seq IS NOT NULL
      AND (source.metadata->>'trigger_mode' IS NULL OR source.metadata->>'trigger_mode' <> 'passive_sync')
      AND source.compact_claim_finalized = true
      AND source.compact_claim_invalidated = false
    )::integer AS current_count
  FROM direct_ancestors ancestor
  CROSS JOIN source_lock_barrier
  LEFT JOIN locked_sources source ON source.compact_id = ancestor.id
  GROUP BY ancestor.id, ancestor.message_count
),
ancestor_stats AS MATERIALIZED (
  SELECT
    COUNT(*)::integer AS direct_count,
    COUNT(*) FILTER (WHERE
      source.message_count > 0
      AND source.claimed_count = source.message_count
      AND source.current_count = source.message_count
    )::integer AS current_count
  FROM direct_source_stats source
),
parent_coverage AS MATERIALIZED (
  SELECT
    parent.ordinal AS parent_ordinal,
    covered.ordinality::bigint AS source_ordinal,
    covered.source,
    CASE
      WHEN jsonb_typeof(covered.source->'created_at_ms') = 'number'
       AND COALESCE(covered.source->>'created_at_ms', '') ~ '^[0-9]+$'
      THEN (covered.source->>'created_at_ms')::bigint
    END AS created_at_ms
  FROM locked_parents parent
  CROSS JOIN LATERAL jsonb_array_elements(
    CASE
      WHEN jsonb_typeof(parent.coverage) = 'array' THEN parent.coverage
      ELSE '[]'::jsonb
    END
  ) WITH ORDINALITY AS covered(source, ordinality)
),
ordered_coverage AS MATERIALIZED (
  SELECT
    coverage.*,
    LAG(coverage.created_at_ms) OVER (
      ORDER BY coverage.parent_ordinal, coverage.source_ordinal
    ) AS previous_created_at_ms
  FROM parent_coverage coverage
),
parent_coverage_stats AS MATERIALIZED (
  SELECT
    parent.id,
    parent.message_count,
    parent.anchor_start_ms,
    parent.anchor_end_ms,
    COUNT(coverage.source)::integer AS coverage_count,
    MIN(coverage.created_at_ms)::bigint AS first_created_at_ms,
    MAX(coverage.created_at_ms)::bigint AS last_created_at_ms
  FROM locked_parents parent
  LEFT JOIN parent_coverage coverage ON coverage.parent_ordinal = parent.ordinal
  GROUP BY parent.id, parent.message_count, parent.anchor_start_ms, parent.anchor_end_ms
),
parent_coverage_validity AS MATERIALIZED (
  SELECT COALESCE(BOOL_AND(
    coverage_count = message_count
    AND anchor_start_ms = first_created_at_ms
    AND anchor_end_ms = last_created_at_ms
  ), false) AS valid
  FROM parent_coverage_stats
),
covered_messages AS MATERIALIZED (
  SELECT
    coverage.parent_ordinal,
    coverage.source_ordinal,
    message.id,
    message.turn_position,
    message.turn_message_seq,
    message.created_at
  FROM ordered_coverage coverage
  JOIN bot_history_messages message
    ON message.id::text = coverage.source->'ref'->>'id'
),
ordered_covered_messages AS MATERIALIZED (
  SELECT
    covered.*,
    LAG(covered.turn_position) OVER coverage_order AS previous_turn_position,
    LAG(covered.turn_message_seq) OVER coverage_order AS previous_turn_message_seq,
    LAG(covered.created_at) OVER coverage_order AS previous_created_at,
    LAG(covered.id) OVER coverage_order AS previous_id
  FROM covered_messages covered
  WINDOW coverage_order AS (ORDER BY covered.parent_ordinal, covered.source_ordinal)
),
coverage_message_stats AS MATERIALIZED (
  SELECT
    COUNT(*)::integer AS matched_count,
    COALESCE(BOOL_AND(
      previous_turn_position IS NULL
      OR ROW(turn_position, turn_message_seq, created_at, id) >=
         ROW(previous_turn_position, previous_turn_message_seq, previous_created_at, previous_id)
    ), false) AS ordered
  FROM ordered_covered_messages
),
coverage_bounds AS MATERIALIZED (
  SELECT
    (ARRAY_AGG(id ORDER BY parent_ordinal, source_ordinal))[1] AS first_id,
    (ARRAY_AGG(id ORDER BY parent_ordinal DESC, source_ordinal DESC))[1] AS last_id,
    (ARRAY_AGG(turn_position ORDER BY parent_ordinal, source_ordinal))[1] AS first_turn_position,
    (ARRAY_AGG(turn_position ORDER BY parent_ordinal DESC, source_ordinal DESC))[1] AS last_turn_position
  FROM covered_messages
),
topology_snapshot AS MATERIALIZED (
  SELECT COALESCE(counter.revision, 0)::bigint AS revision
  FROM (SELECT 1) seed
  LEFT JOIN bot_history_topology_counters counter
    ON counter.session_id = sqlc.narg(scope_session_id)::uuid
),
pending_topology_stats AS MATERIALIZED (
  SELECT COUNT(*)::integer AS pending_count
  FROM bot_history_topology_pending pending
  CROSS JOIN coverage_bounds bounds
  WHERE pending.transaction_id = pg_current_xact_id()
    AND pending.session_id = sqlc.narg(scope_session_id)::uuid
    AND pending.turn_position BETWEEN bounds.first_turn_position AND bounds.last_turn_position
),
gap_stats AS MATERIALIZED (
  SELECT COUNT(candidate.id)::integer AS visible_count
  FROM coverage_bounds bounds
  JOIN bot_history_messages first_source ON first_source.id = bounds.first_id
  JOIN bot_history_messages last_source ON last_source.id = bounds.last_id
  JOIN bot_history_messages candidate
    ON candidate.bot_id = sqlc.arg(scope_bot_id)
   AND candidate.session_id IS NOT DISTINCT FROM sqlc.narg(scope_session_id)::uuid
   AND candidate.turn_visible = true
   AND candidate.turn_id IS NOT NULL
   AND candidate.turn_position IS NOT NULL
   AND candidate.turn_message_seq IS NOT NULL
   AND (candidate.metadata->>'trigger_mode' IS NULL OR candidate.metadata->>'trigger_mode' <> 'passive_sync')
   AND ROW(candidate.turn_position, candidate.turn_message_seq, candidate.created_at, candidate.id) >=
       ROW(first_source.turn_position, first_source.turn_message_seq, first_source.created_at, first_source.id)
   AND ROW(candidate.turn_position, candidate.turn_message_seq, candidate.created_at, candidate.id) <=
       ROW(last_source.turn_position, last_source.turn_message_seq, last_source.created_at, last_source.id)
),
coverage_stats AS MATERIALIZED (
  SELECT
    COUNT(*)::integer AS coverage_count,
    COUNT(DISTINCT source->'ref'->>'id')::integer AS distinct_source_count,
    COUNT(*) FILTER (WHERE EXISTS (
      SELECT 1
      FROM locked_sources source_message
      WHERE source_message.id::text = source->'ref'->>'id'
    ))::integer AS matched_source_count,
    COALESCE(BOOL_AND(
      jsonb_typeof(source) = 'object'
      AND jsonb_typeof(source->'ref') = 'object'
      AND jsonb_typeof(source->'ref'->'namespace') = 'string'
      AND COALESCE(BTRIM(source->'ref'->>'namespace'), '') = 'bot_history_message'
      AND jsonb_typeof(source->'ref'->'id') = 'string'
      AND COALESCE(BTRIM(source->'ref'->>'id'), '') <> ''
      AND jsonb_typeof(source->'ref'->'version') = 'number'
      AND COALESCE(source->'ref'->>'version', '') = '1'
      AND jsonb_typeof(source->'ref'->'schema') = 'string'
      AND COALESCE(BTRIM(source->'ref'->>'schema'), '') = 'context_ref'
      AND jsonb_typeof(source->'ref'->'durability') = 'string'
      AND COALESCE(BTRIM(source->'ref'->>'durability'), '') = 'durable'
      AND jsonb_typeof(source->'ref'->'hash_algo') = 'string'
      AND COALESCE(BTRIM(source->'ref'->>'hash_algo'), '') = 'sha256'
      AND jsonb_typeof(source->'ref'->'hash_scope') = 'string'
      AND COALESCE(BTRIM(source->'ref'->>'hash_scope'), '') = 'source_payload'
      AND jsonb_typeof(source->'ref'->'content_hash') = 'string'
      AND COALESCE(source->'ref'->>'content_hash', '') ~ '^[0-9a-f]{64}$'
      AND source->'ref'->'range' IS NULL
      AND created_at_ms IS NOT NULL
      AND (previous_created_at_ms IS NULL OR created_at_ms >= previous_created_at_ms)
      AND (
        NOT source ? 'external_message_id'
        OR jsonb_typeof(source->'external_message_id') = 'string'
      )
      AND (
        NOT source ? 'source_reply_to_message_id'
        OR jsonb_typeof(source->'source_reply_to_message_id') = 'string'
      )
    ), false) AS valid,
    COALESCE(jsonb_agg(source ORDER BY parent_ordinal, source_ordinal), '[]'::jsonb) AS coverage,
    MIN(created_at_ms)::bigint AS anchor_start_ms,
    MAX(created_at_ms)::bigint AS anchor_end_ms
  FROM ordered_coverage
),
derived_shape AS MATERIALIZED (
  SELECT
    stats.coverage_count AS message_count,
    stats.coverage,
    stats.anchor_start_ms,
    stats.anchor_end_ms,
    (MAX(parent.artifact_level) + 1)::integer AS artifact_level,
    ARRAY_AGG(parent.id ORDER BY parent.ordinal)::uuid[] AS parent_ids,
    topology.revision AS topology_revision,
    bounds.first_turn_position AS range_start_turn_position,
    bounds.last_turn_position AS range_end_turn_position
  FROM coverage_stats stats
  CROSS JOIN locked_parents parent
  CROSS JOIN topology_snapshot topology
  CROSS JOIN coverage_bounds bounds
  GROUP BY
    stats.coverage_count,
    stats.coverage,
    stats.anchor_start_ms,
    stats.anchor_end_ms,
    topology.revision,
    bounds.first_turn_position,
    bounds.last_turn_position
),
eligible AS MATERIALIZED (
  SELECT target.id, shape.*
  FROM locked_target target
  CROSS JOIN request_shape request
  CROSS JOIN parent_stats parents
  CROSS JOIN ancestor_stats ancestors
  CROSS JOIN source_lock_barrier source_locks
  CROSS JOIN pending_topology_stats pending_topology
  CROSS JOIN parent_coverage_validity parent_coverage
  CROSS JOIN coverage_message_stats covered_messages
  CROSS JOIN gap_stats gaps
  CROSS JOIN coverage_stats coverage
  CROSS JOIN derived_shape shape
  WHERE request.requested_count >= 2
    AND request.requested_count = request.unique_count
    AND request.excludes_target
    AND parents.matched_count = request.requested_count
    AND parents.valid
    AND parent_coverage.valid
    AND ancestors.direct_count > 0
    AND ancestors.current_count = ancestors.direct_count
    AND pending_topology.pending_count = 0
    AND coverage.coverage_count > 0
    AND coverage.coverage_count = coverage.distinct_source_count
    AND coverage.coverage_count = coverage.matched_source_count
    AND coverage.coverage_count = source_locks.locked_count
    AND covered_messages.matched_count = coverage.coverage_count
    AND covered_messages.ordered
    AND shape.range_start_turn_position IS NOT NULL
    AND shape.range_end_turn_position IS NOT NULL
    AND shape.range_start_turn_position <= shape.range_end_turn_position
    AND gaps.visible_count = coverage.coverage_count
    AND coverage.valid
    AND BTRIM(sqlc.arg(summary)) <> ''
),
completed_rollup AS MATERIALIZED (
  UPDATE bot_history_message_compacts rollup
  SET status = 'ok',
      summary = BTRIM(sqlc.arg(summary)),
      message_count = eligible.message_count,
      error_message = '',
      usage = sqlc.narg(usage)::jsonb,
      model_id = sqlc.narg(model_id)::uuid,
      artifact_version = 1,
      coverage = eligible.coverage,
      anchor_start_ms = eligible.anchor_start_ms,
      anchor_end_ms = eligible.anchor_end_ms,
      artifact_level = eligible.artifact_level,
      parent_ids = eligible.parent_ids,
      completed_at = now()
  FROM eligible
  WHERE rollup.id = eligible.id
  RETURNING rollup.id
),
validated_rollup AS MATERIALIZED (
  INSERT INTO bot_history_message_compact_validations (compact_id)
  SELECT completed.id
  FROM completed_rollup completed
  ON CONFLICT (compact_id) DO UPDATE SET compact_id = EXCLUDED.compact_id
  RETURNING compact_id
),
recorded_topology AS MATERIALIZED (
  INSERT INTO bot_history_message_compact_topology (
    compact_id,
    session_id,
    topology_revision,
    range_start_turn_position,
    range_end_turn_position
  )
  SELECT
    completed.compact_id,
    sqlc.narg(scope_session_id)::uuid,
    eligible.topology_revision,
    eligible.range_start_turn_position,
    eligible.range_end_turn_position
  FROM validated_rollup completed
  JOIN eligible ON eligible.id = completed.compact_id
  RETURNING compact_id
),
superseded_parents AS (
  UPDATE bot_history_message_compacts parent
  SET superseded_by = recorded.compact_id,
      superseded_at = now()
  FROM recorded_topology recorded
  WHERE parent.id IN (SELECT parent_id FROM requested_parents)
  RETURNING parent.id
)
SELECT
  (
    (SELECT COUNT(*) FROM completed_rollup) = 1
    AND (SELECT COUNT(*) FROM validated_rollup) = 1
    AND (SELECT COUNT(*) FROM recorded_topology) = 1
    AND (SELECT COUNT(*) FROM superseded_parents) = request.requested_count
  )::boolean AS finalized,
  request.requested_count,
  parents.matched_count,
  (SELECT COUNT(*) FROM superseded_parents)::integer AS superseded_count
FROM request_shape request
CROSS JOIN parent_stats parents;
