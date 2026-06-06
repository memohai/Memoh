import { describe, expect, it, vi } from 'vitest'
import { closeFocusedTabOrWindow, CLOSE_CURRENT_WORKSPACE_TAB_CHANNEL } from './window-commands'

function createWindow() {
  return {
    close: vi.fn(),
    isDestroyed: vi.fn(() => false),
    webContents: {
      send: vi.fn(),
    },
  }
}

describe('closeFocusedTabOrWindow', () => {
  it('asks the chat renderer to close the current workspace tab', () => {
    const chatWindow = createWindow()

    const handled = closeFocusedTabOrWindow(chatWindow, chatWindow)

    expect(handled).toBe(true)
    expect(chatWindow.webContents.send).toHaveBeenCalledWith(CLOSE_CURRENT_WORKSPACE_TAB_CHANNEL)
    expect(chatWindow.close).not.toHaveBeenCalled()
  })

  it('closes the focused non-chat window', () => {
    const chatWindow = createWindow()
    const settingsWindow = createWindow()

    const handled = closeFocusedTabOrWindow(chatWindow, settingsWindow)

    expect(handled).toBe(true)
    expect(settingsWindow.close).toHaveBeenCalledOnce()
    expect(settingsWindow.webContents.send).not.toHaveBeenCalled()
  })
})
