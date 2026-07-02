-- Benchmark scenario: graph_metadata
-- Production query: db/postgres/queries/messages.sql ListSessionTurnGraphNodeMetadata
WITH RECURSIVE graph_turns AS (
  SELECT t.id, t.parent_turn_id
  FROM bot_sessions bs
  JOIN bot_session_turn_heads h ON h.session_id = bs.id
    AND h.bot_id = bs.bot_id
  JOIN bot_history_turns t ON t.id = h.head_turn_id
  WHERE bs.id = $1::uuid
    AND bs.deleted_at IS NULL
  UNION
  SELECT p.id, p.parent_turn_id
  FROM bot_history_turns p
  JOIN graph_turns gt ON gt.parent_turn_id = p.id
),
request_assets AS (
  SELECT
    a.message_id,
    string_agg(
      concat_ws(
        ':',
        COALESCE(a.content_hash, ''),
        COALESCE(a.name, ''),
        COALESCE(a.role, ''),
        COALESCE(a.ordinal::text, '')
      ),
      '|'
      ORDER BY a.content_hash, a.name, a.role, a.ordinal, a.id
    ) AS request_asset_key
  FROM graph_turns gt
  JOIN bot_history_turns t ON t.id = gt.id
  JOIN bot_history_message_assets a ON a.message_id = t.request_message_id
  GROUP BY a.message_id
)
SELECT
  gt.id AS turn_id,
  gt.parent_turn_id,
  COALESCE(rm.created_at, t.created_at)::timestamptz AS node_created_at,
  COALESCE(rm.content, 'null'::jsonb) AS request_content,
  COALESCE(rm.display_text, '')::text AS request_display_text,
  COALESCE(ra.request_asset_key, '')::text AS request_asset_key,
  (t.request_message_id IS NOT NULL)::boolean AS has_user,
  (t.final_assistant_message_id IS NOT NULL)::boolean AS has_assistant
FROM graph_turns gt
JOIN bot_history_turns t ON t.id = gt.id
LEFT JOIN bot_history_messages rm ON rm.id = t.request_message_id
LEFT JOIN request_assets ra ON ra.message_id = t.request_message_id
ORDER BY COALESCE(rm.created_at, t.created_at)::timestamptz ASC, gt.id ASC;
