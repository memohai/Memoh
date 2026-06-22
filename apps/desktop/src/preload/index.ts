import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'
import {
  DESKTOP_KEYBOARD_COMMAND_CHANNEL,
  isAppKeyboardCommand,
  type AppKeyboardCommand,
} from '../shared/keyboard-commands'

// Renderer query-cache invalidation payload. Mirrors the subset of
// Pinia Colada's `UseQueryEntryFilter` that survives structured-clone
// serialization across the IPC boundary (no functions / predicates).
export interface RendererInvalidatePayload {
  filters?: {
    key?: unknown
    exact?: boolean
    stale?: boolean | null
    status?: unknown
  }
  refetchActive?: boolean | 'all'
}

// Bundled CLI status — mirrors CliStatus in main/cli-integration.ts.
// Kept inline rather than imported to avoid pulling main-process modules
// into the preload bundle.
export interface CliStatusPayload {
  state: 'not-installed' | 'installed-current' | 'installed-stale' | 'installed-foreign'
  source: string
  target: string | null
  error?: string
}

export type DesktopRuntimeMode = 'local' | 'remote'

// Renderer-facing API surface. Keep this intentionally small — it is the
// full security boundary between chromium renderer processes and the
// node-privileged main process.
const api = {
  desktop: {
    getServerStatus: (): Promise<{
      mode: DesktopRuntimeMode
      baseUrl: string
      ready: boolean
      managed: boolean
      error?: string
      qdrant?: {
        grpcBaseUrl: string
        httpBaseUrl: string
        ready: boolean
      }
    }> =>
      ipcRenderer.invoke('desktop:server-status'),
    apiBaseUrl: (): Promise<string> => ipcRenderer.invoke('desktop:api-base-url'),
    authToken: (): Promise<string> => ipcRenderer.invoke('desktop:auth-token'),
    defaultWorkspacePath: (displayName: string): Promise<string> =>
      ipcRenderer.invoke('desktop:default-workspace-path', displayName),
    getCliStatus: (): Promise<CliStatusPayload> => ipcRenderer.invoke('desktop:cli-status'),
    installCli: (): Promise<CliStatusPayload> => ipcRenderer.invoke('desktop:cli-install'),
    uninstallCli: (): Promise<CliStatusPayload> => ipcRenderer.invoke('desktop:cli-uninstall'),
    // Push the renderer's authoritative menu accelerators (derived from the
    // Keyboard Shortcuts store) so the main process can rebuild native menu
    // items with the user's bindings instead of the static table defaults.
    setMenuAccelerators: (overrides: Record<string, string>): Promise<void> =>
      ipcRenderer.invoke('desktop:set-menu-accelerators', overrides),
    openExternalUrl: (url: string): Promise<void> => ipcRenderer.invoke('desktop:open-external-url', url),
    // Tell the main process to fan a query-cache invalidation out to other
    // renderer hosts. Used by `setupRendererCacheSync` to mirror mutations
    // without sharing JS heap state.
    broadcastInvalidate: (payload: RendererInvalidatePayload): Promise<void> =>
      ipcRenderer.invoke('desktop:broadcast-invalidate', payload),
    // Subscribe to invalidation events forwarded from sibling renderers.
    // Listener lives for the entire renderer lifetime.
    onInvalidate: (cb: (payload: RendererInvalidatePayload) => void): void => {
      ipcRenderer.on('desktop:invalidate', (_event: IpcRendererEvent, payload: RendererInvalidatePayload) => {
        cb(payload)
      })
    },
  },
  window: {
    closeSelf: (): Promise<void> => ipcRenderer.invoke('window:close-self'),
    // Native app/tray menu actions ask the renderer to navigate by route path.
    // Listener lives for the entire renderer lifetime.
    onNavigate: (cb: (target: string) => void): void => {
      ipcRenderer.on('renderer:navigate', (_event: IpcRendererEvent, target: string) => {
        cb(target)
      })
    },
    onKeyboardCommand: (cb: (command: AppKeyboardCommand) => void): (() => void) => {
      const listener = (_event: IpcRendererEvent, command: unknown) => {
        if (isAppKeyboardCommand(command)) cb(command)
      }
      ipcRenderer.on(DESKTOP_KEYBOARD_COMMAND_CHANNEL, listener)
      return () => {
        ipcRenderer.removeListener(DESKTOP_KEYBOARD_COMMAND_CHANNEL, listener)
      }
    },
  },
}

export type MemohApi = typeof api

if (process.contextIsolated) {
  try {
    contextBridge.exposeInMainWorld('electron', electronAPI)
    contextBridge.exposeInMainWorld('api', api)
  } catch (error) {
    console.error(error)
  }
} else {
  window.electron = electronAPI
  window.api = api
}
