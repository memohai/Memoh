-- 0012_subagent_usage (rollback)
-- Remove usage column from subagents table.

ALTER TABLE subagents DROP COLUMN IF EXISTS usage;

