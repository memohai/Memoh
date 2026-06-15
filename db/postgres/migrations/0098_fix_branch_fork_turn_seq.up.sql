-- 0098_fix_branch_fork_turn_seq
-- Correct fork boundaries so a fork from an assistant reply includes that reply's turn.

UPDATE bot_session_branches b
SET fork_from_turn_seq = t.turn_seq
FROM bot_history_turns t
WHERE b.fork_from_turn_id = t.id
  AND b.fork_from_turn_id IS NOT NULL
  AND (b.fork_from_turn_seq IS NULL OR b.fork_from_turn_seq != t.turn_seq);
