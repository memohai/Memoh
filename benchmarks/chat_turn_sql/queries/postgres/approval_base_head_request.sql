-- Benchmark scenario: approval_base_head_request
-- Production query: db/postgres/queries/tool_approval.sql GetPendingToolApprovalByBaseHeadRequestID
WITH RECURSIVE visible_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions s
  JOIN bot_session_turn_heads h ON h.session_id = s.id
    AND h.bot_id = s.bot_id
    AND h.head_turn_id = $4::uuid
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE s.id = $3::uuid
    AND s.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN visible_turns vt ON vt.parent_turn_id = p.id
)
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.id = $1::uuid
  AND tar.bot_id = $2::uuid
  AND tar.session_id = $3::uuid
  AND tar.status = 'pending'
  AND (tar.persist_turn_id IS NULL OR tar.persist_turn_id IN (SELECT visible_turns.id FROM visible_turns));
