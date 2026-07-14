import type { InjectionKey } from 'vue'

// Provided by the Electron desktop shell to enable a macOS-style top inset
// (traffic-light reserve + custom TopBar) inside reusable web sidebars.
// Web (browser) does not provide this key, so consumers fall back to false.
export const DesktopShellKey: InjectionKey<boolean> = Symbol('memohai:desktop-shell')

export type DesktopRuntimeStatus =
  | 'disabled'
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'stopped'
  | 'error'

export interface DesktopRuntimeState {
  enabled: boolean
  runtimeId?: string
  runtimeName?: string
  status: DesktopRuntimeStatus
  deviceName: string
  error?: string
}

export interface DesktopRuntimeBridge {
  runtimeState(): Promise<DesktopRuntimeState>
  configureRuntime(config: { runtimeId: string, name: string, key: string } | null): Promise<DesktopRuntimeState>
  onRuntimeStateChanged(listener: (state: DesktopRuntimeState) => void): () => void
}

export const DesktopRuntimeKey: InjectionKey<DesktopRuntimeBridge | undefined> = Symbol('memohai:desktop-runtime')
