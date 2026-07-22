import type { BotWorkspaceBackend } from './bot-detail-tabs'

export function resolveBotWorkspaceBackend(workspaceBackend?: string | null): BotWorkspaceBackend {
  const live = (workspaceBackend ?? '').trim().toLowerCase()
  if (live === 'remote') return 'remote'
  return 'container'
}
