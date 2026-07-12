-- 0113_compaction_topology_positions
-- Remove range-local history topology tracking.

DROP TRIGGER IF EXISTS history_message_topology_capture
  ON bot_history_messages;
DROP TRIGGER IF EXISTS history_topology_pending_flush
  ON bot_history_topology_pending;
DROP FUNCTION IF EXISTS flush_history_topology_positions();
DROP FUNCTION IF EXISTS record_history_message_topology_change();
DROP FUNCTION IF EXISTS enqueue_history_topology_position(UUID, BIGINT);

DROP TABLE IF EXISTS bot_history_message_compact_topology;
DROP TABLE IF EXISTS bot_history_topology_pending;
DROP TABLE IF EXISTS bot_history_topology_positions;
DROP TABLE IF EXISTS bot_history_topology_counters;
