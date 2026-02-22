-- 0014_reasoning
-- Add reasoning support flag to models and reasoning settings to bots.

ALTER TABLE models ADD COLUMN IF NOT EXISTS supports_reasoning BOOLEAN NOT NULL DEFAULT false;

ALTER TABLE bots ADD COLUMN IF NOT EXISTS reasoning_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE bots ADD COLUMN IF NOT EXISTS reasoning_effort TEXT NOT NULL DEFAULT 'medium';
ALTER TABLE bots ADD CONSTRAINT bots_reasoning_effort_check
  CHECK (reasoning_effort IN ('low', 'medium', 'high'));
