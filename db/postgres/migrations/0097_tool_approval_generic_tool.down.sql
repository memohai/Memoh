-- 0097_tool_approval_generic_tool (down)
-- Remove the generic tool approval operation.

UPDATE tool_approval_requests
SET operation = 'exec'
WHERE operation = 'tool';

ALTER TABLE tool_approval_requests
  DROP CONSTRAINT IF EXISTS tool_approval_operation_check;

ALTER TABLE tool_approval_requests
  ADD CONSTRAINT tool_approval_operation_check CHECK (operation IN ('read', 'write', 'exec'));
