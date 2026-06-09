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
