-- 0080_add_orchestration_action_ledger
-- Add durable orchestration action ledger for worker and verifier tool calls.

CREATE TABLE IF NOT EXISTS orchestration_action_ledger (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  run_id UUID NOT NULL REFERENCES orchestration_runs(id) ON DELETE CASCADE,
  task_id UUID NOT NULL REFERENCES orchestration_tasks(id) ON DELETE CASCADE,
  attempt_id UUID,
  verification_id UUID,
  action_kind TEXT NOT NULL DEFAULT 'tool_call' CHECK (action_kind IN ('tool_call')),
  status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
  tool_name TEXT NOT NULL DEFAULT '',
  tool_call_id TEXT NOT NULL DEFAULT '',
  input_payload JSONB NOT NULL DEFAULT 'null'::jsonb,
  output_payload JSONB NOT NULL DEFAULT 'null'::jsonb,
  error_payload JSONB NOT NULL DEFAULT 'null'::jsonb,
  summary TEXT NOT NULL DEFAULT '',
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT orchestration_action_ledger_exactly_one_subject CHECK (
    (attempt_id IS NOT NULL AND verification_id IS NULL)
    OR (attempt_id IS NULL AND verification_id IS NOT NULL)
  )
);

CREATE INDEX IF NOT EXISTS idx_orchestration_action_ledger_run_started_at ON orchestration_action_ledger(run_id, started_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_action_ledger_task_started_at ON orchestration_action_ledger(task_id, started_at, id);
CREATE INDEX IF NOT EXISTS idx_orchestration_action_ledger_attempt_started_at ON orchestration_action_ledger(attempt_id, started_at, id) WHERE attempt_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_orchestration_action_ledger_verification_started_at ON orchestration_action_ledger(verification_id, started_at, id) WHERE verification_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestration_action_ledger_attempt_tool_call_unique ON orchestration_action_ledger(attempt_id, tool_call_id) WHERE attempt_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orchestration_action_ledger_verification_tool_call_unique ON orchestration_action_ledger(verification_id, tool_call_id) WHERE verification_id IS NOT NULL;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_action_ledger_task_run_fk') THEN
    ALTER TABLE orchestration_action_ledger
      ADD CONSTRAINT orchestration_action_ledger_task_run_fk
      FOREIGN KEY (task_id, run_id) REFERENCES orchestration_tasks(id, run_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_action_ledger_attempt_fk') THEN
    ALTER TABLE orchestration_action_ledger
      ADD CONSTRAINT orchestration_action_ledger_attempt_fk
      FOREIGN KEY (attempt_id, run_id, task_id) REFERENCES orchestration_task_attempts(id, run_id, task_id) ON DELETE CASCADE;
  END IF;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'orchestration_action_ledger_verification_fk') THEN
    ALTER TABLE orchestration_action_ledger
      ADD CONSTRAINT orchestration_action_ledger_verification_fk
      FOREIGN KEY (verification_id, run_id, task_id) REFERENCES orchestration_task_verifications(id, run_id, task_id) ON DELETE CASCADE;
  END IF;
END $$;
