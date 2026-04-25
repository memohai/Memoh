-- 0068_add_orchestration_kernel (down)
-- Drop the final orchestration kernel tables.

DROP TABLE IF EXISTS orchestration_workers;
DROP TABLE IF EXISTS orchestration_idempotency_records;
DROP TABLE IF EXISTS orchestration_projection_snapshots;
DROP TABLE IF EXISTS orchestration_events;
DROP TABLE IF EXISTS orchestration_planning_intents;
DROP TABLE IF EXISTS orchestration_task_dependencies;
DROP TABLE IF EXISTS orchestration_artifacts;
DROP TABLE IF EXISTS orchestration_task_attempts;
DROP TABLE IF EXISTS orchestration_input_manifests;
DROP TABLE IF EXISTS orchestration_task_results;
DROP TABLE IF EXISTS orchestration_human_checkpoints;
DROP TABLE IF EXISTS orchestration_tasks CASCADE;
DROP TABLE IF EXISTS orchestration_runs;
