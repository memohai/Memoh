import type { CommandActionListItem } from '@/composables/api/useChat'

const QUICK_ACTION_SLASH_TEXT: Record<string, string> = {
  help: '/help',
  'skill.list': '/skill list',
}

function commandResultItemKind(item: CommandActionListItem): string {
  return item.kind?.trim().toLowerCase() ?? ''
}

function commandResultQuickActionID(item: CommandActionListItem): string {
  return item.id?.trim() ?? ''
}

function isCurrentQuickAction(item: CommandActionListItem, currentActionID = ''): boolean {
  const id = commandResultQuickActionID(item)
  return !!id && id === currentActionID.trim()
}

export function commandResultQuickActionText(item: CommandActionListItem, currentActionID = ''): string {
  if (commandResultItemKind(item) !== 'quick_action') return ''
  if (isCurrentQuickAction(item, currentActionID)) return ''
  const id = commandResultQuickActionID(item)
  if (id && QUICK_ACTION_SLASH_TEXT[id]) return QUICK_ACTION_SLASH_TEXT[id]

  const title = item.title.trim()
  return title.startsWith('/') ? title : ''
}

export function isCommandResultItemSelectable(item: CommandActionListItem, currentActionID = ''): boolean {
  const kind = commandResultItemKind(item)
  if (kind === 'quick_action') return !!commandResultQuickActionText(item, currentActionID)
  if (kind === 'skill') return !!item.id?.trim() && !!item.title.trim()
  return false
}
