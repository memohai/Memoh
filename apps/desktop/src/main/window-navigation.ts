export const RENDERER_NAVIGATE_CHANNEL = 'renderer:navigate'

interface NavigationWindow {
  isDestroyed(): boolean
  webContents: {
    isLoading(): boolean
    once(event: 'did-finish-load', listener: () => void): void
    send(channel: string, target: string): void
  }
}

export function dispatchRendererNavigate(window: NavigationWindow, target: string): void {
  if (window.webContents.isLoading()) {
    window.webContents.once('did-finish-load', () => {
      if (window.isDestroyed()) return
      window.webContents.send(RENDERER_NAVIGATE_CHANNEL, target)
    })
    return
  }
  window.webContents.send(RENDERER_NAVIGATE_CHANNEL, target)
}
