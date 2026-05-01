-- 0076_container_workspace_backend
-- Add explicit workspace backend tracking for container and local workspaces.

ALTER TABLE containers
  ADD COLUMN IF NOT EXISTS workspace_backend TEXT NOT NULL DEFAULT 'container';

UPDATE containers
SET workspace_backend = 'local'
WHERE workspace_backend = 'container'
  AND (container_id LIKE 'local-%' OR image = 'local');
