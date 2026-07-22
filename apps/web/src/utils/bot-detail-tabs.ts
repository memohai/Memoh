export type BotWorkspaceBackend = 'container' | 'remote'

export interface BotDetailsTabRule {
  value: string
  containerWorkspaceOnly?: boolean
}

export interface BotDetailsTabPolicyContext {
  canManageBot: boolean
  botWorkspaceBackend: BotWorkspaceBackend
}

export function filterBotDetailsTabs<T extends BotDetailsTabRule>(
  tabs: T[],
  context: BotDetailsTabPolicyContext,
): T[] {
  if (!context.canManageBot) {
    return tabs.filter(tab => tab.value === 'overview')
  }

  return tabs.filter((tab) => {
    if (tab.containerWorkspaceOnly && context.botWorkspaceBackend === 'remote') return false
    return true
  })
}
