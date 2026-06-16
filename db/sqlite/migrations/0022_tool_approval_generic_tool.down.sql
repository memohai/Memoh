-- 0022_tool_approval_generic_tool (down)
-- Remove the generic tool approval operation.

CREATE TABLE IF NOT EXISTS tool_approval_requests_old (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  route_id TEXT REFERENCES bot_channel_routes(id) ON DELETE SET NULL,
  channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  tool_call_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  operation TEXT NOT NULL,
  tool_input TEXT NOT NULL,
  short_id INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  decision_reason TEXT NOT NULL DEFAULT '',
  requested_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  decided_by_channel_identity_id TEXT REFERENCES channel_identities(id) ON DELETE SET NULL,
  requested_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  prompt_external_message_id TEXT NOT NULL DEFAULT '',
  source_platform TEXT NOT NULL DEFAULT '',
  reply_target TEXT NOT NULL DEFAULT '',
  conversation_type TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  decided_at TEXT,
  CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec')),
  CONSTRAINT tool_approval_status_check CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'cancelled')),
  CONSTRAINT tool_approval_short_id_unique UNIQUE (session_id, short_id),
  CONSTRAINT tool_approval_tool_call_unique UNIQUE (session_id, tool_call_id)
);

INSERT INTO tool_approval_requests_old (
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id, tool_name, operation, tool_input, short_id, status,
  decision_reason, requested_by_channel_identity_id, decided_by_channel_identity_id,
  requested_message_id, prompt_message_id, prompt_external_message_id,
  source_platform, reply_target, conversation_type, created_at, decided_at
)
SELECT
  id, bot_id, session_id, route_id, channel_identity_id,
  tool_call_id, tool_name,
  CASE WHEN operation = 'tool' THEN 'exec' ELSE operation END,
  tool_input, short_id, status,
  decision_reason, requested_by_channel_identity_id, decided_by_channel_identity_id,
  requested_message_id, prompt_message_id, prompt_external_message_id,
  source_platform, reply_target, conversation_type, created_at, decided_at
FROM tool_approval_requests;

DROP TABLE tool_approval_requests;
ALTER TABLE tool_approval_requests_old RENAME TO tool_approval_requests;

CREATE INDEX IF NOT EXISTS idx_tool_approvals_bot_status_created
  ON tool_approval_requests(bot_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_session_status_created
  ON tool_approval_requests(session_id, status, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_approvals_prompt_external
  ON tool_approval_requests(prompt_external_message_id)
  WHERE prompt_external_message_id <> '';
