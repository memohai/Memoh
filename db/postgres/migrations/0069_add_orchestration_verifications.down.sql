-- 0069_add_orchestration_verifications (down)
-- Drop orchestration verifier work queue.

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM orchestration_tasks
    WHERE status = 'verifying'
  ) THEN
    RAISE EXCEPTION 'cannot roll back 0069 while orchestration_tasks.status contains verifying rows';
  END IF;
END $$;

DROP TABLE IF EXISTS orchestration_task_verifications;

ALTER TABLE orchestration_tasks
  DROP CONSTRAINT IF EXISTS orchestration_tasks_status_check;

ALTER TABLE orchestration_tasks
  ADD CONSTRAINT orchestration_tasks_status_check
  CHECK (status IN ('created', 'ready', 'dispatching', 'running', 'waiting_human', 'completed', 'blocked', 'failed', 'cancelled'));
