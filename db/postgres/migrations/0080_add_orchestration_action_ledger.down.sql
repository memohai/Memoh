-- 0080_add_orchestration_action_ledger
-- Remove durable orchestration action ledger for worker and verifier tool calls.

DROP TABLE IF EXISTS orchestration_action_ledger;
