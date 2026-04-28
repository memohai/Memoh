-- 0070_add_orchestration_worker_lease_fencing
-- Remove worker lease snapshot columns and run barrier uniqueness.

DROP INDEX IF EXISTS idx_orchestration_human_checkpoints_open_run_barrier_unique;

ALTER TABLE orchestration_task_verifications
  DROP COLUMN IF EXISTS worker_lease_token;

ALTER TABLE orchestration_task_attempts
  DROP COLUMN IF EXISTS worker_lease_token;
