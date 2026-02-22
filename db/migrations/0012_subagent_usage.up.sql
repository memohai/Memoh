-- 0012_subagent_usage
-- Add usage JSONB column to subagents table for tracking cumulative token usage.

ALTER TABLE subagents ADD COLUMN IF NOT EXISTS usage JSONB NOT NULL DEFAULT '{}'::jsonb;

