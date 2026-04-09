-- 0065_add_context_token_budget (down)

ALTER TABLE bots DROP COLUMN IF EXISTS persist_full_tool_results;
ALTER TABLE bots DROP COLUMN IF EXISTS context_token_budget;
