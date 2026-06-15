-- 0097_history_turns
-- Add first-class history turns for session branch fork semantics.

ALTER TABLE bot_session_branches
  ADD COLUMN IF NOT EXISTS fork_from_turn_id UUID,
  ADD COLUMN IF NOT EXISTS fork_from_turn_seq BIGINT;

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS turn_id UUID,
  ADD COLUMN IF NOT EXISTS turn_message_seq BIGINT;

CREATE TABLE IF NOT EXISTS bot_history_turns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  branch_id UUID NOT NULL REFERENCES bot_session_branches(id) ON DELETE CASCADE,
  turn_seq BIGINT NOT NULL,
  request_message_id UUID REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  final_assistant_message_id UUID REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'completed', 'failed', 'canceled')),
  title TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  completed_at TIMESTAMPTZ
);

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_history_messages_turn'
  ) THEN
    ALTER TABLE bot_history_messages
      ADD CONSTRAINT fk_bot_history_messages_turn
      FOREIGN KEY (turn_id) REFERENCES bot_history_turns(id) ON DELETE SET NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_session_branches_fork_from_turn'
  ) THEN
    ALTER TABLE bot_session_branches
      ADD CONSTRAINT fk_bot_session_branches_fork_from_turn
      FOREIGN KEY (fork_from_turn_id) REFERENCES bot_history_turns(id) ON DELETE SET NULL;
  END IF;
END $$;

WITH user_turns AS (
  SELECT
    m.id AS user_id,
    m.session_id,
    m.branch_id,
    m.branch_seq,
    m.created_at AS user_created_at,
    (
      SELECT MIN(next_user.branch_seq)
      FROM bot_history_messages next_user
      WHERE next_user.branch_id = m.branch_id
        AND next_user.role = 'user'
        AND next_user.branch_seq > m.branch_seq
    ) AS next_user_seq
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.branch_id IS NOT NULL
    AND m.role = 'user'
),
complete_turns AS (
  SELECT
    ut.*,
    (
      SELECT a.id
      FROM bot_history_messages a
      WHERE a.branch_id = ut.branch_id
        AND a.role = 'assistant'
        AND a.branch_seq > ut.branch_seq
        AND (ut.next_user_seq IS NULL OR a.branch_seq < ut.next_user_seq)
      ORDER BY a.branch_seq DESC, a.created_at DESC
      LIMIT 1
    ) AS assistant_id,
    (
      SELECT a.created_at
      FROM bot_history_messages a
      WHERE a.branch_id = ut.branch_id
        AND a.role = 'assistant'
        AND a.branch_seq > ut.branch_seq
        AND (ut.next_user_seq IS NULL OR a.branch_seq < ut.next_user_seq)
      ORDER BY a.branch_seq DESC, a.created_at DESC
      LIMIT 1
    ) AS assistant_created_at
  FROM user_turns ut
),
numbered_turns AS (
  SELECT
    gen_random_uuid() AS turn_id,
    ct.*,
    ROW_NUMBER() OVER (PARTITION BY ct.branch_id ORDER BY ct.branch_seq ASC)::BIGINT AS turn_seq
  FROM complete_turns ct
  WHERE ct.assistant_id IS NOT NULL
)
INSERT INTO bot_history_turns (
  id, session_id, branch_id, turn_seq, request_message_id, final_assistant_message_id,
  status, created_at, updated_at, completed_at
)
SELECT
  nt.turn_id,
  nt.session_id,
  nt.branch_id,
  nt.turn_seq,
  nt.user_id,
  nt.assistant_id,
  'completed',
  nt.user_created_at,
  COALESCE(nt.assistant_created_at, nt.user_created_at),
  nt.assistant_created_at
FROM numbered_turns nt
WHERE NOT EXISTS (
  SELECT 1 FROM bot_history_turns existing WHERE existing.request_message_id = nt.user_id
);

WITH turn_ranges AS (
  SELECT
    t.id AS turn_id,
    t.branch_id,
    req.branch_seq AS start_seq,
    COALESCE((
      SELECT MIN(next_req.branch_seq)
      FROM bot_history_messages next_req
      WHERE next_req.branch_id = t.branch_id
        AND next_req.role = 'user'
        AND next_req.branch_seq > req.branch_seq
    ), 9223372036854775807) AS end_seq
  FROM bot_history_turns t
  JOIN bot_history_messages req ON req.id = t.request_message_id
)
UPDATE bot_history_messages m
SET turn_id = tr.turn_id,
    turn_message_seq = m.branch_seq - tr.start_seq + 1
FROM turn_ranges tr
WHERE m.branch_id = tr.branch_id
  AND m.branch_seq >= tr.start_seq
  AND m.branch_seq < tr.end_seq
  AND m.turn_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_turns_branch_seq
  ON bot_history_turns(branch_id, turn_seq);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_session_branch
  ON bot_history_turns(session_id, branch_id, turn_seq);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_request
  ON bot_history_turns(request_message_id) WHERE request_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_assistant
  ON bot_history_turns(final_assistant_message_id) WHERE final_assistant_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL;
