import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'

// Cross-window query-cache invalidation payload. Mirrors the subset of
// Pinia Colada's `UseQueryEntryFilter` that survives structured-clone
// serialization across the IPC boundary (no functions / predicates).
export interface CrossWindowInvalidatePayload {
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

// Renderer-facing API surface. Keep this intentionally small — it is the
// full security boundary between chromium renderer processes and the
// node-privileged main process.
const api = {
  desktop: {
    getServerStatus: (): Promise<{
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
    // Tell the main process to fan a query-cache invalidation out to every
    // other BrowserWindow. Used by `setupCrossWindowCacheSync` to mirror
    // mutations performed in one renderer onto siblings.
    broadcastInvalidate: (payload: CrossWindowInvalidatePayload): Promise<void> =>
      ipcRenderer.invoke('desktop:broadcast-invalidate', payload),
    // Subscribe to invalidation events forwarded from sibling windows.
    // Listener lives for the entire window lifetime.
    onInvalidate: (cb: (payload: CrossWindowInvalidatePayload) => void): void => {
      ipcRenderer.on('desktop:invalidate', (_event: IpcRendererEvent, payload: CrossWindowInvalidatePayload) => {
        cb(payload)
      })
    },
  },
  window: {
    // Focus (or create) the settings window. When `target` is supplied —
    // e.g. `/settings/bots/<botId>?tab=mcp` resolved by the chat router —
    // the main process forwards it to the settings renderer over the
    // `settings:navigate` channel after the window has finished loading.
    openSettings: (target?: string): Promise<void> =>
      ipcRenderer.invoke('window:open-settings', target),
    closeSelf: (): Promise<void> => ipcRenderer.invoke('window:close-self'),
    // Settings renderer subscribes here to handle in-window navigation
    // requests pushed by the main process (cold-start replay or warm
    // updates). Returns no unsubscribe handle — the listener is meant to
    // live for the entire window lifetime.
    onSettingsNavigate: (cb: (target: string) => void): void => {
      ipcRenderer.on('settings:navigate', (_event: IpcRendererEvent, target: string) => {
        cb(target)
      })
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
