-- 0081_add_orchestration_event_outbox_index
-- Speeds up the orchestration run-event outbox dispatcher by adding a partial
-- index covering only rows that are still pending JetStream publication. The
-- dispatcher scans these in commit order (run_id, seq) and marks them as
-- published once the bus accepts them.

CREATE INDEX IF NOT EXISTS idx_orchestration_events_unpublished
    ON orchestration_events(run_id, seq) WHERE published_at IS NULL;
