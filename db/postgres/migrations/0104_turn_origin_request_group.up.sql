-- 0104_turn_origin_request_group
-- Materialize turn provenance on bot_history_turns: origin_kind/origin_turn_id
-- record how a turn was created (message | retry | edit), and request_group_id
-- groups sibling turns that carry the same logical request (retry copies the
-- source turn's group, send/edit start a new group). NULL request_group_id
-- means the turn is its own group (COALESCE(request_group_id, id)). This lets
-- the session turn graph endpoint drop its request-content/asset hashing joins.

ALTER TABLE bot_history_turns
  ADD COLUMN IF NOT EXISTS origin_kind TEXT,
  ADD COLUMN IF NOT EXISTS origin_turn_id UUID REFERENCES bot_history_turns(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS request_group_id UUID;

-- Backfill legacy sibling groups with the same request fingerprint the old
-- graph endpoint hashed at read time (request text + ordered asset key), so
-- pre-existing retry variants keep grouping together. Turns without a request
-- message stay NULL (each is its own group). Group id = earliest turn in the
-- group; the leader itself stays NULL (self-group by convention).
WITH request_assets AS (
  SELECT
    a.message_id,
    string_agg(
      concat_ws(
        ':',
        COALESCE(a.content_hash, ''),
        COALESCE(a.name, ''),
        COALESCE(a.role, ''),
        COALESCE(a.ordinal::text, '')
      ),
      '|'
      ORDER BY a.content_hash, a.name, a.role, a.ordinal, a.id
    ) AS asset_key
  FROM bot_history_message_assets a
  GROUP BY a.message_id
),
keyed AS (
  SELECT
    t.id,
    COALESCE(t.parent_turn_id::text, '') AS parent_key,
    COALESCE(NULLIF(btrim(rm.display_text), ''), md5(rm.content::text)) AS request_text_key,
    COALESCE(ra.asset_key, '') AS asset_key,
    t.created_at
  FROM bot_history_turns t
  JOIN bot_history_messages rm ON rm.id = t.request_message_id
  LEFT JOIN request_assets ra ON ra.message_id = t.request_message_id
  WHERE t.request_group_id IS NULL
),
grouped AS (
  SELECT
    id,
    first_value(id) OVER (
      PARTITION BY parent_key, request_text_key, asset_key
      ORDER BY created_at, id
    ) AS group_id,
    count(*) OVER (
      PARTITION BY parent_key, request_text_key, asset_key
    ) AS group_size
  FROM keyed
)
UPDATE bot_history_turns t
SET request_group_id = g.group_id
FROM grouped g
WHERE t.id = g.id
  AND g.group_size > 1
  AND g.group_id <> t.id;
