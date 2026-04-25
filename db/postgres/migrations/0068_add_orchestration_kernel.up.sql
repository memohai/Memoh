-- 0068_add_orchestration_kernel
-- Add the final orchestration kernel schema, including runtime tables and integrity constraints.

CREATE TABLE IF NOT EXISTS orchestration_runs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id TEXT NOT NULL,
  owner_subject TEXT NOT NULL,
  lifecycle_status TEXT NOT NULL CHECK (lifecycle_status IN ('created', 'running', 'waiting_human', 'cancelling', 'completed', 'failed', 'cancelled')),
  planning_status TEXT NOT NULL CHECK (planning_status IN ('idle', 'active')),
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
  status TEXT NOT NULL CHECK (status IN ('created', 'ready', 'dispatching', 'running', 'verifying', 'waiting_human', 'completed', 'blocked', 'failed', 'cancelled')),
  status_version BIGINT NOT NULL DEFAULT 1,
  waiting_checkpoint_id UUID,
  waiting_scope TEXT NOT NULL DEFAULT '' CHECK (waiting_scope IN ('', 'task', 'run')),
  latest_result_id UUID,
  ready_at TIMESTAMPTZ,
  blocked_reason TEXT NOT NULL DEFAULT '',
  terminal_reason TEXT NOT NULL DEFAULT '',
  blackboard_scope TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_tasks_id_run_unique UNIQUE (id, run_id)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_tasks_run_created_at ON orchestration_tasks(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_tasks_run_status ON orchestration_tasks(run_id, status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_tasks_waiting_checkpoint ON orchestration_tasks(waiting_checkpoint_id) WHERE waiting_checkpoint_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS orchestration_input_manifests (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  captured_task_inputs JSONB NOT NULL DEFAULT '{}'::jsonb,
  captured_artifact_versions JSONB NOT NULL DEFAULT '[]'::jsonb,
  captured_blackboard_revisions JSONB NOT NULL DEFAULT '[]'::jsonb,
  projection_hash TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_input_manifests_id_run_task_unique UNIQUE (id, run_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_input_manifests_task_created_at ON orchestration_input_manifests(task_id, created_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS orchestration_task_results (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL UNIQUE REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  attempt_id UUID,
  status TEXT NOT NULL DEFAULT 'completed' CHECK (status IN ('completed', 'failed')),
  summary TEXT NOT NULL DEFAULT '',
  failure_class TEXT NOT NULL DEFAULT '',
  request_replan BOOLEAN NOT NULL DEFAULT FALSE,
  artifact_intents JSONB NOT NULL DEFAULT '[]'::jsonb,
  structured_output JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_task_results_id_run_task_unique UNIQUE (id, run_id, task_id)
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
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_artifacts_id_run_task_unique UNIQUE (id, run_id, task_id)
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
  status TEXT NOT NULL CHECK (status IN ('open', 'resolved', 'timed_out', 'cancelled', 'superseded')),
  status_version BIGINT NOT NULL DEFAULT 1,
  question TEXT NOT NULL DEFAULT '',
  options JSONB NOT NULL DEFAULT '[]'::jsonb,
  default_action JSONB NOT NULL DEFAULT '{}'::jsonb,
  resume_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
  timeout_at TIMESTAMPTZ,
  resolved_by TEXT NOT NULL DEFAULT '',
  resolved_mode TEXT NOT NULL DEFAULT '' CHECK (resolved_mode IN ('', 'select_option', 'freeform', 'use_default')),
  resolved_option_id TEXT NOT NULL DEFAULT '',
  resolved_freeform_input TEXT NOT NULL DEFAULT '',
  resolved_at TIMESTAMPTZ,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_human_checkpoints_id_run_unique UNIQUE (id, run_id),
  CONSTRAINT orchestration_human_checkpoints_id_run_task_unique UNIQUE (id, run_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_human_checkpoints_run_created_at ON orchestration_human_checkpoints(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_human_checkpoints_run_status ON orchestration_human_checkpoints(run_id, status, created_at, id);

CREATE TABLE IF NOT EXISTS orchestration_planning_intents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  checkpoint_id UUID,
  kind TEXT NOT NULL CHECK (kind IN ('start_run', 'checkpoint_resume', 'attempt_finalize', 'replan')),
  status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
  base_planner_epoch BIGINT NOT NULL DEFAULT 0,
  claim_epoch BIGINT NOT NULL DEFAULT 0,
  claim_token TEXT NOT NULL DEFAULT '',
  claimed_by TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  last_heartbeat_at TIMESTAMPTZ,
  failure_reason TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_planning_intents_checkpoint_requires_task CHECK (checkpoint_id IS NULL OR task_id IS NOT NULL),
  CONSTRAINT orchestration_planning_intents_id_run_unique UNIQUE (id, run_id)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_planning_intents_run_created_at ON orchestration_planning_intents(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_planning_intents_status_created_at ON orchestration_planning_intents(status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_planning_intents_lease_expires_at ON orchestration_planning_intents(lease_expires_at) WHERE lease_expires_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS orchestration_task_dependencies (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  predecessor_task_id UUID NOT NULL,
  successor_task_id UUID NOT NULL,
  planner_epoch BIGINT NOT NULL DEFAULT 1,
  superseded_by_planner_epoch BIGINT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_task_dependencies_no_self_edge CHECK (predecessor_task_id <> successor_task_id),
  CONSTRAINT orchestration_task_dependencies_unique UNIQUE (run_id, predecessor_task_id, successor_task_id, planner_epoch)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_task_dependencies_successor ON orchestration_task_dependencies(successor_task_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_task_dependencies_predecessor ON orchestration_task_dependencies(predecessor_task_id, created_at, id);

CREATE TABLE IF NOT EXISTS orchestration_task_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  attempt_no INTEGER NOT NULL,
  worker_id TEXT NOT NULL DEFAULT '',
  executor_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('created', 'claimed', 'running', 'completed', 'failed', 'lost')),
  claim_epoch BIGINT NOT NULL DEFAULT 0,
  claim_token TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  last_heartbeat_at TIMESTAMPTZ,
  input_manifest_id UUID,
  park_checkpoint_id UUID,
  failure_class TEXT NOT NULL DEFAULT '',
  terminal_reason TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_task_attempts_task_attempt_no_unique UNIQUE (task_id, attempt_no),
  CONSTRAINT orchestration_task_attempts_id_run_task_unique UNIQUE (id, run_id, task_id)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_task_attempts_run_created_at ON orchestration_task_attempts(run_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_task_attempts_task_created_at ON orchestration_task_attempts(task_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_task_attempts_status_created_at ON orchestration_task_attempts(status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_task_attempts_lease_expires_at ON orchestration_task_attempts(lease_expires_at) WHERE lease_expires_at IS NOT NULL;

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
  projection_kind TEXT NOT NULL CHECK (projection_kind IN ('tasks', 'checkpoints', 'artifacts', 'run')),
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
  state TEXT NOT NULL DEFAULT 'in_progress' CHECK (state IN ('in_progress', 'completed')),
  response_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_idempotency_records_unique UNIQUE (tenant_id, caller_subject, method, target_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_idempotency_records_lookup ON orchestration_idempotency_records(tenant_id, caller_subject, method, target_id, idempotency_key);

CREATE TABLE IF NOT EXISTS orchestration_workers (
  id TEXT PRIMARY KEY,
  executor_id TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'unavailable')),
  lease_token TEXT NOT NULL DEFAULT '',
  last_heartbeat_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  lease_expires_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orchestration_workers_status_lease_expires_at ON orchestration_workers(status, lease_expires_at);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_runs_root_task_fk') THEN
    ALTER TABLE orchestration_runs
      ADD CONSTRAINT orchestration_runs_root_task_fk
      FOREIGN KEY (root_task_id, id) REFERENCES orchestration_tasks(id, run_id)
      DEFERRABLE INITIALLY DEFERRED;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_input_manifests_task_run_fk') THEN
    ALTER TABLE orchestration_input_manifests
      ADD CONSTRAINT orchestration_input_manifests_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_results_task_run_fk') THEN
    ALTER TABLE orchestration_task_results
      ADD CONSTRAINT orchestration_task_results_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_results_attempt_fk') THEN
    ALTER TABLE orchestration_task_results
      ADD CONSTRAINT orchestration_task_results_attempt_fk
      FOREIGN KEY (attempt_id, run_id, task_id) REFERENCES orchestration_task_attempts(id, run_id, task_id);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_artifacts_task_run_fk') THEN
    ALTER TABLE orchestration_artifacts
      ADD CONSTRAINT orchestration_artifacts_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_artifacts_attempt_fk') THEN
    ALTER TABLE orchestration_artifacts
      ADD CONSTRAINT orchestration_artifacts_attempt_fk
      FOREIGN KEY (attempt_id, run_id, task_id) REFERENCES orchestration_task_attempts(id, run_id, task_id);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_human_checkpoints_task_run_fk') THEN
    ALTER TABLE orchestration_human_checkpoints
      ADD CONSTRAINT orchestration_human_checkpoints_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_planning_intents_task_run_fk') THEN
    ALTER TABLE orchestration_planning_intents
      ADD CONSTRAINT orchestration_planning_intents_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_planning_intents_checkpoint_run_fk') THEN
    ALTER TABLE orchestration_planning_intents
      ADD CONSTRAINT orchestration_planning_intents_checkpoint_run_fk
      FOREIGN KEY (checkpoint_id, run_id, task_id) REFERENCES orchestration_human_checkpoints(id, run_id, task_id)
      ON DELETE SET NULL (checkpoint_id);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_dependencies_predecessor_fk') THEN
    ALTER TABLE orchestration_task_dependencies
      ADD CONSTRAINT orchestration_task_dependencies_predecessor_fk
      FOREIGN KEY (predecessor_task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_dependencies_successor_fk') THEN
    ALTER TABLE orchestration_task_dependencies
      ADD CONSTRAINT orchestration_task_dependencies_successor_fk
      FOREIGN KEY (successor_task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_attempts_task_run_fk') THEN
    ALTER TABLE orchestration_task_attempts
      ADD CONSTRAINT orchestration_task_attempts_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_attempts_manifest_fk') THEN
    ALTER TABLE orchestration_task_attempts
      ADD CONSTRAINT orchestration_task_attempts_manifest_fk
      FOREIGN KEY (input_manifest_id, run_id, task_id) REFERENCES orchestration_input_manifests(id, run_id, task_id)
      ON DELETE SET NULL (input_manifest_id);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_attempts_checkpoint_fk') THEN
    ALTER TABLE orchestration_task_attempts
      ADD CONSTRAINT orchestration_task_attempts_checkpoint_fk
      FOREIGN KEY (park_checkpoint_id, run_id, task_id) REFERENCES orchestration_human_checkpoints(id, run_id, task_id)
      ON DELETE SET NULL (park_checkpoint_id);
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_tasks_waiting_checkpoint_fk') THEN
    ALTER TABLE orchestration_tasks
      ADD CONSTRAINT orchestration_tasks_waiting_checkpoint_fk
      FOREIGN KEY (waiting_checkpoint_id, run_id) REFERENCES orchestration_human_checkpoints(id, run_id)
      DEFERRABLE INITIALLY DEFERRED;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_tasks_latest_result_fk') THEN
    ALTER TABLE orchestration_tasks
      ADD CONSTRAINT orchestration_tasks_latest_result_fk
      FOREIGN KEY (latest_result_id, run_id, id) REFERENCES orchestration_task_results(id, run_id, task_id)
      DEFERRABLE INITIALLY DEFERRED;
  END IF;
END $$;
