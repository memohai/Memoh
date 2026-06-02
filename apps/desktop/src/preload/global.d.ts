import type { ElectronAPI } from '@electron-toolkit/preload'
import type { MemohApi } from './index'

declare global {
  const __MEMOH_DESKTOP_FLAVOR__: 'online' | 'offline'

  interface Window {
    electron: ElectronAPI
    api: MemohApi
  }
}

export {}
