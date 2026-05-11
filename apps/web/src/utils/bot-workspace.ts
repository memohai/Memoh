export function workspaceBackendFromMetadata(metadata: unknown): string {
  if (!metadata || typeof metadata !== 'object') return ''
  const workspace = (metadata as Record<string, unknown>).workspace
  if (!workspace || typeof workspace !== 'object') return ''
  const backend = (workspace as Record<string, unknown>).backend
  return typeof backend === 'string' ? backend.trim().toLowerCase() : ''
}

export function isLocalWorkspaceBot(metadata: unknown, workspaceBackend?: string | null): boolean {
  return workspaceBackendFromMetadata(metadata) === 'local'
    || (workspaceBackend ?? '').trim().toLowerCase() === 'local'
}
