-- 0069_add_orchestration_verifications
-- Add verifier work queue for orchestration task result validation.

ALTER TABLE orchestration_tasks
  DROP CONSTRAINT IF EXISTS orchestration_tasks_status_check;

ALTER TABLE orchestration_tasks
  ADD CONSTRAINT orchestration_tasks_status_check
  CHECK (status IN ('created', 'ready', 'dispatching', 'running', 'verifying', 'waiting_human', 'completed', 'blocked', 'failed', 'cancelled'));

CREATE TABLE IF NOT EXISTS orchestration_task_verifications (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL UNIQUE REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  result_id UUID NOT NULL UNIQUE,
  attempt_no INTEGER NOT NULL DEFAULT 1,
  worker_id TEXT NOT NULL DEFAULT '',
  executor_id TEXT NOT NULL DEFAULT '',
  verifier_profile TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL CHECK (status IN ('created', 'claimed', 'running', 'completed', 'failed', 'lost')),
  claim_epoch BIGINT NOT NULL DEFAULT 0,
  claim_token TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  last_heartbeat_at TIMESTAMPTZ,
  verdict TEXT NOT NULL DEFAULT '' CHECK (verdict IN ('', 'accepted', 'rejected')),
  summary TEXT NOT NULL DEFAULT '',
  failure_class TEXT NOT NULL DEFAULT '',
  terminal_reason TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_task_verifications_id_run_task_unique UNIQUE (id, run_id, task_id),
  CONSTRAINT orchestration_task_verifications_id_run_result_unique UNIQUE (id, run_id, result_id)
);

CREATE INDEX IF NOT EXISTS idx_orchestration_task_verifications_run_status ON orchestration_task_verifications(run_id, status, created_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_task_verifications_claim_queue ON orchestration_task_verifications(status, verifier_profile, created_at, id) WHERE status = 'created';
CREATE INDEX IF NOT EXISTS idx_orchestration_task_verifications_lease_expiry ON orchestration_task_verifications(lease_expires_at, id) WHERE status IN ('claimed', 'running') AND lease_expires_at IS NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_verifications_task_run_fk') THEN
    ALTER TABLE orchestration_task_verifications
      ADD CONSTRAINT orchestration_task_verifications_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_task_verifications_result_fk') THEN
    ALTER TABLE orchestration_task_verifications
      ADD CONSTRAINT orchestration_task_verifications_result_fk
      FOREIGN KEY (result_id, run_id, task_id) REFERENCES orchestration_task_results(id, run_id, task_id) ON DELETE CASCADE;
  END IF;
END $$;
