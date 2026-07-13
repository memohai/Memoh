-- 0114_compaction_topology_positions
-- Track range-local history topology changes for derived compaction validity.

LOCK TABLE bot_history_message_compacts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

CREATE TABLE IF NOT EXISTS bot_history_topology_counters (
  session_id UUID PRIMARY KEY,
  revision BIGINT NOT NULL CHECK (revision > 0)
);

CREATE TABLE IF NOT EXISTS bot_history_topology_positions (
  session_id UUID NOT NULL,
  turn_position BIGINT NOT NULL,
  revision BIGINT NOT NULL CHECK (revision > 0),
  PRIMARY KEY (session_id, turn_position)
);

CREATE INDEX IF NOT EXISTS idx_history_topology_positions_session_revision
  ON bot_history_topology_positions (session_id, revision, turn_position);

CREATE TABLE IF NOT EXISTS bot_history_topology_pending (
  transaction_id XID8 NOT NULL,
  session_id UUID NOT NULL,
  turn_position BIGINT NOT NULL,
  PRIMARY KEY (transaction_id, session_id, turn_position)
);

CREATE TABLE IF NOT EXISTS bot_history_message_compact_topology (
  compact_id UUID PRIMARY KEY REFERENCES bot_history_message_compacts(id) ON DELETE CASCADE,
  session_id UUID NOT NULL,
  topology_revision BIGINT NOT NULL CHECK (topology_revision >= 0),
  range_start_turn_position BIGINT NOT NULL,
  range_end_turn_position BIGINT NOT NULL,
  CONSTRAINT compaction_topology_range_order_check
    CHECK (range_start_turn_position <= range_end_turn_position)
);

CREATE INDEX IF NOT EXISTS idx_compaction_topology_session_range
  ON bot_history_message_compact_topology (
    session_id,
    range_start_turn_position,
    range_end_turn_position
  );

CREATE OR REPLACE FUNCTION enqueue_history_topology_position(
  target_session_id UUID,
  target_turn_position BIGINT
)
RETURNS VOID
LANGUAGE plpgsql
AS $$
BEGIN
  IF target_session_id IS NULL
     OR target_turn_position IS NULL THEN
    RETURN;
  END IF;

  INSERT INTO bot_history_topology_pending (
    transaction_id,
    session_id,
    turn_position
  ) VALUES (
    pg_current_xact_id(),
    target_session_id,
    target_turn_position
  )
  ON CONFLICT DO NOTHING;
END;
$$;

CREATE OR REPLACE FUNCTION flush_history_topology_positions()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  current_transaction_id XID8 := pg_current_xact_id();
  changed_session RECORD;
  next_revision BIGINT;
BEGIN
  FOR changed_session IN
    SELECT pending.session_id
    FROM bot_history_topology_pending pending
    JOIN bot_sessions session ON session.id = pending.session_id
    WHERE pending.transaction_id = current_transaction_id
    GROUP BY pending.session_id
    ORDER BY pending.session_id
  LOOP
    INSERT INTO bot_history_topology_counters (session_id, revision)
    VALUES (changed_session.session_id, 1)
    ON CONFLICT (session_id) DO UPDATE
    SET revision = bot_history_topology_counters.revision + 1
    RETURNING revision INTO next_revision;

    INSERT INTO bot_history_topology_positions (session_id, turn_position, revision)
    SELECT pending.session_id, pending.turn_position, next_revision
    FROM bot_history_topology_pending pending
    WHERE pending.transaction_id = current_transaction_id
      AND pending.session_id = changed_session.session_id
    ON CONFLICT (session_id, turn_position) DO UPDATE
    SET revision = EXCLUDED.revision;
  END LOOP;

  DELETE FROM bot_history_topology_pending
  WHERE transaction_id = current_transaction_id;

  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION cleanup_history_topology_session()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  DELETE FROM bot_history_message_compact_topology
  WHERE session_id = OLD.id;
  DELETE FROM bot_history_topology_pending
  WHERE session_id = OLD.id;
  DELETE FROM bot_history_topology_positions
  WHERE session_id = OLD.id;
  DELETE FROM bot_history_topology_counters
  WHERE session_id = OLD.id;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION record_history_message_topology_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
  old_eligible BOOLEAN := false;
  new_eligible BOOLEAN := false;
  topology_changed BOOLEAN := true;
BEGIN
  IF TG_OP <> 'INSERT' THEN
    old_eligible := OLD.session_id IS NOT NULL
      AND OLD.turn_visible
      AND OLD.turn_id IS NOT NULL
      AND OLD.turn_position IS NOT NULL
      AND OLD.turn_message_seq IS NOT NULL
      AND (OLD.metadata->>'trigger_mode' IS NULL OR OLD.metadata->>'trigger_mode' <> 'passive_sync');
  END IF;

  IF TG_OP <> 'DELETE' THEN
    new_eligible := NEW.session_id IS NOT NULL
      AND NEW.turn_visible
      AND NEW.turn_id IS NOT NULL
      AND NEW.turn_position IS NOT NULL
      AND NEW.turn_message_seq IS NOT NULL
      AND (NEW.metadata->>'trigger_mode' IS NULL OR NEW.metadata->>'trigger_mode' <> 'passive_sync');
  END IF;

  IF TG_OP = 'UPDATE' THEN
    topology_changed := ROW(
      OLD.id,
      OLD.bot_id,
      OLD.session_id,
      OLD.turn_id,
      OLD.turn_position,
      OLD.turn_message_seq,
      OLD.turn_visible,
      OLD.metadata->>'trigger_mode',
      OLD.created_at
    ) IS DISTINCT FROM ROW(
      NEW.id,
      NEW.bot_id,
      NEW.session_id,
      NEW.turn_id,
      NEW.turn_position,
      NEW.turn_message_seq,
      NEW.turn_visible,
      NEW.metadata->>'trigger_mode',
      NEW.created_at
    );
  END IF;

  IF NOT topology_changed OR (NOT old_eligible AND NOT new_eligible) THEN
    RETURN COALESCE(NEW, OLD);
  END IF;

  IF old_eligible
     AND new_eligible
     AND OLD.session_id = NEW.session_id THEN
    PERFORM enqueue_history_topology_position(NEW.session_id, OLD.turn_position);
    PERFORM enqueue_history_topology_position(NEW.session_id, NEW.turn_position);
    RETURN NEW;
  END IF;

  IF old_eligible THEN
    PERFORM enqueue_history_topology_position(OLD.session_id, OLD.turn_position);
  END IF;
  IF new_eligible THEN
    PERFORM enqueue_history_topology_position(NEW.session_id, NEW.turn_position);
  END IF;

  RETURN COALESCE(NEW, OLD);
END;
$$;

DROP TRIGGER IF EXISTS history_message_topology_capture
  ON bot_history_messages;
CREATE TRIGGER history_message_topology_capture
AFTER INSERT OR DELETE OR UPDATE ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION record_history_message_topology_change();

DROP TRIGGER IF EXISTS history_topology_pending_flush
  ON bot_history_topology_pending;
CREATE CONSTRAINT TRIGGER history_topology_pending_flush
AFTER INSERT ON bot_history_topology_pending
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION flush_history_topology_positions();

DROP TRIGGER IF EXISTS history_topology_session_cleanup
  ON bot_sessions;
CREATE CONSTRAINT TRIGGER history_topology_session_cleanup
AFTER DELETE ON bot_sessions
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION cleanup_history_topology_session();
