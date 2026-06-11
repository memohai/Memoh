-- 0016_user_input_tool_call_unique
-- Remove ask_user per-tool-call idempotency.

DROP INDEX IF EXISTS user_input_tool_call_unique;
