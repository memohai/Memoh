import { contextBridge } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'

// Renderer-facing API surface. Intentionally minimal for now — extend here when
// the desktop shell needs to expose native capabilities (window control, file
// dialogs, deep links, etc.).
const api = {}

if (process.contextIsolated) {
  try {
    contextBridge.exposeInMainWorld('electron', electronAPI)
    contextBridge.exposeInMainWorld('api', api)
  } catch (error) {
    console.error(error)
  }
} else {
  // @ts-expect-error — fall-through for sandbox-less builds
  window.electron = electronAPI
  // @ts-expect-error — fall-through for sandbox-less builds
  window.api = api
}
