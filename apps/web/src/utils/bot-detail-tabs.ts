import type { HostSurface } from './desktop-runtime'

export type BotWorkspaceBackend = 'container' | 'local' | 'remote' | 'unknown'

export interface BotDetailsTabRule {
  value: string
  containerWorkspaceOnly?: boolean
  hideForLocalWorkspace?: boolean
}

export interface BotDetailsTabPolicyContext {
  host: HostSurface
  canManageBot: boolean
  botWorkspaceBackend: BotWorkspaceBackend
  serverCapabilities?: {
    localWorkspaceEnabled?: boolean
    snapshotSupported?: boolean
    containerBackend?: string
  }
}

export function filterBotDetailsTabs<T extends BotDetailsTabRule>(
  tabs: T[],
  context: BotDetailsTabPolicyContext,
): T[] {
  if (!context.canManageBot) {
    return tabs.filter(tab => tab.value === 'overview')
  }

  return tabs.filter((tab) => {
    if (tab.hideForLocalWorkspace && context.botWorkspaceBackend === 'local') return false
    // Container-only tabs disappear for both non-container workspace forms:
    // local (trusted host) and remote (user's own computer).
    if (tab.containerWorkspaceOnly && (context.botWorkspaceBackend === 'local' || context.botWorkspaceBackend === 'remote')) return false
    return true
  })
}
