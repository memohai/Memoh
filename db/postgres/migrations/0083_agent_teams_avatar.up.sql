-- 0083_agent_teams_avatar
-- Add avatar_url column to agent_teams so teams can have a custom icon
-- alongside the auto-generated initial-based fallback.

ALTER TABLE agent_teams
  ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '';
