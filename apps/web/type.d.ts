declare module '@jamescoyle/vue-icon'

interface Window {
  api?: {
    desktop?: {
      openExternalUrl?: (url: string) => Promise<void>
    }
  }
}
