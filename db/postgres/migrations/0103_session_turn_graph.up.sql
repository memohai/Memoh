-- 0103_session_turn_graph
-- Add immutable turn chains and session-level fork pointers for session history.

ALTER TABLE IF EXISTS bot_history_messages
  DROP CONSTRAINT IF EXISTS fk_bot_history_messages_turn;
ALTER TABLE IF EXISTS bot_history_messages
  DROP CONSTRAINT IF EXISTS bot_history_messages_turn_id_fkey;
ALTER TABLE IF EXISTS bot_sessions
  DROP CONSTRAINT IF EXISTS fk_bot_sessions_default_head_turn;
ALTER TABLE IF EXISTS bot_sessions
  DROP CONSTRAINT IF EXISTS fk_bot_sessions_forked_from_turn;
ALTER TABLE IF EXISTS tool_approval_requests
  DROP CONSTRAINT IF EXISTS fk_tool_approval_persist_turn;
ALTER TABLE IF EXISTS tool_approval_requests
  DROP CONSTRAINT IF EXISTS tool_approval_requests_persist_turn_id_fkey;
ALTER TABLE IF EXISTS user_input_requests
  DROP CONSTRAINT IF EXISTS fk_user_input_persist_turn;
ALTER TABLE IF EXISTS user_input_requests
  DROP CONSTRAINT IF EXISTS user_input_requests_persist_turn_id_fkey;

DROP VIEW IF EXISTS bot_branch_visible_messages;

DROP INDEX IF EXISTS idx_bot_history_turns_request;
DROP INDEX IF EXISTS idx_bot_history_turns_assistant;
DROP INDEX IF EXISTS idx_bot_history_turns_bot_created;
DROP INDEX IF EXISTS idx_bot_history_turns_owner_session;
DROP INDEX IF EXISTS idx_bot_history_turns_parent;
DROP INDEX IF EXISTS idx_bot_history_turns_session_branch;
DROP INDEX IF EXISTS idx_bot_history_turns_branch_seq;
DROP INDEX IF EXISTS idx_bot_history_messages_turn;
DROP INDEX IF EXISTS idx_bot_history_messages_turn_seq_unique;
DROP INDEX IF EXISTS idx_bot_history_messages_branch;
DROP INDEX IF EXISTS idx_bot_history_messages_branch_seq;
DROP INDEX IF EXISTS idx_bot_session_turn_heads_head;
DROP INDEX IF EXISTS idx_bot_session_turn_heads_bot;
DROP INDEX IF EXISTS idx_bot_session_branches_parent;
DROP INDEX IF EXISTS idx_bot_session_branches_session;
DROP INDEX IF EXISTS idx_bot_session_branches_root;

CREATE TABLE IF NOT EXISTS bot_history_turns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  owner_session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL,
  parent_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  request_message_id UUID,
  final_assistant_message_id UUID,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Turn provenance: origin_kind/origin_turn_id record how the turn was
  -- created (message | retry | edit); request_group_id groups sibling turns
  -- carrying the same logical request (retry copies the source turn's group,
  -- send/edit start a new one). NULL request_group_id means the turn is its
  -- own group (read as COALESCE(request_group_id, id)).
  origin_kind TEXT,
  origin_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  request_group_id UUID
);

-- Guard for databases where an earlier iteration already created the table:
-- make sure the provenance columns exist either way.
ALTER TABLE bot_history_turns ADD COLUMN IF NOT EXISTS origin_kind TEXT;
ALTER TABLE bot_history_turns ADD COLUMN IF NOT EXISTS origin_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL;
ALTER TABLE bot_history_turns ADD COLUMN IF NOT EXISTS request_group_id UUID;

CREATE TABLE IF NOT EXISTS bot_session_turn_heads (
  session_id UUID NOT NULL,
  head_turn_id UUID NOT NULL,
  bot_id UUID NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (session_id, head_turn_id)
);

ALTER TABLE bot_sessions ADD COLUMN IF NOT EXISTS default_head_turn_id UUID;
ALTER TABLE bot_sessions ADD COLUMN IF NOT EXISTS forked_from_session_id UUID REFERENCES bot_sessions(id) ON DELETE SET NULL;
ALTER TABLE bot_sessions ADD COLUMN IF NOT EXISTS forked_from_turn_id UUID;

ALTER TABLE bot_history_messages ADD COLUMN IF NOT EXISTS turn_id UUID;
ALTER TABLE bot_history_messages ADD COLUMN IF NOT EXISTS turn_message_seq BIGINT;

ALTER TABLE tool_approval_requests ADD COLUMN IF NOT EXISTS persist_turn_id UUID;
ALTER TABLE user_input_requests ADD COLUMN IF NOT EXISTS persist_turn_id UUID;

DO $$
DECLARE
  has_legacy_turns BOOLEAN;
BEGIN
  SELECT EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'bot_history_turns'
      AND column_name = 'turn_seq'
  ) INTO has_legacy_turns;

  IF has_legacy_turns THEN
    ALTER TABLE IF EXISTS bot_session_branches
      DROP CONSTRAINT IF EXISTS fk_bot_session_branches_fork_from_turn;
    ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS bot_history_turns_session_id_fkey;
    ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS bot_history_turns_branch_id_fkey;
    ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS bot_history_turns_request_message_id_fkey;
    ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS bot_history_turns_final_assistant_message_id_fkey;
    ALTER TABLE bot_history_turns DROP CONSTRAINT IF EXISTS bot_history_turns_status_check;

    ALTER TABLE bot_history_turns ADD COLUMN IF NOT EXISTS bot_id UUID;
    ALTER TABLE bot_history_turns ADD COLUMN IF NOT EXISTS owner_session_id UUID;
    ALTER TABLE bot_history_turns ADD COLUMN IF NOT EXISTS parent_turn_id UUID;

    UPDATE bot_history_turns t
    SET bot_id = s.bot_id,
        owner_session_id = COALESCE(t.owner_session_id, t.session_id)
    FROM bot_sessions s
    WHERE t.session_id = s.id
      AND (t.bot_id IS NULL OR t.owner_session_id IS NULL);

    UPDATE bot_history_turns t
    SET parent_turn_id = previous.id
    FROM bot_history_turns previous
    WHERE previous.branch_id = t.branch_id
      AND previous.turn_seq = t.turn_seq - 1
      AND t.parent_turn_id IS NULL;

    UPDATE bot_history_turns t
    SET parent_turn_id = b.fork_from_turn_id
    FROM bot_session_branches b
    WHERE b.id = t.branch_id
      AND b.parent_branch_id IS NOT NULL
      AND t.parent_turn_id IS NULL;

    IF EXISTS (SELECT 1 FROM bot_history_turns WHERE bot_id IS NULL) THEN
      RAISE EXCEPTION 'legacy bot_history_turns contains rows without a resolvable bot_id';
    END IF;

    ALTER TABLE bot_history_turns ALTER COLUMN bot_id SET NOT NULL;

    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint
      WHERE conname = 'bot_history_turns_bot_id_fkey'
        AND conrelid = 'bot_history_turns'::regclass
    ) THEN
      ALTER TABLE bot_history_turns
        ADD CONSTRAINT bot_history_turns_bot_id_fkey
        FOREIGN KEY (bot_id) REFERENCES bots(id) ON DELETE CASCADE;
    END IF;
    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint
      WHERE conname = 'bot_history_turns_owner_session_id_fkey'
        AND conrelid = 'bot_history_turns'::regclass
    ) THEN
      ALTER TABLE bot_history_turns
        ADD CONSTRAINT bot_history_turns_owner_session_id_fkey
        FOREIGN KEY (owner_session_id) REFERENCES bot_sessions(id) ON DELETE SET NULL;
    END IF;
    IF NOT EXISTS (
      SELECT 1 FROM pg_constraint
      WHERE conname = 'bot_history_turns_parent_turn_id_fkey'
        AND conrelid = 'bot_history_turns'::regclass
    ) THEN
      ALTER TABLE bot_history_turns
        ADD CONSTRAINT bot_history_turns_parent_turn_id_fkey
        FOREIGN KEY (parent_turn_id) REFERENCES bot_history_turns(id) ON DELETE SET NULL;
    END IF;

    UPDATE bot_sessions s
    SET default_head_turn_id = branch_head.id
    FROM (
      SELECT DISTINCT ON (session_row.id)
        session_row.id AS session_id,
        t.id
      FROM bot_sessions session_row
      JOIN bot_history_turns t
        ON t.owner_session_id = session_row.id
      ORDER BY
        session_row.id,
        CASE
          WHEN session_row.active_branch_id IS NOT NULL
           AND t.branch_id = session_row.active_branch_id THEN 0
          ELSE 1
        END,
        t.turn_seq DESC,
        t.created_at DESC,
        t.id DESC
    ) branch_head
    WHERE s.id = branch_head.session_id
      AND s.default_head_turn_id IS NULL;

    INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id, created_at, updated_at)
    SELECT DISTINCT ON (t.branch_id)
      t.owner_session_id,
      t.id,
      t.bot_id,
      t.created_at,
      t.updated_at
    FROM bot_history_turns t
    WHERE t.branch_id IS NOT NULL
      AND t.owner_session_id IS NOT NULL
    ORDER BY t.branch_id, t.turn_seq DESC, t.created_at DESC, t.id DESC
    ON CONFLICT (session_id, head_turn_id) DO NOTHING;

    ALTER TABLE tool_approval_requests DROP COLUMN IF EXISTS persist_branch_id;
    ALTER TABLE user_input_requests DROP COLUMN IF EXISTS persist_branch_id;
    ALTER TABLE bot_history_messages DROP COLUMN IF EXISTS branch_seq;
    ALTER TABLE bot_history_messages DROP COLUMN IF EXISTS branch_id;
    ALTER TABLE bot_sessions DROP COLUMN IF EXISTS active_branch_id;
    DROP TABLE IF EXISTS bot_session_branches;
    ALTER TABLE bot_history_turns DROP COLUMN IF EXISTS completed_at;
    ALTER TABLE bot_history_turns DROP COLUMN IF EXISTS title;
    ALTER TABLE bot_history_turns DROP COLUMN IF EXISTS status;
    ALTER TABLE bot_history_turns DROP COLUMN IF EXISTS turn_seq;
    ALTER TABLE bot_history_turns DROP COLUMN IF EXISTS branch_id;
    ALTER TABLE bot_history_turns DROP COLUMN IF EXISTS session_id;
  END IF;
END $$;

WITH ordered AS (
  SELECT
    m.id,
    m.bot_id,
    m.session_id,
    m.role,
    m.created_at,
    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at, m.id) AS message_seq,
    SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END) OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
      ROWS UNBOUNDED PRECEDING
    ) AS user_group
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.turn_id IS NULL
),
grouped AS (
  SELECT
    *,
    CASE WHEN user_group = 0 THEN -message_seq ELSE user_group END AS turn_group
  FROM ordered
),
turn_seeds AS (
  SELECT
    gen_random_uuid() AS turn_id,
    aggregated.*,
    ROW_NUMBER() OVER (
      PARTITION BY session_id
      ORDER BY created_at, COALESCE(request_message_id, final_assistant_message_id, '00000000-0000-0000-0000-000000000000'::uuid), turn_group
    ) AS turn_pos
  FROM (
    SELECT
      bot_id,
      session_id,
      turn_group,
      MIN(created_at) AS created_at,
      (ARRAY_AGG(id ORDER BY created_at ASC, id ASC) FILTER (WHERE role = 'user'))[1] AS request_message_id,
      (ARRAY_AGG(id ORDER BY created_at DESC, id DESC) FILTER (WHERE role = 'assistant'))[1] AS final_assistant_message_id
    FROM grouped
    GROUP BY bot_id, session_id, turn_group
  ) aggregated
),
linked_seeds AS (
  SELECT
    current.*,
    previous.turn_id AS parent_turn_id
  FROM turn_seeds current
  LEFT JOIN turn_seeds previous
    ON previous.session_id = current.session_id
   AND previous.turn_pos = current.turn_pos - 1
),
inserted AS (
  INSERT INTO bot_history_turns (
    id,
    bot_id,
    owner_session_id,
    parent_turn_id,
    request_message_id,
    final_assistant_message_id,
    created_at,
    updated_at
  )
  SELECT
    turn_id,
    bot_id,
    session_id,
    parent_turn_id,
    request_message_id,
    final_assistant_message_id,
    created_at,
    created_at
  FROM linked_seeds
  RETURNING id
),
mapped AS (
  SELECT
    g.id AS message_id,
    s.turn_id,
    ROW_NUMBER() OVER (PARTITION BY g.session_id, g.turn_group ORDER BY g.created_at, g.id) AS turn_message_seq
  FROM grouped g
  JOIN turn_seeds s
    ON s.session_id = g.session_id
   AND s.turn_group = g.turn_group
)
UPDATE bot_history_messages m
SET turn_id = mapped.turn_id,
    turn_message_seq = mapped.turn_message_seq
FROM mapped
WHERE m.id = mapped.message_id;

WITH ordered AS (
  SELECT
    m.id,
    m.bot_id,
    m.session_id,
    m.turn_id,
    m.role,
    m.created_at,
    ROW_NUMBER() OVER (PARTITION BY m.session_id ORDER BY m.created_at, m.id) AS message_seq,
    SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END) OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
      ROWS UNBOUNDED PRECEDING
    ) AS user_group
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.turn_id IS NOT NULL
),
grouped AS (
  SELECT
    *,
    CASE WHEN user_group = 0 THEN -message_seq ELSE user_group END AS turn_group
  FROM ordered
),
turn_heads AS (
  SELECT
    session_id,
    turn_id,
    ROW_NUMBER() OVER (
      PARTITION BY session_id
      ORDER BY created_at, turn_group
    ) AS turn_pos
  FROM (
    SELECT
      session_id,
      turn_group,
      MIN(created_at) AS created_at,
      (ARRAY_AGG(turn_id ORDER BY created_at ASC, id ASC))[1] AS turn_id
    FROM grouped
    GROUP BY session_id, turn_group
  ) aggregated
),
heads AS (
  SELECT session_id, turn_id AS default_head_turn_id
  FROM (
    SELECT
      turn_heads.*,
      ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY turn_pos DESC) AS rn
    FROM turn_heads
  ) ranked
  WHERE rn = 1
)
UPDATE bot_sessions s
SET default_head_turn_id = heads.default_head_turn_id
FROM heads
WHERE s.id = heads.session_id
  AND s.default_head_turn_id IS NULL;

INSERT INTO bot_session_turn_heads (session_id, head_turn_id, bot_id)
SELECT s.id, s.default_head_turn_id, s.bot_id
FROM bot_sessions s
JOIN bot_history_turns t
  ON t.id = s.default_head_turn_id
 AND t.bot_id = s.bot_id
WHERE s.default_head_turn_id IS NOT NULL
ON CONFLICT (session_id, head_turn_id) DO NOTHING;

UPDATE bot_sessions s
SET default_head_turn_id = NULL,
    updated_at = now()
WHERE s.default_head_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = s.default_head_turn_id
      AND t.bot_id = s.bot_id
  );

UPDATE bot_sessions s
SET forked_from_turn_id = NULL,
    updated_at = now()
WHERE s.forked_from_turn_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = s.forked_from_turn_id
      AND t.bot_id = s.bot_id
  );

CREATE INDEX IF NOT EXISTS idx_bot_sessions_default_head_turn ON bot_sessions(default_head_turn_id) WHERE default_head_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_forked_from_session ON bot_sessions(forked_from_session_id) WHERE forked_from_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_forked_from_turn ON bot_sessions(forked_from_turn_id) WHERE forked_from_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_sessions_bot_active_updated ON bot_sessions(bot_id, updated_at DESC, id DESC) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_sessions_id_bot_unique ON bot_sessions(id, bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_session_turn_heads_head ON bot_session_turn_heads(head_turn_id);
CREATE INDEX IF NOT EXISTS idx_bot_session_turn_heads_bot ON bot_session_turn_heads(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_bot_created ON bot_history_turns(bot_id, created_at, id);
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_owner_session ON bot_history_turns(owner_session_id, created_at, id) WHERE owner_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_parent ON bot_history_turns(parent_turn_id) WHERE parent_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_request ON bot_history_turns(request_message_id) WHERE request_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_assistant ON bot_history_turns(final_assistant_message_id) WHERE final_assistant_message_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_turns_id_bot_unique ON bot_history_turns(id, bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn ON bot_history_messages(turn_id, turn_message_seq, created_at) WHERE turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_tool_approval_persist_turn ON tool_approval_requests(persist_turn_id) WHERE persist_turn_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_input_persist_turn ON user_input_requests(persist_turn_id) WHERE persist_turn_id IS NOT NULL;

ALTER TABLE tool_approval_requests DROP CONSTRAINT IF EXISTS tool_approval_tool_call_unique;
ALTER TABLE user_input_requests DROP CONSTRAINT IF EXISTS user_input_tool_call_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_turn_unique;
DROP INDEX IF EXISTS tool_approval_tool_call_legacy_unique;
DROP INDEX IF EXISTS user_input_tool_call_turn_unique;
DROP INDEX IF EXISTS user_input_tool_call_legacy_unique;

WITH ranked_tool_approvals AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id, persist_turn_id
      ORDER BY created_at DESC, id DESC
    ) AS row_num
  FROM tool_approval_requests
)
DELETE FROM tool_approval_requests request
USING ranked_tool_approvals ranked
WHERE request.id = ranked.id
  AND ranked.row_num > 1;

WITH ranked_user_inputs AS (
  SELECT
    id,
    ROW_NUMBER() OVER (
      PARTITION BY session_id, tool_call_id, persist_turn_id
      ORDER BY created_at DESC, id DESC
    ) AS row_num
  FROM user_input_requests
)
DELETE FROM user_input_requests request
USING ranked_user_inputs ranked
WHERE request.id = ranked.id
  AND ranked.row_num > 1;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_tool_approval_session_bot'
  ) THEN
    ALTER TABLE tool_approval_requests
      ADD CONSTRAINT fk_tool_approval_session_bot
      FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_user_input_session_bot'
  ) THEN
    ALTER TABLE user_input_requests
      ADD CONSTRAINT fk_user_input_session_bot
      FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_session_turn_heads_session_bot'
  ) THEN
    ALTER TABLE bot_session_turn_heads
      ADD CONSTRAINT fk_bot_session_turn_heads_session_bot
      FOREIGN KEY (session_id, bot_id) REFERENCES bot_sessions(id, bot_id) ON DELETE CASCADE;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_session_turn_heads_turn_bot'
  ) THEN
    ALTER TABLE bot_session_turn_heads
      ADD CONSTRAINT fk_bot_session_turn_heads_turn_bot
      FOREIGN KEY (head_turn_id, bot_id) REFERENCES bot_history_turns(id, bot_id) ON DELETE CASCADE;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_sessions_default_head_turn'
  ) THEN
    ALTER TABLE bot_sessions
      ADD CONSTRAINT fk_bot_sessions_default_head_turn
      FOREIGN KEY (default_head_turn_id, bot_id) REFERENCES bot_history_turns(id, bot_id)
      ON DELETE SET NULL (default_head_turn_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_sessions_forked_from_turn'
  ) THEN
    ALTER TABLE bot_sessions
      ADD CONSTRAINT fk_bot_sessions_forked_from_turn
      FOREIGN KEY (forked_from_turn_id, bot_id) REFERENCES bot_history_turns(id, bot_id)
      ON DELETE SET NULL (forked_from_turn_id);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_history_messages_turn'
  ) THEN
    ALTER TABLE bot_history_messages
      ADD CONSTRAINT fk_bot_history_messages_turn
      FOREIGN KEY (turn_id) REFERENCES bot_history_turns(id) ON DELETE SET NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_history_turns_request_message'
  ) THEN
    ALTER TABLE bot_history_turns
      ADD CONSTRAINT fk_bot_history_turns_request_message
      FOREIGN KEY (request_message_id) REFERENCES bot_history_messages(id) ON DELETE SET NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_bot_history_turns_final_assistant_message'
  ) THEN
    ALTER TABLE bot_history_turns
      ADD CONSTRAINT fk_bot_history_turns_final_assistant_message
      FOREIGN KEY (final_assistant_message_id) REFERENCES bot_history_messages(id) ON DELETE SET NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_tool_approval_persist_turn'
  ) THEN
    ALTER TABLE tool_approval_requests
      ADD CONSTRAINT fk_tool_approval_persist_turn
      FOREIGN KEY (persist_turn_id) REFERENCES bot_history_turns(id) ON DELETE SET NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_user_input_persist_turn'
  ) THEN
    ALTER TABLE user_input_requests
      ADD CONSTRAINT fk_user_input_persist_turn
      FOREIGN KEY (persist_turn_id) REFERENCES bot_history_turns(id) ON DELETE SET NULL;
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS tool_approval_tool_call_legacy_unique
  ON tool_approval_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS tool_approval_tool_call_turn_unique
  ON tool_approval_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_legacy_unique
  ON user_input_requests(session_id, tool_call_id)
  WHERE persist_turn_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS user_input_tool_call_turn_unique
  ON user_input_requests(session_id, tool_call_id, persist_turn_id)
  WHERE persist_turn_id IS NOT NULL;

CREATE OR REPLACE FUNCTION enforce_request_persist_turn_owner()
RETURNS trigger AS $$
BEGIN
  IF NEW.persist_turn_id IS NULL THEN
    RETURN NEW;
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM bot_history_turns t
    WHERE t.id = NEW.persist_turn_id
      AND t.bot_id = NEW.bot_id
      AND t.owner_session_id = NEW.session_id
  ) THEN
    RAISE EXCEPTION 'persist_turn_id must reference a turn from the same bot session';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tool_approval_persist_turn_owner_guard ON tool_approval_requests;
CREATE TRIGGER tool_approval_persist_turn_owner_guard
BEFORE INSERT OR UPDATE OF persist_turn_id, bot_id, session_id ON tool_approval_requests
FOR EACH ROW EXECUTE FUNCTION enforce_request_persist_turn_owner();

DROP TRIGGER IF EXISTS user_input_persist_turn_owner_guard ON user_input_requests;
CREATE TRIGGER user_input_persist_turn_owner_guard
BEFORE INSERT OR UPDATE OF persist_turn_id, bot_id, session_id ON user_input_requests
FOR EACH ROW EXECUTE FUNCTION enforce_request_persist_turn_owner();
