-- Add task tracking for exec_id and PID
ALTER TABLE tasks ADD COLUMN exec_id VARCHAR(255) NULL;
ALTER TABLE tasks ADD COLUMN pid INTEGER NULL;
CREATE INDEX idx_tasks_exec_id ON tasks(exec_id);
CREATE INDEX idx_tasks_pid ON tasks(pid);