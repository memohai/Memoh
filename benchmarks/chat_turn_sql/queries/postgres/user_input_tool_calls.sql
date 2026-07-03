-- Benchmark scenario: user_input_tool_calls
-- Production query: db/postgres/queries/user_input.sql ListUserInputsBySessionToolCalls
-- Args: $1 bot_id, $2 session_id, $3 tool_call_ids, $4 turn_ids
SELECT uir.*
FROM user_input_requests uir
JOIN bot_sessions s ON s.id = uir.session_id
  AND s.bot_id = uir.bot_id
  AND s.deleted_at IS NULL
WHERE uir.bot_id = $1::uuid
  AND uir.session_id = $2::uuid
  AND uir.tool_call_id = ANY($3::text[])
  AND (uir.expires_at IS NULL OR uir.expires_at > now())
  AND (
    uir.persist_turn_id IS NULL
    OR uir.persist_turn_id = ANY($4::uuid[])
  )
ORDER BY uir.created_at ASC, uir.short_id ASC;
