-- 0097_tool_approval_generic_tool
-- Allow hooks to request approval for non-file and non-exec tools.

ALTER TABLE tool_approval_requests
  DROP CONSTRAINT IF EXISTS tool_approval_operation_check;

ALTER TABLE tool_approval_requests
  ADD CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec', 'tool'));
