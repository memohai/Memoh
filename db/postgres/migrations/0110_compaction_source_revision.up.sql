-- 0110_compaction_source_revision
-- Give each persisted history source an explicit aggregate revision.

LOCK TABLE bot_history_message_assets IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

ALTER TABLE bot_history_messages
  ADD COLUMN IF NOT EXISTS source_revision BIGINT NOT NULL DEFAULT 1;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'history_message_source_revision_positive'
      AND conrelid = 'bot_history_messages'::regclass
  ) THEN
    ALTER TABLE bot_history_messages
      ADD CONSTRAINT history_message_source_revision_positive
      CHECK (source_revision > 0);
  END IF;
END $$;

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

DROP TRIGGER IF EXISTS history_message_source_revision_bump
  ON bot_history_messages;
CREATE TRIGGER history_message_source_revision_bump
BEFORE UPDATE ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION bump_history_message_source_revision();

CREATE OR REPLACE FUNCTION bump_message_source_revision_for_asset()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND (to_jsonb(OLD) - 'created_at') IS NOT DISTINCT FROM (to_jsonb(NEW) - 'created_at') THEN
    RETURN NULL;
  END IF;

  IF TG_OP = 'INSERT' THEN
    PERFORM message.id
    FROM bot_history_messages message
    WHERE message.id = NEW.message_id
    ORDER BY message.id
    FOR UPDATE;

    UPDATE bot_history_messages
    SET source_revision = source_revision + 1
    WHERE id = NEW.message_id;
  ELSIF TG_OP = 'DELETE' THEN
    PERFORM message.id
    FROM bot_history_messages message
    WHERE message.id = OLD.message_id
    ORDER BY message.id
    FOR UPDATE;

    UPDATE bot_history_messages
    SET source_revision = source_revision + 1
    WHERE id = OLD.message_id;
  ELSE
    PERFORM message.id
    FROM bot_history_messages message
    WHERE message.id IN (OLD.message_id, NEW.message_id)
    ORDER BY message.id
    FOR UPDATE;

    UPDATE bot_history_messages
    SET source_revision = source_revision + 1
    WHERE id IN (OLD.message_id, NEW.message_id);
  END IF;

  RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS history_message_asset_source_revision_bump
  ON bot_history_message_assets;
CREATE TRIGGER history_message_asset_source_revision_bump
AFTER INSERT OR UPDATE OR DELETE ON bot_history_message_assets
FOR EACH ROW
EXECUTE FUNCTION bump_message_source_revision_for_asset();
