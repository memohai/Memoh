-- 0028_repair_pending_branch_turn_context
-- Repair pending rows from the earlier SQLite 0026 schema rewrite draft.

DROP TABLE IF EXISTS tool_approval_requests_0027_repair;
CREATE TEMP TABLE tool_approval_requests_0027_repair AS
SELECT
  id,
  COALESCE(persist_branch_id, '') AS old_prompt_external_message_id,
  COALESCE(persist_turn_id, '') AS old_source_platform,
  COALESCE(prompt_external_message_id, '') AS old_reply_target,
  COALESCE(source_platform, '') AS old_conversation_type,
  COALESCE(reply_target, '') AS old_created_at,
  COALESCE(conversation_type, '') AS old_decided_at
FROM tool_approval_requests
WHERE persist_branch_id IS NOT NULL
  AND (
    NOT EXISTS (
      SELECT 1
      FROM bot_session_branches b
      WHERE b.id = tool_approval_requests.persist_branch_id
        AND b.session_id = tool_approval_requests.session_id
    )
    OR (
      persist_turn_id IS NOT NULL
      AND persist_turn_id NOT LIKE '________-____-____-____-____________'
      AND (
        reply_target LIKE '____-__-__ __:__:__'
        OR reply_target LIKE '____-__-__T__:__:__%'
      )
    )
  );

UPDATE tool_approval_requests
SET prompt_external_message_id = (SELECT old_prompt_external_message_id FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id),
    source_platform = (SELECT old_source_platform FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id),
    reply_target = (SELECT old_reply_target FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id),
    conversation_type = (SELECT old_conversation_type FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id),
    created_at = CASE
      WHEN (SELECT old_created_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id) LIKE '____-__-__ __:__:__' THEN (SELECT old_created_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id)
      WHEN (SELECT old_created_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id) LIKE '____-__-__T__:__:__%' THEN (SELECT old_created_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id)
      ELSE created_at
    END,
    decided_at = CASE
      WHEN (SELECT old_decided_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id) LIKE '____-__-__ __:__:__' THEN (SELECT old_decided_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id)
      WHEN (SELECT old_decided_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id) LIKE '____-__-__T__:__:__%' THEN (SELECT old_decided_at FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id)
      ELSE decided_at
    END,
    persist_branch_id = NULL,
    persist_turn_id = NULL
WHERE EXISTS (
  SELECT 1 FROM tool_approval_requests_0027_repair r WHERE r.id = tool_approval_requests.id
);

DROP TABLE tool_approval_requests_0027_repair;

DROP TABLE IF EXISTS user_input_requests_0027_repair;
CREATE TEMP TABLE user_input_requests_0027_repair AS
SELECT
  id,
  COALESCE(persist_branch_id, '') AS old_prompt_external_message_id,
  COALESCE(persist_turn_id, '') AS old_source_platform,
  COALESCE(prompt_external_message_id, '') AS old_reply_target,
  COALESCE(source_platform, '') AS old_conversation_type,
  reply_target AS old_expires_at,
  conversation_type AS old_created_at,
  expires_at AS old_responded_at,
  created_at AS old_canceled_at,
  responded_at AS old_updated_at
FROM user_input_requests
WHERE persist_branch_id IS NOT NULL
  AND (
    NOT EXISTS (
      SELECT 1
      FROM bot_session_branches b
      WHERE b.id = user_input_requests.persist_branch_id
        AND b.session_id = user_input_requests.session_id
    )
    OR (
      persist_turn_id IS NOT NULL
      AND persist_turn_id NOT LIKE '________-____-____-____-____________'
      AND (
        conversation_type LIKE '____-__-__ __:__:__'
        OR conversation_type LIKE '____-__-__T__:__:__%'
      )
    )
  );

UPDATE user_input_requests
SET prompt_external_message_id = (SELECT old_prompt_external_message_id FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id),
    source_platform = (SELECT old_source_platform FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id),
    reply_target = (SELECT old_reply_target FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id),
    conversation_type = (SELECT old_conversation_type FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id),
    expires_at = (SELECT old_expires_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id),
    created_at = CASE
      WHEN (SELECT old_created_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__ __:__:__' THEN (SELECT old_created_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      WHEN (SELECT old_created_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__T__:__:__%' THEN (SELECT old_created_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      ELSE created_at
    END,
    responded_at = CASE
      WHEN (SELECT old_responded_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__ __:__:__' THEN (SELECT old_responded_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      WHEN (SELECT old_responded_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__T__:__:__%' THEN (SELECT old_responded_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      ELSE responded_at
    END,
    canceled_at = CASE
      WHEN (SELECT old_canceled_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__ __:__:__' THEN (SELECT old_canceled_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      WHEN (SELECT old_canceled_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__T__:__:__%' THEN (SELECT old_canceled_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      ELSE canceled_at
    END,
    updated_at = CASE
      WHEN (SELECT old_updated_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__ __:__:__' THEN (SELECT old_updated_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      WHEN (SELECT old_updated_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id) LIKE '____-__-__T__:__:__%' THEN (SELECT old_updated_at FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id)
      ELSE updated_at
    END,
    persist_branch_id = NULL,
    persist_turn_id = NULL
WHERE EXISTS (
  SELECT 1 FROM user_input_requests_0027_repair r WHERE r.id = user_input_requests.id
);

DROP TABLE user_input_requests_0027_repair;

UPDATE tool_approval_requests
SET persist_turn_id = NULL
WHERE persist_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = tool_approval_requests.persist_turn_id
      AND t.branch_id = tool_approval_requests.persist_branch_id
      AND t.session_id = tool_approval_requests.session_id
  );

UPDATE user_input_requests
SET persist_turn_id = NULL
WHERE persist_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = user_input_requests.persist_turn_id
      AND t.branch_id = user_input_requests.persist_branch_id
      AND t.session_id = user_input_requests.session_id
  );
