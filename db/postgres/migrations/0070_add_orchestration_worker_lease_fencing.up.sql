-- 0070_add_orchestration_worker_lease_fencing
-- Persist worker lease snapshots on claimed work and enforce one open run barrier per run.

ALTER TABLE orchestration_task_attempts
  ADD COLUMN IF NOT EXISTS worker_lease_token TEXT NOT NULL DEFAULT '';

ALTER TABLE orchestration_task_verifications
  ADD COLUMN IF NOT EXISTS worker_lease_token TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestration_human_checkpoints_open_run_barrier_unique
  ON orchestration_human_checkpoints(run_id)
  WHERE blocks_run = TRUE AND status = 'open';
