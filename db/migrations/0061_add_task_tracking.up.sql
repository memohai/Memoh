-- 0061_add_task_tracking
-- Add exec_id and pid columns to tasks table for process tracking.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS exec_id VARCHAR(255) NULL;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS pid INTEGER NULL;
CREATE INDEX IF NOT EXISTS idx_tasks_exec_id ON tasks(exec_id);
CREATE INDEX IF NOT EXISTS idx_tasks_pid ON tasks(pid);
