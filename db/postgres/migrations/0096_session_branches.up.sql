-- 0096_session_branches
-- Add in-session branch paths for fork/edit-and-rerun chat history.

ALTER TABLE bot_sessions
  ADD COLUMN IF NOT EXISTS active_branch_id UUID;

CREATE TABLE IF NOT EXISTS bot_session_branches (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  session_id UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  parent_branch_id UUID REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  fork_from_message_id UUID,
  fork_from_seq BIGINT,
  title TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_session_branches_root
  ON bot_session_branches(session_id)
  WHERE parent_branch_id IS NULL AND fork_from_message_id IS NULL;
CREATE INDEX IF NOT EXISTS idx_bot_session_branches_session ON bot_session_branches(session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_bot_session_branches_parent ON bot_session_branches(parent_branch_id) WHERE parent_branch_id IS NOT NULL;

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS branch_id UUID REFERENCES bot_session_branches(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS branch_seq BIGINT;

INSERT INTO bot_session_branches (session_id, created_at, updated_at)
SELECT s.id, COALESCE(MIN(m.created_at), s.created_at), COALESCE(MAX(m.created_at), s.updated_at)
FROM bot_sessions s
LEFT JOIN bot_history_messages m ON m.session_id = s.id
WHERE NOT EXISTS (
  SELECT 1
  FROM bot_session_branches b
  WHERE b.session_id = s.id
    AND b.parent_branch_id IS NULL
    AND b.fork_from_message_id IS NULL
)
GROUP BY s.id, s.created_at, s.updated_at;

UPDATE bot_sessions s
SET active_branch_id = b.id
FROM bot_session_branches b
WHERE b.session_id = s.id
  AND b.parent_branch_id IS NULL
  AND b.fork_from_message_id IS NULL
  AND s.active_branch_id IS NULL;

WITH numbered AS (
  SELECT
    m.id,
    b.id AS branch_id,
    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at ASC, m.id ASC)::BIGINT AS branch_seq
  FROM bot_history_messages m
  JOIN bot_session_branches b ON b.session_id = m.session_id
  WHERE b.parent_branch_id IS NULL
    AND b.fork_from_message_id IS NULL
    AND m.session_id IS NOT NULL
    AND (m.branch_id IS NULL OR m.branch_seq IS NULL)
)
UPDATE bot_history_messages m
SET branch_id = numbered.branch_id,
    branch_seq = numbered.branch_seq
FROM numbered
WHERE m.id = numbered.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_branch_seq
  ON bot_history_messages(branch_id, branch_seq)
  WHERE branch_id IS NOT NULL AND branch_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_branch
  ON bot_history_messages(branch_id, branch_seq);
