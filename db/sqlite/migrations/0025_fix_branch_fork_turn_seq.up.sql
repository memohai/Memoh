-- 0025_fix_branch_fork_turn_seq
-- Correct fork boundaries so a fork from an assistant reply includes that reply's turn.

UPDATE bot_session_branches
SET fork_from_turn_id = (
      SELECT t.id
      FROM bot_history_turns t
      WHERE t.branch_id = bot_session_branches.parent_branch_id
        AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
      ORDER BY t.turn_seq DESC, t.created_at DESC
      LIMIT 1
    ),
    fork_from_turn_seq = (
      SELECT t.turn_seq
      FROM bot_history_turns t
      WHERE t.branch_id = bot_session_branches.parent_branch_id
        AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
      ORDER BY t.turn_seq DESC, t.created_at DESC
      LIMIT 1
    )
WHERE fork_from_message_id IS NOT NULL
  AND parent_branch_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.branch_id = bot_session_branches.parent_branch_id
      AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
  )
  AND (fork_from_turn_id IS NULL OR fork_from_turn_seq IS NULL);

UPDATE bot_session_branches
SET fork_from_turn_seq = (
  SELECT t.turn_seq
  FROM bot_history_turns t
  WHERE t.id = bot_session_branches.fork_from_turn_id
  LIMIT 1
)
WHERE fork_from_turn_id IS NOT NULL
  AND EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = bot_session_branches.fork_from_turn_id
      AND (
        bot_session_branches.fork_from_turn_seq IS NULL
        OR bot_session_branches.fork_from_turn_seq != t.turn_seq
      )
  );
