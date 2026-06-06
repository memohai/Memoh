-- 0015_plugin_system
-- Add plugin installation records and plugin-managed MCP metadata.

PRAGMA foreign_keys = OFF;

CREATE TABLE IF NOT EXISTS bot_plugin_installations (
  id TEXT PRIMARY KEY,
  bot_id TEXT NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
  plugin_id TEXT NOT NULL,
  plugin_name TEXT NOT NULL DEFAULT '',
  version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'ready',
  enabled INTEGER NOT NULL DEFAULT 1,
  config TEXT NOT NULL DEFAULT '{}',
  metadata TEXT NOT NULL DEFAULT '{}',
  manifest TEXT NOT NULL DEFAULT '{}',
  installed_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_plugin_installations_unique UNIQUE (bot_id, plugin_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_plugin_installations_bot_id ON bot_plugin_installations(bot_id);
CREATE INDEX IF NOT EXISTS idx_bot_plugin_installations_plugin_id ON bot_plugin_installations(plugin_id);

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
  managed_by_plugin_installation_id TEXT REFERENCES bot_plugin_installations(id) ON DELETE SET NULL,
  managed_resource_key TEXT NOT NULL DEFAULT '',
  visible INTEGER NOT NULL DEFAULT 1,
  metadata TEXT NOT NULL DEFAULT '{}',
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
CREATE INDEX IF NOT EXISTS idx_mcp_connections_plugin_installation_id ON mcp_connections(managed_by_plugin_installation_id);

CREATE TABLE IF NOT EXISTS bot_plugin_resources (
  id TEXT PRIMARY KEY,
  installation_id TEXT NOT NULL REFERENCES bot_plugin_installations(id) ON DELETE CASCADE,
  resource_type TEXT NOT NULL,
  resource_key TEXT NOT NULL,
  resource_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT bot_plugin_resources_unique UNIQUE (installation_id, resource_type, resource_key)
);

CREATE INDEX IF NOT EXISTS idx_bot_plugin_resources_installation_id ON bot_plugin_resources(installation_id);
CREATE INDEX IF NOT EXISTS idx_bot_plugin_resources_resource ON bot_plugin_resources(resource_type, resource_id);

PRAGMA foreign_keys = ON;
