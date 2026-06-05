-- 0015_plugin_system
-- Remove plugin installation records and plugin-managed MCP metadata.

PRAGMA foreign_keys = OFF;

DROP TABLE IF EXISTS bot_plugin_resources;

CREATE TABLE mcp_connections_new (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  type TEXT NOT NULL,
  config TEXT NOT NULL DEFAULT '{}',
  is_active INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'unknown',
  tools_cache TEXT NOT NULL DEFAULT '[]',
  last_probed_at TEXT,
  status_message TEXT NOT NULL DEFAULT '',
  auth_type TEXT NOT NULL DEFAULT 'none',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT mcp_connections_type_check CHECK (type IN ('stdio', 'http', 'sse')),
  CONSTRAINT mcp_connections_unique UNIQUE (bot_id, name)
);

INSERT INTO mcp_connections_new (
  id, bot_id, name, type, config, is_active, status, tools_cache,
  last_probed_at, status_message, auth_type, created_at, updated_at
)
SELECT
  id, bot_id, name, type, config, is_active, status, tools_cache,
  last_probed_at, status_message, auth_type, created_at, updated_at
FROM mcp_connections;

DROP TABLE mcp_connections;
ALTER TABLE mcp_connections_new RENAME TO mcp_connections;

CREATE INDEX IF NOT EXISTS idx_mcp_connections_bot_id ON mcp_connections(bot_id);

DROP TABLE IF EXISTS bot_plugin_installations;

PRAGMA foreign_keys = ON;
