-- 0028_message_turn_read_model
-- Add single-table history turn read/lifecycle model and session turn position allocator.

BEGIN;

ALTER TABLE bot_sessions
  ADD COLUMN next_turn_position INTEGER NOT NULL DEFAULT 1;

ALTER TABLE bot_history_messages ADD COLUMN turn_id TEXT;
ALTER TABLE bot_history_messages ADD COLUMN turn_message_seq INTEGER;
ALTER TABLE bot_history_messages ADD COLUMN turn_position INTEGER;
ALTER TABLE bot_history_messages ADD COLUMN turn_visible INTEGER NOT NULL DEFAULT 0;
ALTER TABLE bot_history_messages ADD COLUMN turn_superseded_by_turn_id TEXT;
ALTER TABLE bot_history_messages ADD COLUMN turn_superseded_at TEXT;
ALTER TABLE bot_history_messages ADD COLUMN turn_superseded_reason TEXT;

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_session_role_created
  ON bot_history_messages(session_id, role, created_at, id);
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_turn
  ON bot_history_messages(turn_id, turn_message_seq, created_at, id)
  WHERE turn_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_bot_history_messages_turn_seq_unique
  ON bot_history_messages(turn_id, turn_message_seq)
  WHERE turn_id IS NOT NULL AND turn_message_seq IS NOT NULL;

DROP TABLE IF EXISTS _memoh_history_turn_backfill;
CREATE TEMP TABLE _memoh_history_turn_backfill AS
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
    lower(hex(randomblob(4))) || '-' ||
    lower(hex(randomblob(2))) || '-' ||
    '4' || substr(lower(hex(randomblob(2))), 2) || '-' ||
    substr('89ab', abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))), 2) || '-' ||
    lower(hex(randomblob(6))) AS turn_id,
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
SELECT * FROM positioned;

UPDATE bot_history_messages
SET turn_id = (
      SELECT t.turn_id FROM _memoh_history_turn_backfill t WHERE t.request_message_id = bot_history_messages.id
    ),
    turn_message_seq = 1,
    turn_position = (
      SELECT t.position FROM _memoh_history_turn_backfill t WHERE t.request_message_id = bot_history_messages.id
    ),
    turn_visible = 1
WHERE turn_id IS NULL
  AND EXISTS (
    SELECT 1 FROM _memoh_history_turn_backfill t WHERE t.request_message_id = bot_history_messages.id
  );

UPDATE bot_history_messages
SET turn_id = (
      SELECT t.turn_id FROM _memoh_history_turn_backfill t WHERE t.assistant_message_id = bot_history_messages.id
    ),
    turn_message_seq = 2,
    turn_position = (
      SELECT t.position FROM _memoh_history_turn_backfill t WHERE t.assistant_message_id = bot_history_messages.id
    ),
    turn_visible = 1
WHERE turn_id IS NULL
  AND EXISTS (
    SELECT 1 FROM _memoh_history_turn_backfill t WHERE t.assistant_message_id = bot_history_messages.id
  );

WITH bounded_turns AS (
  SELECT
    t.turn_id,
    t.session_id,
    t.position,
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
  FROM _memoh_history_turn_backfill t
  LEFT JOIN bot_history_messages req ON req.id = t.request_message_id
  LEFT JOIN bot_history_messages assistant ON assistant.id = t.assistant_message_id
),
tail_messages AS (
  SELECT
    m.id AS message_id,
    t.turn_id,
    t.position,
    2 + ROW_NUMBER() OVER (
      PARTITION BY t.turn_id
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
      FROM _memoh_history_turn_backfill anchored
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
    ),
    turn_position = (
      SELECT tail.position FROM tail_messages tail WHERE tail.message_id = bot_history_messages.id
    ),
    turn_visible = 1
WHERE id IN (SELECT message_id FROM tail_messages);

CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_order
  ON bot_history_messages(session_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = 1
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_bot_history_messages_visible_session_source_order
  ON bot_history_messages(session_id, source_message_id, turn_position DESC, turn_message_seq DESC, created_at DESC, id DESC)
  WHERE turn_visible = 1
    AND source_message_id IS NOT NULL
    AND turn_id IS NOT NULL
    AND turn_position IS NOT NULL
    AND turn_message_seq IS NOT NULL;

DROP VIEW IF EXISTS bot_visible_history_messages;
CREATE VIEW IF NOT EXISTS bot_visible_history_messages AS
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
WHERE m.turn_visible = 1
  AND m.turn_id IS NOT NULL
  AND m.turn_position IS NOT NULL
  AND m.turn_message_seq IS NOT NULL;

UPDATE bot_sessions
SET next_turn_position = MAX(
  next_turn_position,
  COALESCE((
    SELECT MAX(turn_position) + 1
    FROM bot_history_messages
    WHERE bot_history_messages.session_id = bot_sessions.id
      AND turn_position IS NOT NULL
  ), 1)
);

DROP TABLE IF EXISTS _memoh_history_turn_backfill;

COMMIT;
