import type { McpToolDescriptor } from '@memohai/sdk'

// Shared shape between the MCP list (bot-mcp.vue) and the per-server detail
// (mcp-server-detail.vue). The list seeds the detail with one of these; the
// detail keeps its own working copy so probe results never depend on a reload.
export interface McpItem {
  id: string
  name: string
  type: string
  config: Record<string, unknown>
  is_active: boolean
  status: string
  tools_cache: McpToolDescriptor[]
  last_probed_at: string | null
  status_message: string
  auth_type: string
}
