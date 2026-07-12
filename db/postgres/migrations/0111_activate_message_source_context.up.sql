-- 0111_activate_message_source_context
-- Activate immutable source envelopes without invalidating successful legacy artifacts.
-- Maintenance-window migration: stop all pre-0111 writers before applying it.

LOCK TABLE bot_history_message_compacts IN SHARE ROW EXCLUSIVE MODE;
LOCK TABLE bot_history_messages IN SHARE ROW EXCLUSIVE MODE;

UPDATE bot_history_message_compacts
SET status = 'error',
    summary = '',
    message_count = 0,
    error_message = 'compaction attempt retired by source context activation',
    usage = NULL,
    model_id = NULL,
    coverage = '[]'::jsonb,
    anchor_start_ms = 0,
    anchor_end_ms = 0,
    completed_at = now()
WHERE status = 'pending';

CREATE OR REPLACE FUNCTION normalize_history_message_source_text(value TEXT)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
  SELECT btrim(
    value,
    U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000'
  );
$$;

CREATE OR REPLACE FUNCTION resolve_history_message_source_context(
  source_message bot_history_messages
)
RETURNS JSONB
LANGUAGE sql
STABLE
AS $$
WITH matched_event AS MATERIALIZED (
  SELECT event.event_data
  FROM bot_session_events event
  WHERE event.id = source_message.event_id
    AND event.bot_id = source_message.bot_id
    AND event.event_kind = 'message'
    AND (
      source_message.session_id IS NULL
      OR event.session_id = source_message.session_id
    )
    AND (
      source_message.sender_channel_identity_id IS NULL
      OR event.sender_channel_identity_id IS NULL
      OR event.sender_channel_identity_id = source_message.sender_channel_identity_id
    )
  LIMIT 1
), matched_identity AS MATERIALIZED (
  SELECT identity.display_name, identity.channel_type
  FROM channel_identities identity
  WHERE identity.id = source_message.sender_channel_identity_id
  LIMIT 1
), matched_session AS MATERIALIZED (
  SELECT session.channel_type, session.route_id
  FROM bot_sessions session
  WHERE session.id = source_message.session_id
    AND session.bot_id = source_message.bot_id
  LIMIT 1
), matched_route AS MATERIALIZED (
  SELECT route.channel_type, route.conversation_type, route.metadata
  FROM bot_channel_routes route
  JOIN matched_session session ON session.route_id = route.id
  WHERE route.bot_id = source_message.bot_id
  LIMIT 1
)
SELECT jsonb_build_object(
  'version', 1,
  'sender_display_name', COALESCE(
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(event.event_data #> '{sender,display_name}') = 'string'
      THEN event.event_data #>> '{sender,display_name}'
    END), ''),
    NULLIF(normalize_history_message_source_text(identity.display_name), ''),
    ''
  ),
  'platform', COALESCE(
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(event.event_data #> '{conversation,channel}') = 'string'
      THEN event.event_data #>> '{conversation,channel}'
    END), ''),
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(source_message.metadata->'platform') = 'string'
      THEN source_message.metadata->>'platform'
    END), ''),
    NULLIF(normalize_history_message_source_text(route.channel_type), ''),
    NULLIF(normalize_history_message_source_text(session.channel_type), ''),
    NULLIF(normalize_history_message_source_text(identity.channel_type), ''),
    ''
  ),
  'conversation_type', CASE LOWER(COALESCE(
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(event.event_data #> '{conversation,conversation_type}') = 'string'
      THEN event.event_data #>> '{conversation,conversation_type}'
    END), ''),
    NULLIF(normalize_history_message_source_text(route.conversation_type), ''),
    ''
  ))
    WHEN '' THEN ''
    WHEN 'p2p' THEN 'private'
    WHEN 'direct' THEN 'private'
    WHEN 'dm' THEN 'private'
    WHEN 'private' THEN 'private'
    WHEN 'thread' THEN 'thread'
    WHEN 'topic' THEN 'thread'
    ELSE 'group'
  END,
  'conversation_name', COALESCE(
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(event.event_data #> '{conversation,conversation_name}') = 'string'
      THEN event.event_data #>> '{conversation,conversation_name}'
    END), ''),
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(route.metadata->'conversation_name') = 'string'
      THEN route.metadata->>'conversation_name'
    END), ''),
    NULLIF(normalize_history_message_source_text(CASE
      WHEN jsonb_typeof(route.metadata->'conversation_handle') = 'string'
      THEN route.metadata->>'conversation_handle'
    END), ''),
    ''
  )
)
FROM (SELECT 1) seed
LEFT JOIN matched_event event ON true
LEFT JOIN matched_identity identity ON true
LEFT JOIN matched_session session ON true
LEFT JOIN matched_route route ON true;
$$;

CREATE OR REPLACE FUNCTION capture_history_message_source_context()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
  IF TG_OP = 'UPDATE'
     AND NEW.source_context IS DISTINCT FROM OLD.source_context
     AND (OLD.source_context IS NOT NULL OR NEW.source_context IS NULL) THEN
    RAISE EXCEPTION 'message % source context is immutable once captured', OLD.id
      USING ERRCODE = '23514';
  END IF;

  IF NEW.source_context IS NOT NULL THEN
    RETURN NEW;
  END IF;

  IF TG_OP = 'INSERT'
     OR (
       OLD.compact_id IS NOT NULL
       AND NEW.compact_id IS NULL
     ) THEN
    NEW.source_context := resolve_history_message_source_context(NEW);
  END IF;

  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS history_message_source_context_capture
  ON bot_history_messages;
CREATE TRIGGER history_message_source_context_capture
BEFORE INSERT OR UPDATE OF compact_id, source_context ON bot_history_messages
FOR EACH ROW
EXECUTE FUNCTION capture_history_message_source_context();

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
    OLD.created_at,
    OLD.source_context
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
    NEW.created_at,
    NEW.source_context
  ) OR NEW.source_revision IS DISTINCT FROM OLD.source_revision THEN
    NEW.source_revision := OLD.source_revision + 1;
  ELSE
    NEW.source_revision := OLD.source_revision;
  END IF;

  RETURN NEW;
END;
$$;

UPDATE bot_history_messages message
SET source_context = resolve_history_message_source_context(message)
WHERE message.source_context IS NULL
  AND (
    message.compact_id IS NULL
    OR NOT EXISTS (
      SELECT 1
      FROM bot_history_message_compacts compact
      WHERE compact.id = message.compact_id
        AND compact.status = 'ok'
    )
  );

ALTER TABLE bot_history_messages
  VALIDATE CONSTRAINT history_message_source_context_valid;
