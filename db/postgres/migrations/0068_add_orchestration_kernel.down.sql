-- 0068_add_orchestration_kernel (down)
-- Drop the phase-1 orchestration kernel tables.

DROP TABLE IF EXISTS orchestration_idempotency_records;
DROP TABLE IF EXISTS orchestration_projection_snapshots;
DROP TABLE IF EXISTS orchestration_events;
DROP TABLE IF EXISTS orchestration_human_checkpoints;
DROP TABLE IF EXISTS orchestration_artifacts;
DROP TABLE IF EXISTS orchestration_task_results;
DROP TABLE IF EXISTS orchestration_tasks;
DROP TABLE IF EXISTS orchestration_runs;
