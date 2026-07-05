-- 0103_history_turns
-- Add linear history turns, hot message turn read model, and session turn position allocator.

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS turn_id UUID,
  ADD COLUMN IF NOT EXISTS turn_message_seq BIGINT,
  ADD COLUMN IF NOT EXISTS turn_position BIGINT,
  ADD COLUMN IF NOT EXISTS turn_visible BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS bot_history_turns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id UUID NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  position BIGINT NOT NULL,
  request_message_id UUID REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  assistant_message_id UUID REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  superseded_by_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  superseded_at TIMESTAMPTZ,
  superseded_reason TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (session_id, position)
);

CREATE INDEX IF NOT EXISTS idx_bot_history_turns_session_active
  ON bot_history_turns(session_id, position)
  WHERE superseded_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_request_message
  ON bot_history_turns(request_message_id)
  WHERE request_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_turns_assistant_message
  ON bot_history_turns(assistant_message_id)
  WHERE assistant_message_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_role_created
  ON bot_history_messages(session_id, role, created_at, id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn
  ON bot_history_messages(turn_id, turn_message_seq, created_at, id)
  WHERE turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;

WITH ordered AS (
  SELECT
    m.*,
    COUNT(*) FILTER (WHERE m.role = 'user') OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
      ROWS UNBOUNDED PRECEDING
    ) AS user_group
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.role IN ('user', 'assistant')
),
grouped AS (
  SELECT
    CASE
      WHEN role = 'user' THEN user_group
      WHEN user_group = 0 THEN -ROW_NUMBER() OVER (
        PARTITION BY session_id
        ORDER BY created_at, id
      )
      ELSE user_group
    END AS turn_group,
    *
  FROM ordered
),
turns AS (
  SELECT
    bot_id,
    session_id,
    turn_group,
    MIN(created_at) AS created_at,
    (ARRAY_AGG(id ORDER BY created_at, id) FILTER (WHERE role = 'user'))[1] AS request_message_id,
    (ARRAY_AGG(id ORDER BY created_at, id) FILTER (WHERE role = 'assistant'))[1] AS assistant_message_id
  FROM grouped
  GROUP BY bot_id, session_id, turn_group
),
positioned AS (
  SELECT
    *,
    ROW_NUMBER() OVER (
      PARTITION BY session_id
      ORDER BY created_at, turn_group
    ) AS position
  FROM turns
)
INSERT INTO bot_history_turns (
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id,
  created_at,
  updated_at
)
SELECT
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id,
  created_at,
  created_at
FROM positioned
ON CONFLICT (session_id, position) DO NOTHING;

UPDATE bot_history_messages m
SET turn_id = t.id,
    turn_message_seq = 1
FROM bot_history_turns t
WHERE m.id = t.request_message_id
  AND m.turn_id IS NULL;

UPDATE bot_history_messages m
SET turn_id = t.id,
    turn_message_seq = 2
FROM bot_history_turns t
WHERE m.id = t.assistant_message_id
  AND m.turn_id IS NULL;

WITH bounded_turns AS (
  SELECT
    t.id,
    t.session_id,
    assistant.created_at AS assistant_created_at,
    assistant.id AS assistant_id,
    LEAD(COALESCE(req.created_at, assistant.created_at)) OVER (
      PARTITION BY t.session_id
      ORDER BY t.position
    ) AS next_created_at,
    LEAD(COALESCE(req.id, assistant.id)) OVER (
      PARTITION BY t.session_id
      ORDER BY t.position
    ) AS next_message_id
  FROM bot_history_turns t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
tail_messages AS (
  SELECT
    m.id AS message_id,
    t.id AS turn_id,
    2 + ROW_NUMBER() OVER (
      PARTITION BY t.id
      ORDER BY m.created_at, m.id
    ) AS turn_message_seq
  FROM bounded_turns t
  JOIN bot_history_messages m
    ON m.session_id = t.session_id
   AND m.role IN ('assistant', 'tool')
  WHERE t.assistant_id IS NOT NULL
    AND m.id <> t.assistant_id
    AND m.turn_id IS NULL
    AND NOT EXISTS (
      SELECT 1
      FROM bot_history_turns anchored
      WHERE anchored.request_message_id = m.id
         OR anchored.assistant_message_id = m.id
    )
    AND (m.created_at, m.id) > (t.assistant_created_at, t.assistant_id)
    AND (
      t.next_created_at IS NULL
      OR (m.created_at, m.id) < (t.next_created_at, t.next_message_id)
    )
)
UPDATE bot_history_messages m
SET turn_id = tail.turn_id,
    turn_message_seq = tail.turn_message_seq
FROM tail_messages tail
WHERE m.id = tail.message_id;

UPDATE bot_history_messages m
SET turn_position = t.position,
    turn_visible = (t.superseded_at IS NULL)
FROM bot_history_turns t
WHERE m.turn_id = t.id;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_order
  ON bot_history_messages(session_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = true
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_source_order
  ON bot_history_messages(session_id, source_message_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = true
    AND source_message_id IS NOT NULL
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;

CREATE OR REPLACE VIEW bot_visible_history_messages AS
SELECT
  m.turn_id,
  m.turn_position,
  m.turn_message_seq,
  m.id,
  m.bot_id,
  m.session_id,
  m.sender_channel_identity_id,
  m.sender_account_user_id,
  m.source_message_id,
  m.source_reply_to_message_id,
  m.role,
  m.content,
  m.metadata,
  m.usage,
  m.compact_id,
  m.session_mode,
  m.runtime_type,
  m.model_id,
  m.event_id,
  m.display_text,
  m.created_at
FROM bot_history_messages m
WHERE m.turn_visible = true
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL;

ALTER TABLE bot_sessions
  ADD COLUMN IF NOT EXISTS next_turn_position BIGINT NOT NULL DEFAULT 1;

UPDATE bot_sessions s
SET next_turn_position = GREATEST(s.next_turn_position, turns.next_position)
FROM (
  SELECT session_id, MAX(position) + 1 AS next_position
  FROM bot_history_turns
  GROUP BY session_id
) turns
WHERE s.id = turns.session_id;
