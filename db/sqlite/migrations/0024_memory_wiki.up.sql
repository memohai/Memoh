-- 0024_memory_wiki
-- Move memory content into SQLite as a wiki/graph source of truth.
-- memory_nodes holds the canonical memory entries (one row per memory item);
-- memory_edges holds relationships between nodes (profile/topic/day/refs/...).
-- Markdown files remain the agent-facing derived view, synced from these tables.

CREATE TABLE IF NOT EXISTS memory_nodes (
    id               TEXT        PRIMARY KEY,            -- botID:mem_<nanosec>
    bot_id           TEXT        NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    body             TEXT        NOT NULL,
    hash             TEXT        NOT NULL,
    layer            TEXT        NOT NULL DEFAULT 'note',-- preference|identity|context|experience|activity|persona|note
    fact_type        TEXT        NOT NULL DEFAULT '',
    subject          TEXT        NOT NULL DEFAULT '',
    confidence       REAL        NOT NULL DEFAULT 0.5,
    metadata         TEXT        NOT NULL DEFAULT '{}',
    source_message_ids TEXT      NOT NULL DEFAULT '[]',
    profile_ref      TEXT        NOT NULL DEFAULT '',
    topic            TEXT        NOT NULL DEFAULT '',
    captured_at      TEXT        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at       TEXT,
    updated_at       TEXT        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at       TEXT        NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT memory_nodes_confidence_check CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE INDEX IF NOT EXISTS idx_memory_nodes_bot_layer  ON memory_nodes (bot_id, layer);
CREATE INDEX IF NOT EXISTS idx_memory_nodes_bot_topic  ON memory_nodes (bot_id, topic);
CREATE INDEX IF NOT EXISTS idx_memory_nodes_bot_prof   ON memory_nodes (bot_id, profile_ref);
CREATE INDEX IF NOT EXISTS idx_memory_nodes_updated    ON memory_nodes (bot_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS memory_edges (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    bot_id      TEXT    NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    src_node    TEXT    NOT NULL,
    dst_node    TEXT    NOT NULL,
    rel         TEXT    NOT NULL,                  -- same_profile|same_topic|same_day|refs|supersedes|contradicts|followup
    weight      REAL    NOT NULL DEFAULT 1.0,
    metadata    TEXT    NOT NULL DEFAULT '{}',
    created_at  TEXT    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT memory_edges_unique UNIQUE (bot_id, src_node, dst_node, rel)
);

CREATE INDEX IF NOT EXISTS idx_memory_edges_src  ON memory_edges (bot_id, src_node);
CREATE INDEX IF NOT EXISTS idx_memory_edges_dst  ON memory_edges (bot_id, dst_node);
CREATE INDEX IF NOT EXISTS idx_memory_edges_rel  ON memory_edges (bot_id, rel);
