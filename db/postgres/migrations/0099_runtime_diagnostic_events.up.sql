-- 0099_runtime_diagnostic_events
-- Persist recent runtime diagnostic failures for Bot runtime health panels.

CREATE TABLE IF NOT EXISTS runtime_diagnostic_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  scope TEXT NOT NULL CHECK (scope IN ('workspace', 'container', 'display', 'acp')),
  agent_id TEXT NOT NULL DEFAULT '',
  session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  runtime_id TEXT NOT NULL DEFAULT '',
  phase TEXT NOT NULL,
  severity TEXT NOT NULL CHECK (severity IN ('info', 'warn', 'error')),
  code TEXT NOT NULL,
  message TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_runtime_diagnostic_events_bot_created
  ON runtime_diagnostic_events(bot_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_runtime_diagnostic_events_bot_scope_created
  ON runtime_diagnostic_events(bot_id, scope, created_at DESC);
