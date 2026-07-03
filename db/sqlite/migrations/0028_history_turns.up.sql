-- 0028_history_turns
-- Add linear history turns for stable retry/edit replacement.

ALTER TABLE bot_history_messages ADD COLUMN turn_id TEXT;
ALTER TABLE bot_history_messages ADD COLUMN turn_message_seq INTEGER;

CREATE TABLE IF NOT EXISTS bot_history_turns (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  session_id TEXT NOT NULL REFERENCES bot_sessions(id) ON DELETE CASCADE,
  position INTEGER NOT NULL,
  request_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  assistant_message_id TEXT REFERENCES bot_history_messages(id) ON DELETE SET NULL,
  superseded_by_turn_id TEXT REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  superseded_at TEXT,
  superseded_reason TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
    SUM(CASE WHEN m.role = 'user' THEN 1 ELSE 0 END) OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
      ROWS UNBOUNDED PRECEDING
    ) AS user_group,
    ROW_NUMBER() OVER (
      PARTITION BY m.session_id
      ORDER BY m.created_at, m.id
    ) AS message_pos
  FROM bot_history_messages m
  WHERE m.session_id IS NOT NULL
    AND m.role IN ('user', 'assistant')
),
grouped AS (
  SELECT
    CASE
      WHEN role = 'user' THEN user_group
      WHEN user_group = 0 THEN -message_pos
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
    substr(MIN(CASE WHEN role = 'user' THEN printf('%020d', message_pos) || id END), 21) AS request_message_id,
    substr(MIN(CASE WHEN role = 'assistant' THEN printf('%020d', message_pos) || id END), 21) AS assistant_message_id
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
INSERT OR IGNORE INTO bot_history_turns (
  id,
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id,
  created_at,
  updated_at
)
SELECT
  lower(hex(randomblob(4))) || '-' ||
  lower(hex(randomblob(2))) || '-' ||
  '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
  substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
  lower(hex(randomblob(6))),
  bot_id,
  session_id,
  position,
  request_message_id,
  assistant_message_id,
  created_at,
  created_at
FROM positioned;

UPDATE bot_history_messages
SET turn_id = (
      SELECT t.id FROM bot_history_turns t WHERE t.request_message_id = bot_history_messages.id
    ),
    turn_message_seq = 1
WHERE turn_id IS NULL
  AND EXISTS (
    SELECT 1 FROM bot_history_turns t WHERE t.request_message_id = bot_history_messages.id
  );

UPDATE bot_history_messages
SET turn_id = (
      SELECT t.id FROM bot_history_turns t WHERE t.assistant_message_id = bot_history_messages.id
    ),
    turn_message_seq = 2
WHERE turn_id IS NULL
  AND EXISTS (
    SELECT 1 FROM bot_history_turns t WHERE t.assistant_message_id = bot_history_messages.id
  );

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
    AND (
      m.created_at > t.assistant_created_at
      OR (m.created_at = t.assistant_created_at AND m.id > t.assistant_id)
    )
    AND (
      t.next_created_at IS NULL
      OR m.created_at < t.next_created_at
      OR (m.created_at = t.next_created_at AND m.id < t.next_message_id)
    )
)
UPDATE bot_history_messages
SET turn_id = (
      SELECT tail.turn_id FROM tail_messages tail WHERE tail.message_id = bot_history_messages.id
    ),
    turn_message_seq = (
      SELECT tail.turn_message_seq FROM tail_messages tail WHERE tail.message_id = bot_history_messages.id
    )
WHERE id IN (SELECT message_id FROM tail_messages);

DROP VIEW IF EXISTS bot_visible_history_messages;

CREATE VIEW IF NOT EXISTS bot_visible_history_messages AS
SELECT
  t.id AS turn_id,
  t.position AS turn_position,
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
JOIN bot_history_turns t ON t.id = m.turn_id
WHERE t.superseded_at IS NULL;
