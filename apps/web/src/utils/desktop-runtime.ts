export type DesktopRuntimeMode = 'local' | 'remote'
export type HostSurface = 'web' | 'desktop'

export interface DesktopServerStatus {
  mode?: unknown
  baseUrl?: string
  ready?: boolean
  managed?: boolean
}

export interface DesktopApiBridge {
  desktop?: {
    getServerStatus?: () => Promise<DesktopServerStatus>
    defaultWorkspacePath?: (displayName: string) => Promise<string>
    openProjectFolder?: () => Promise<string | null>
  }
}

export interface LocalWorkspaceCreatePolicyInput {
  serverLocalWorkspaceEnabled: boolean
  host: HostSurface
  desktopRuntimeMode: DesktopRuntimeMode | null
}

export function normalizeDesktopRuntimeMode(value: unknown): DesktopRuntimeMode | null {
  return value === 'local' || value === 'remote' ? value : null
}

export function desktopApiBridge(): DesktopApiBridge | null {
  if (typeof window === 'undefined') return null
  const bridge = (window as unknown as { api?: DesktopApiBridge }).api
  return bridge && typeof bridge === 'object' ? bridge : null
}

export function hostSurface(): HostSurface {
  const bridge = desktopApiBridge()
  return typeof bridge?.desktop?.getServerStatus === 'function' ? 'desktop' : 'web'
}

export function canCreateLocalWorkspace(input: LocalWorkspaceCreatePolicyInput): boolean {
  if (!input.serverLocalWorkspaceEnabled) return false
  if (input.host === 'web') return true
  return input.desktopRuntimeMode === 'local'
}

export function canPickProjectFolder(): boolean {
  return typeof desktopApiBridge()?.desktop?.openProjectFolder === 'function'
}

export async function pickProjectFolder(): Promise<string | null> {
  const open = desktopApiBridge()?.desktop?.openProjectFolder
  if (typeof open !== 'function') return null
  try {
    return await open()
  } catch {
    return null
  }
}
