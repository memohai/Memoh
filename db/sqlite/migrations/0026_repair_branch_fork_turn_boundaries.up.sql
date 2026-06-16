-- 0026_repair_branch_fork_turn_boundaries
-- Repair SQLite branch fork turn boundaries for installs that already ran 0023/0024.

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
  AND (
    fork_from_turn_id IS NULL
    OR fork_from_turn_seq IS NULL
    OR fork_from_turn_id != (
      SELECT t.id
      FROM bot_history_turns t
      WHERE t.branch_id = bot_session_branches.parent_branch_id
        AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
      ORDER BY t.turn_seq DESC, t.created_at DESC
      LIMIT 1
    )
    OR fork_from_turn_seq != (
      SELECT t.turn_seq
      FROM bot_history_turns t
      WHERE t.branch_id = bot_session_branches.parent_branch_id
        AND t.final_assistant_message_id = bot_session_branches.fork_from_message_id
      ORDER BY t.turn_seq DESC, t.created_at DESC
      LIMIT 1
    )
  );

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

UPDATE bot_session_branches
SET fork_from_turn_seq = NULL
WHERE fork_from_turn_id IS NULL
  AND fork_from_turn_seq IS NOT NULL
  AND parent_branch_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_turns_branch_seq
  ON bot_history_turns(branch_id, turn_seq);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_session_branch
  ON bot_history_turns(session_id, branch_id, turn_seq);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_request
  ON bot_history_turns(request_message_id) WHERE request_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_assistant
  ON bot_history_turns(final_assistant_message_id) WHERE final_assistant_message_id IS NOT NULL;
