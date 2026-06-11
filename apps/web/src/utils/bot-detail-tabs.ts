import type { DesktopRuntimeMode, HostSurface } from './desktop-runtime'

export type BotWorkspaceBackend = 'container' | 'local' | 'unknown'

export interface BotDetailsTabRule {
  value: string
  remoteOnly?: boolean
  localOnly?: boolean
  containerWorkspaceOnly?: boolean
  hideForLocalWorkspace?: boolean
}

export interface BotDetailsTabPolicyContext {
  host: HostSurface
  desktopRuntimeMode: DesktopRuntimeMode | null
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
    if (tab.remoteOnly && context.desktopRuntimeMode !== 'remote') return false
    if (tab.localOnly && context.desktopRuntimeMode !== 'local') return false
    if (tab.hideForLocalWorkspace && context.botWorkspaceBackend === 'local') return false
    if (tab.containerWorkspaceOnly && context.botWorkspaceBackend === 'local') return false
    return true
  })
}
