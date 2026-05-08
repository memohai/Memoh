-- 0082_add_orchestration_env_runtime
-- Drop the Stage 3 env runtime tables in reverse dependency order so
-- foreign keys unwind cleanly. The base orchestration tables stay
-- untouched.

DROP INDEX IF EXISTS idx_orchestration_env_snapshots_run_kind;
DROP INDEX IF EXISTS idx_orchestration_env_snapshots_attempt;
DROP INDEX IF EXISTS idx_orchestration_env_snapshots_session;
DROP TABLE IF EXISTS orchestration_env_snapshots;

DROP INDEX IF EXISTS idx_orchestration_env_bindings_active_session_unique;
DROP INDEX IF EXISTS idx_orchestration_env_bindings_session;
DROP INDEX IF EXISTS idx_orchestration_env_bindings_task_attempt;
DROP INDEX IF EXISTS idx_orchestration_env_bindings_run;
DROP TABLE IF EXISTS orchestration_env_bindings;

DROP INDEX IF EXISTS idx_orchestration_env_lease_reservations_attempt;
DROP INDEX IF EXISTS idx_orchestration_env_lease_reservations_tenant;
DROP INDEX IF EXISTS idx_orchestration_env_lease_reservations_queue;
DROP TABLE IF EXISTS orchestration_env_lease_reservations;

DROP INDEX IF EXISTS idx_orchestration_env_sessions_attempt;
DROP INDEX IF EXISTS idx_orchestration_env_sessions_lease_expiry;
DROP INDEX IF EXISTS idx_orchestration_env_sessions_tenant_status;
DROP INDEX IF EXISTS idx_orchestration_env_sessions_resource_status;
DROP TABLE IF EXISTS orchestration_env_sessions;

DROP INDEX IF EXISTS idx_orchestration_env_resources_tenant_kind;
DROP TABLE IF EXISTS orchestration_env_resources;
