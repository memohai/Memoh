-- Benchmark scenario: approval_tool_calls
-- Production query: db/postgres/queries/tool_approval.sql ListToolApprovalsBySessionToolCalls
-- Args: $1 bot_id, $2 session_id, $3 tool_call_ids, $4 turn_ids
SELECT tar.*
FROM tool_approval_requests tar
JOIN bot_sessions s ON s.id = tar.session_id
  AND s.bot_id = tar.bot_id
  AND s.deleted_at IS NULL
WHERE tar.bot_id = $1::uuid
  AND tar.session_id = $2::uuid
  AND tar.tool_call_id = ANY($3::text[])
  AND (
    tar.persist_turn_id IS NULL
    OR tar.persist_turn_id = ANY($4::uuid[])
  )
ORDER BY tar.created_at ASC, tar.short_id ASC;
