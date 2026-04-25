-- name: CreateOrchestrationRun :one
INSERT INTO orchestration_runs (
  id,
  tenant_id,
  owner_subject,
  lifecycle_status,
  planning_status,
  status_version,
  planner_epoch,
  last_event_seq,
  root_task_id,
  goal,
  input,
  output_schema,
  requested_control_policy,
  control_policy,
  source_metadata,
  policies,
  created_by,
  terminal_reason
) VALUES (
  sqlc.arg(id),
  sqlc.arg(tenant_id),
  sqlc.arg(owner_subject),
  sqlc.arg(lifecycle_status),
  sqlc.arg(planning_status),
  sqlc.arg(status_version),
  sqlc.arg(planner_epoch),
  sqlc.arg(last_event_seq),
  sqlc.arg(root_task_id),
  sqlc.arg(goal),
  sqlc.arg(input),
  sqlc.arg(output_schema),
  sqlc.arg(requested_control_policy),
  sqlc.arg(control_policy),
  sqlc.arg(source_metadata),
  sqlc.arg(policies),
  sqlc.arg(created_by),
  sqlc.arg(terminal_reason)
) RETURNING *;

-- name: GetOrchestrationRunByID :one
SELECT *
FROM orchestration_runs
WHERE id = sqlc.arg(id);

-- name: GetOrchestrationRunByIDForUpdate :one
SELECT *
FROM orchestration_runs
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: AllocateOrchestrationRunEventSeqs :one
UPDATE orchestration_runs
SET last_event_seq = last_event_seq + sqlc.arg(delta)::bigint,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING last_event_seq;

-- name: MarkOrchestrationRunRunning :one
UPDATE orchestration_runs
SET lifecycle_status = 'running',
    status_version = status_version + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkOrchestrationRunWaitingHuman :one
UPDATE orchestration_runs
SET lifecycle_status = 'waiting_human',
    status_version = status_version + 1,
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateOrchestrationTask :one
INSERT INTO orchestration_tasks (
  id,
  run_id,
  decomposed_from_task_id,
  kind,
  goal,
  inputs,
  planner_epoch,
  worker_profile,
  priority,
  retry_policy,
  verification_policy,
  status,
  status_version,
  waiting_scope,
  blocked_reason,
  terminal_reason,
  blackboard_scope
) VALUES (
  sqlc.arg(id),
  sqlc.arg(run_id),
  sqlc.arg(decomposed_from_task_id),
  sqlc.arg(kind),
  sqlc.arg(goal),
  sqlc.arg(inputs),
  sqlc.arg(planner_epoch),
  sqlc.arg(worker_profile),
  sqlc.arg(priority),
  sqlc.arg(retry_policy),
  sqlc.arg(verification_policy),
  sqlc.arg(status),
  sqlc.arg(status_version),
  sqlc.arg(waiting_scope),
  sqlc.arg(blocked_reason),
  sqlc.arg(terminal_reason),
  sqlc.arg(blackboard_scope)
) RETURNING *;

-- name: GetOrchestrationTaskByID :one
SELECT *
FROM orchestration_tasks
WHERE id = sqlc.arg(id);

-- name: GetOrchestrationTaskByIDForUpdate :one
SELECT *
FROM orchestration_tasks
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: ListCurrentOrchestrationTasksByRun :many
SELECT *
FROM orchestration_tasks
WHERE run_id = sqlc.arg(run_id)
ORDER BY created_at ASC, id ASC;

-- name: CountOpenRunBlockingCheckpointsByRun :one
SELECT COUNT(*)
FROM orchestration_human_checkpoints
WHERE run_id = sqlc.arg(run_id)
  AND blocks_run = TRUE
  AND status = 'open';

-- name: MarkOrchestrationTaskWaitingHuman :one
UPDATE orchestration_tasks
SET status = 'waiting_human',
    status_version = status_version + 1,
    waiting_checkpoint_id = sqlc.arg(waiting_checkpoint_id),
    waiting_scope = sqlc.arg(waiting_scope),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: MarkOrchestrationTaskReadyFromCheckpoint :one
UPDATE orchestration_tasks
SET status = 'ready',
    status_version = status_version + 1,
    waiting_checkpoint_id = NULL,
    waiting_scope = '',
    ready_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateOrchestrationTaskResult :one
INSERT INTO orchestration_task_results (
  run_id,
  task_id,
  attempt_id,
  status,
  summary,
  failure_class,
  request_replan,
  artifact_intents,
  structured_output
) VALUES (
  sqlc.arg(run_id),
  sqlc.arg(task_id),
  sqlc.arg(attempt_id),
  sqlc.arg(status),
  sqlc.arg(summary),
  sqlc.arg(failure_class),
  sqlc.arg(request_replan),
  sqlc.arg(artifact_intents),
  sqlc.arg(structured_output)
) ON CONFLICT (task_id) DO UPDATE
SET attempt_id = EXCLUDED.attempt_id,
    status = EXCLUDED.status,
    summary = EXCLUDED.summary,
    failure_class = EXCLUDED.failure_class,
    request_replan = EXCLUDED.request_replan,
    artifact_intents = EXCLUDED.artifact_intents,
    structured_output = EXCLUDED.structured_output,
    updated_at = now()
RETURNING *;

-- name: ListCurrentOrchestrationArtifactsByRun :many
SELECT *
FROM orchestration_artifacts
WHERE run_id = sqlc.arg(run_id)
ORDER BY created_at ASC, id ASC;

-- name: CreateOrchestrationArtifact :one
INSERT INTO orchestration_artifacts (
  id,
  run_id,
  task_id,
  attempt_id,
  kind,
  uri,
  version,
  digest,
  content_type,
  summary,
  metadata
) VALUES (
  sqlc.arg(id),
  sqlc.arg(run_id),
  sqlc.arg(task_id),
  sqlc.arg(attempt_id),
  sqlc.arg(kind),
  sqlc.arg(uri),
  sqlc.arg(version),
  sqlc.arg(digest),
  sqlc.arg(content_type),
  sqlc.arg(summary),
  sqlc.arg(metadata)
) RETURNING *;

-- name: CreateOrchestrationHumanCheckpoint :one
INSERT INTO orchestration_human_checkpoints (
  id,
  run_id,
  task_id,
  blocks_run,
  planner_epoch,
  status,
  status_version,
  question,
  options,
  default_action,
  resume_policy,
  timeout_at,
  metadata
) VALUES (
  sqlc.arg(id),
  sqlc.arg(run_id),
  sqlc.arg(task_id),
  sqlc.arg(blocks_run),
  sqlc.arg(planner_epoch),
  sqlc.arg(status),
  sqlc.arg(status_version),
  sqlc.arg(question),
  sqlc.arg(options),
  sqlc.arg(default_action),
  sqlc.arg(resume_policy),
  sqlc.arg(timeout_at),
  sqlc.arg(metadata)
) RETURNING *;

-- name: GetOrchestrationHumanCheckpointByID :one
SELECT *
FROM orchestration_human_checkpoints
WHERE id = sqlc.arg(id);

-- name: GetOrchestrationHumanCheckpointByIDForUpdate :one
SELECT *
FROM orchestration_human_checkpoints
WHERE id = sqlc.arg(id)
FOR UPDATE;

-- name: ListCurrentOrchestrationCheckpointsByRun :many
SELECT *
FROM orchestration_human_checkpoints
WHERE run_id = sqlc.arg(run_id)
ORDER BY created_at ASC, id ASC;

-- name: ResolveOrchestrationHumanCheckpoint :one
UPDATE orchestration_human_checkpoints
SET status = 'resolved',
    status_version = status_version + 1,
    resolved_by = sqlc.arg(resolved_by),
    resolved_mode = sqlc.arg(resolved_mode),
    resolved_option_id = sqlc.arg(resolved_option_id),
    resolved_freeform_input = sqlc.arg(resolved_freeform_input),
    resolved_at = now(),
    updated_at = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: CreateOrchestrationEvent :one
INSERT INTO orchestration_events (
  run_id,
  task_id,
  attempt_id,
  checkpoint_id,
  seq,
  aggregate_type,
  aggregate_id,
  aggregate_version,
  type,
  causation_event_id,
  correlation_id,
  idempotency_key,
  payload
) VALUES (
  sqlc.arg(run_id),
  sqlc.arg(task_id),
  sqlc.arg(attempt_id),
  sqlc.arg(checkpoint_id),
  sqlc.arg(seq),
  sqlc.arg(aggregate_type),
  sqlc.arg(aggregate_id),
  sqlc.arg(aggregate_version),
  sqlc.arg(type),
  sqlc.arg(causation_event_id),
  sqlc.arg(correlation_id),
  sqlc.arg(idempotency_key),
  sqlc.arg(payload)
) RETURNING *;

-- name: ListOrchestrationRunEvents :many
SELECT *
FROM orchestration_events
WHERE run_id = sqlc.arg(run_id)
  AND seq > sqlc.arg(after_seq)
  AND seq <= sqlc.arg(until_seq)
ORDER BY seq ASC
LIMIT sqlc.arg(limit_count);

-- name: CreateOrchestrationProjectionSnapshot :one
INSERT INTO orchestration_projection_snapshots (
  run_id,
  projection_kind,
  seq,
  payload
) VALUES (
  sqlc.arg(run_id),
  sqlc.arg(projection_kind),
  sqlc.arg(seq),
  sqlc.arg(payload)
) RETURNING *;

-- name: GetOrchestrationProjectionSnapshotAtOrBeforeSeq :one
SELECT *
FROM orchestration_projection_snapshots
WHERE run_id = sqlc.arg(run_id)
  AND projection_kind = sqlc.arg(projection_kind)
  AND seq <= sqlc.arg(seq)
ORDER BY seq DESC
LIMIT 1;

-- name: TryCreateOrchestrationIdempotencyRecord :one
INSERT INTO orchestration_idempotency_records (
  tenant_id,
  caller_subject,
  method,
  target_id,
  idempotency_key,
  request_hash,
  state
) VALUES (
  sqlc.arg(tenant_id),
  sqlc.arg(caller_subject),
  sqlc.arg(method),
  sqlc.arg(target_id),
  sqlc.arg(idempotency_key),
  sqlc.arg(request_hash),
  'in_progress'
) ON CONFLICT (tenant_id, caller_subject, method, target_id, idempotency_key) DO NOTHING
RETURNING *;

-- name: GetOrchestrationIdempotencyRecordForUpdate :one
SELECT *
FROM orchestration_idempotency_records
WHERE tenant_id = sqlc.arg(tenant_id)
  AND caller_subject = sqlc.arg(caller_subject)
  AND method = sqlc.arg(method)
  AND target_id = sqlc.arg(target_id)
  AND idempotency_key = sqlc.arg(idempotency_key)
FOR UPDATE;

-- name: CompleteOrchestrationIdempotencyRecord :one
UPDATE orchestration_idempotency_records
SET state = 'completed',
    response_payload = sqlc.arg(response_payload),
    updated_at = now()
WHERE tenant_id = sqlc.arg(tenant_id)
  AND caller_subject = sqlc.arg(caller_subject)
  AND method = sqlc.arg(method)
  AND target_id = sqlc.arg(target_id)
  AND idempotency_key = sqlc.arg(idempotency_key)
RETURNING *;
