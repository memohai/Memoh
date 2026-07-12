-- 0111_activate_message_source_context
-- Retire all artifacts and restore dormant source-context storage.
-- Maintenance-window migration: stop all post-0111 writers before applying it.

LOCK TABLE bot_history_message_compacts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

DELETE FROM bot_history_message_compact_parent_edges;
DELETE FROM bot_history_message_compacts;

DROP TRIGGER IF EXISTS history_message_source_context_capture
  ON bot_history_messages;
DROP FUNCTION IF EXISTS capture_history_message_source_context();

UPDATE bot_history_messages
SET source_context = NULL
WHERE source_context IS NOT NULL;

CREATE OR REPLACE FUNCTION bump_history_message_source_revision()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF ROW(
    OLD.id,
    OLD.bot_id,
    OLD.session_id,
    OLD.sender_channel_identity_id,
    OLD.sender_account_user_id,
    OLD.source_message_id,
    OLD.source_reply_to_message_id,
    OLD.role,
    OLD.content,
    OLD.metadata,
    OLD.usage,
    OLD.session_mode,
    OLD.runtime_type,
    OLD.model_id,
    OLD.event_id,
    OLD.display_text,
    OLD.turn_id,
    OLD.turn_position,
    OLD.turn_message_seq,
    OLD.turn_visible,
    OLD.turn_superseded_by_turn_id,
    OLD.turn_superseded_at,
    OLD.turn_superseded_reason,
    OLD.created_at
  ) IS DISTINCT FROM ROW(
    NEW.id,
    NEW.bot_id,
    NEW.session_id,
    NEW.sender_channel_identity_id,
    NEW.sender_account_user_id,
    NEW.source_message_id,
    NEW.source_reply_to_message_id,
    NEW.role,
    NEW.content,
    NEW.metadata,
    NEW.usage,
    NEW.session_mode,
    NEW.runtime_type,
    NEW.model_id,
    NEW.event_id,
    NEW.display_text,
    NEW.turn_id,
    NEW.turn_position,
    NEW.turn_message_seq,
    NEW.turn_visible,
    NEW.turn_superseded_by_turn_id,
    NEW.turn_superseded_at,
    NEW.turn_superseded_reason,
    NEW.created_at
  ) OR NEW.source_revision IS DISTINCT FROM OLD.source_revision THEN
    NEW.source_revision := OLD.source_revision + 1;
  ELSE
    NEW.source_revision := OLD.source_revision;
  END IF;

  RETURN NEW;
END;
$$;

DROP FUNCTION IF EXISTS resolve_history_message_source_context(bot_history_messages);
DROP FUNCTION IF EXISTS normalize_history_message_source_text(TEXT);
