-- 0098_fix_branch_fork_turn_seq
-- Correct fork boundaries so a fork from an assistant reply includes that reply's turn.

UPDATE bot_session_branches b
SET fork_from_turn_id = t.id,
    fork_from_turn_seq = t.turn_seq
FROM bot_history_turns t
WHERE b.parent_branch_id = t.branch_id
  AND b.fork_from_message_id = t.final_assistant_message_id
  AND b.parent_branch_id IS NOT NULL
  AND b.fork_from_message_id IS NOT NULL
  AND (b.fork_from_turn_id IS NULL OR b.fork_from_turn_seq IS NULL);

UPDATE bot_session_branches b
SET fork_from_turn_seq = t.turn_seq
FROM bot_history_turns t
WHERE b.fork_from_turn_id = t.id
  AND b.fork_from_turn_id IS NOT NULL
  AND (b.fork_from_turn_seq IS NULL OR b.fork_from_turn_seq != t.turn_seq);
