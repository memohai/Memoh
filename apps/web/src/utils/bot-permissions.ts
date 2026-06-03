export type BotPermission =
  | 'chat'
  | 'workspace_read'
  | 'workspace_write'
  | 'workspace_exec'
  | 'manage'

export const BOT_PERMISSION_ORDER: BotPermission[] = [
  'chat',
  'workspace_read',
  'workspace_write',
  'workspace_exec',
  'manage',
]

export function expandBotPermissions(permissions: readonly string[] | null | undefined): BotPermission[] {
  const seen = new Set<BotPermission>()
  for (const permission of permissions ?? []) {
    if (BOT_PERMISSION_ORDER.includes(permission as BotPermission)) {
      seen.add(permission as BotPermission)
    }
  }
  if (seen.has('manage')) {
    for (const permission of BOT_PERMISSION_ORDER) seen.add(permission)
  }
  if (seen.has('workspace_write')) {
    seen.add('workspace_read')
  }
  return BOT_PERMISSION_ORDER.filter(permission => seen.has(permission))
}

export function hasBotPermission(permissions: readonly string[] | null | undefined, permission: BotPermission): boolean {
  return expandBotPermissions(permissions).includes(permission)
}
