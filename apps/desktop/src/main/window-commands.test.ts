import { describe, expect, it, vi } from 'vitest'
import { DESKTOP_KEYBOARD_COMMAND_CHANNEL, desktopKeyboardCommands } from '../shared/keyboard-commands'
import { dispatchFocusedWindowCommand } from './window-commands'

function createWindow() {
  return {
    close: vi.fn(),
    isDestroyed: vi.fn(() => false),
    webContents: {
      send: vi.fn(),
    },
  }
}

describe('dispatchFocusedWindowCommand', () => {
  it('dispatches keyboard commands to the chat renderer', () => {
    const chatWindow = createWindow()

    const handled = dispatchFocusedWindowCommand(
      chatWindow,
      chatWindow,
      desktopKeyboardCommands.closeCurrentWorkspaceTab,
    )

    expect(handled).toBe(true)
    expect(chatWindow.webContents.send).toHaveBeenCalledWith(
      DESKTOP_KEYBOARD_COMMAND_CHANNEL,
      desktopKeyboardCommands.closeCurrentWorkspaceTab,
    )
    expect(chatWindow.close).not.toHaveBeenCalled()
  })

  it('keeps the close-tab command mapped to native close for non-chat windows', () => {
    const chatWindow = createWindow()
    const settingsWindow = createWindow()

    const handled = dispatchFocusedWindowCommand(
      chatWindow,
      settingsWindow,
      desktopKeyboardCommands.closeCurrentWorkspaceTab,
    )

    expect(handled).toBe(true)
    expect(settingsWindow.close).toHaveBeenCalledOnce()
    expect(settingsWindow.webContents.send).not.toHaveBeenCalled()
  })
})
