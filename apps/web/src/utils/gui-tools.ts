const GUI_TOOL_NAMES = new Set([
  'browser_action',
  'browser_observe',
  'browser_remote_session',
  'computer_action',
  'computer_observe',
])

export function isGuiToolName(toolName: string): boolean {
  return GUI_TOOL_NAMES.has(toolName)
}
