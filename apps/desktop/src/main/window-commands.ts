export const CLOSE_CURRENT_WORKSPACE_TAB_CHANNEL = 'workspace-tab:close-current'

interface CommandWindow {
  close(): void
  isDestroyed(): boolean
  webContents: {
    send(channel: string): void
  }
}

export function closeFocusedTabOrWindow(
  chatWindow: CommandWindow | null,
  focusedWindow: CommandWindow | null,
): boolean {
  if (!focusedWindow || focusedWindow.isDestroyed()) return false

  if (chatWindow && !chatWindow.isDestroyed() && focusedWindow === chatWindow) {
    focusedWindow.webContents.send(CLOSE_CURRENT_WORKSPACE_TAB_CHANNEL)
    return true
  }

  focusedWindow.close()
  return true
}
