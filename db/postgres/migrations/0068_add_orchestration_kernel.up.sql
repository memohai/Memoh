-- 0068_add_orchestration_kernel
-- Add the phase-1 orchestration kernel tables for runs, tasks, checkpoints, events, snapshots, and idempotency.

CREATE TABLE IF NOT EXISTS orchestration_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  owner_subject TEXT NOT NULL,
  lifecycle_status TEXT NOT NULL,
  planning_status TEXT NOT NULL,
  status_version BIGINT NOT NULL DEFAULT 1,
  planner_epoch BIGINT NOT NULL DEFAULT 1,
  last_event_seq BIGINT NOT NULL DEFAULT 0,
  root_task_id UUID NOT NULL,
  goal TEXT NOT NULL DEFAULT '',
  input JSONB NOT NULL DEFAULT '{}'::jsonb,
  output_schema JSONB NOT NULL DEFAULT '{}'::jsonb,
  requested_control_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  control_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  source_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  policies JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_by TEXT NOT NULL,
  terminal_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_orchestration_runs_owner_created_at ON orchestration_runs(owner_subject, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_orchestration_runs_lifecycle_status ON orchestration_runs(lifecycle_status);

CREATE TABLE IF NOT EXISTS orchestration_tasks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  decomposed_from_task_id UUID,
  kind TEXT NOT NULL DEFAULT 'step',
  goal TEXT NOT NULL DEFAULT '',
  inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
  planner_epoch BIGINT NOT NULL DEFAULT 1,
  superseded_by_planner_epoch BIGINT,
  worker_profile TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  retry_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  verification_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL,
  status_version BIGINT NOT NULL DEFAULT 1,
  waiting_checkpoint_id UUID,
  waiting_scope TEXT NOT NULL DEFAULT '' CHECK (waiting_scope IN ('', 'task', 'run')),
  latest_result_id UUID,
  ready_at TIMESTAMPTZ,
  blocked_reason TEXT NOT NULL DEFAULT '',
  terminal_reason TEXT NOT NULL DEFAULT '',
  blackboard_scope TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orchestration_tasks_run_created_at ON orchestration_tasks(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_tasks_run_status ON orchestration_tasks(run_id, status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_tasks_waiting_checkpoint ON orchestration_tasks(waiting_checkpoint_id) WHERE waiting_checkpoint_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS orchestration_task_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL UNIQUE REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  attempt_id UUID,
  status TEXT NOT NULL DEFAULT 'completed',
  summary TEXT NOT NULL DEFAULT '',
  failure_class TEXT NOT NULL DEFAULT '',
  request_replan BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_intents JSONB NOT NULL DEFAULT '[]'::jsonb,
  structured_output JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orchestration_task_results_run_created_at ON orchestration_task_results(run_id, created_at DESC);

CREATE TABLE IF NOT EXISTS orchestration_artifacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  attempt_id UUID,
  kind TEXT NOT NULL,
  uri TEXT NOT NULL,
  version TEXT NOT NULL,
  digest TEXT NOT NULL,
  content_type TEXT NOT NULL DEFAULT '',
  summary TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orchestration_artifacts_run_created_at ON orchestration_artifacts(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_artifacts_task_created_at ON orchestration_artifacts(task_id, created_at, id);

CREATE TABLE IF NOT EXISTS orchestration_human_checkpoints (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  blocks_run BOOLEAN NOT NULL DEFAULT FALSE,
  planner_epoch BIGINT NOT NULL DEFAULT 1,
  superseded_by_planner_epoch BIGINT,
  status TEXT NOT NULL,
  status_version BIGINT NOT NULL DEFAULT 1,
  question TEXT NOT NULL DEFAULT '',
  options JSONB NOT NULL DEFAULT '[]'::jsonb,
  default_action JSONB NOT NULL DEFAULT '{}'::jsonb,
  resume_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  timeout_at TIMESTAMPTZ,
  resolved_by TEXT NOT NULL DEFAULT '',
  resolved_mode TEXT NOT NULL DEFAULT '',
  resolved_option_id TEXT NOT NULL DEFAULT '',
  resolved_freeform_input TEXT NOT NULL DEFAULT '',
  resolved_at TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orchestration_human_checkpoints_run_created_at ON orchestration_human_checkpoints(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_human_checkpoints_run_status ON orchestration_human_checkpoints(run_id, status, created_at, id);

CREATE TABLE IF NOT EXISTS orchestration_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID,
  attempt_id UUID,
  checkpoint_id UUID,
  seq BIGINT NOT NULL,
  aggregate_type TEXT NOT NULL,
  aggregate_id UUID NOT NULL,
  aggregate_version BIGINT NOT NULL,
  type TEXT NOT NULL,
  causation_event_id UUID,
  correlation_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  CONSTRAINT orchestration_events_run_seq_unique UNIQUE (run_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_events_run_seq ON orchestration_events(run_id, seq);
CREATE INDEX IF NOT EXISTS idx_orchestration_events_task_seq ON orchestration_events(task_id, seq) WHERE task_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orchestration_events_checkpoint_seq ON orchestration_events(checkpoint_id, seq) WHERE checkpoint_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS orchestration_projection_snapshots (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  projection_kind TEXT NOT NULL,
  seq BIGINT NOT NULL,
  payload JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_projection_snapshots_unique UNIQUE (run_id, projection_kind, seq)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_projection_snapshots_lookup ON orchestration_projection_snapshots(run_id, projection_kind, seq DESC);

CREATE TABLE IF NOT EXISTS orchestration_idempotency_records (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  caller_subject TEXT NOT NULL,
  method TEXT NOT NULL,
  target_id TEXT NOT NULL DEFAULT '',
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'in_progress',
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_idempotency_records_unique UNIQUE (tenant_id, caller_subject, method, target_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_idempotency_records_lookup ON orchestration_idempotency_records(tenant_id, caller_subject, method, target_id, idempotency_key);
