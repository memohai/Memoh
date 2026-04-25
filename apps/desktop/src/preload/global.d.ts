import type { ElectronAPI } from '@electron-toolkit/preload'
import type { MemohApi } from './index'

declare global {
  interface Window {
    electron: ElectronAPI
    api: MemohApi
  }
}

export {}
