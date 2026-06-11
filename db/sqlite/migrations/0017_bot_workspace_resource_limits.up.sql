-- 0017_bot_workspace_resource_limits
-- Add desired per-bot workspace resource limits.
CREATE TABLE IF NOT EXISTS bot_workspace_resource_limits (
  bot_id TEXT PRIMARY KEY REFERENCES bots(id) ON DELETE CASCADE,
  cpu_millicores INTEGER NOT NULL DEFAULT 0,
  memory_bytes INTEGER NOT NULL DEFAULT 0,
  storage_bytes INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_workspace_resource_limits_cpu_check CHECK (cpu_millicores >= 0),
  CONSTRAINT bot_workspace_resource_limits_memory_check CHECK (memory_bytes >= 0),
  CONSTRAINT bot_workspace_resource_limits_storage_check CHECK (storage_bytes >= 0)
);
