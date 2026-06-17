-- 0099_branch_visible_turn_boundary
-- Include the full fork boundary turn in visible branch paths.

CREATE OR REPLACE VIEW bot_branch_visible_messages AS
WITH RECURSIVE branch_path AS (
  SELECT
    b.id AS target_branch_id,
    b.id AS branch_id,
    b.parent_branch_id,
    NULL::BIGINT AS max_turn_seq,
    NULL::BIGINT AS max_branch_seq,
    0 AS depth
  FROM bot_session_branches b
  UNION ALL
  SELECT
    bp.target_branch_id,
    parent.id AS branch_id,
    parent.parent_branch_id,
    COALESCE(child.fork_from_turn_seq, boundary_turn.turn_seq) AS max_turn_seq,
    child.fork_from_seq AS max_branch_seq,
    bp.depth + 1 AS depth
  FROM branch_path bp
  JOIN bot_session_branches child ON child.id = bp.branch_id
  JOIN bot_session_branches parent ON parent.id = child.parent_branch_id
  LEFT JOIN bot_history_turns boundary_turn ON boundary_turn.id = child.fork_from_turn_id AND boundary_turn.branch_id = parent.id
)
SELECT
  bp.target_branch_id AS branch_id,
  m.id AS message_id,
  bp.depth
FROM branch_path bp
JOIN bot_history_messages m ON m.branch_id = bp.branch_id
LEFT JOIN bot_history_turns t ON t.id = m.turn_id
WHERE bp.depth = 0
  OR (
    bp.max_turn_seq IS NOT NULL
    AND t.turn_seq <= bp.max_turn_seq
  )
  OR (
    bp.max_turn_seq IS NULL
    AND bp.max_branch_seq IS NOT NULL
    AND m.branch_seq <= bp.max_branch_seq
  )
UNION ALL
SELECT
  b.id AS branch_id,
  m.id AS message_id,
  2147483647 AS depth
FROM bot_session_branches b
JOIN bot_history_messages m ON m.session_id = b.session_id AND m.branch_id IS NULL;
