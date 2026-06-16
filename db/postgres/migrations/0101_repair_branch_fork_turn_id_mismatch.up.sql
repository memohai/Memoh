-- 0101_repair_branch_fork_turn_id_mismatch
-- Re-run fork turn boundary repair for installs that already applied earlier branch migrations.

UPDATE bot_session_branches b
SET fork_from_turn_id = t.id,
    fork_from_turn_seq = t.turn_seq
FROM bot_history_turns t
WHERE b.parent_branch_id = t.branch_id
  AND b.fork_from_message_id = t.final_assistant_message_id
  AND b.parent_branch_id IS NOT NULL
  AND b.fork_from_message_id IS NOT NULL
  AND (
    b.fork_from_turn_id IS NULL
    OR b.fork_from_turn_seq IS NULL
    OR b.fork_from_turn_id != t.id
    OR b.fork_from_turn_seq != t.turn_seq
  );

UPDATE bot_session_branches b
SET fork_from_turn_seq = t.turn_seq
FROM bot_history_turns t
WHERE b.fork_from_turn_id = t.id
  AND b.fork_from_turn_id IS NOT NULL
  AND (b.fork_from_turn_seq IS NULL OR b.fork_from_turn_seq != t.turn_seq);

UPDATE bot_session_branches
SET fork_from_turn_seq = NULL
WHERE fork_from_turn_id IS NULL
  AND fork_from_turn_seq IS NOT NULL
  AND parent_branch_id IS NOT NULL;
