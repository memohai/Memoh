import { contextBridge, ipcRenderer } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'

// Renderer-facing API surface. Keep this intentionally small — it is the
// full security boundary between chromium renderer processes and the
// node-privileged main process.
const api = {
  window: {
    openSettings: (): Promise<void> => ipcRenderer.invoke('window:open-settings'),
    closeSelf: (): Promise<void> => ipcRenderer.invoke('window:close-self'),
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
