export type HostSurface = 'web' | 'desktop'

export interface DesktopServerStatus {
  baseUrl?: string
  ready?: boolean
  managed?: boolean
}

export interface DesktopApiBridge {
  desktop?: {
    getServerStatus?: () => Promise<DesktopServerStatus>
  }
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
