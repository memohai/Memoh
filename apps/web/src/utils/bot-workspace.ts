import type { BotWorkspaceBackend } from './bot-detail-tabs'

export function workspaceBackendFromMetadata(metadata: unknown): string {
  if (!metadata || typeof metadata !== 'object') return ''
  const workspace = (metadata as Record<string, unknown>).workspace
  if (!workspace || typeof workspace !== 'object') return ''
  const backend = (workspace as Record<string, unknown>).backend
  return typeof backend === 'string' ? backend.trim().toLowerCase() : ''
}

export function resolveBotWorkspaceBackend(metadata: unknown, workspaceBackend?: string | null): BotWorkspaceBackend {
  const backend = workspaceBackendFromMetadata(metadata) || (workspaceBackend ?? '').trim().toLowerCase()
  if (backend === 'local') return 'local'
  if (backend === 'container') return 'container'
  return 'unknown'
}

export function isLocalWorkspaceBot(metadata: unknown, workspaceBackend?: string | null): boolean {
  return resolveBotWorkspaceBackend(metadata, workspaceBackend) === 'local'
}
