import {
  DESKTOP_KEYBOARD_COMMAND_CHANNEL,
  desktopKeyboardCommands,
  type DesktopKeyboardCommand,
} from '../shared/keyboard-commands'

interface CommandWindow {
  close(): void
  isDestroyed(): boolean
  webContents: {
    send(channel: string, command: DesktopKeyboardCommand): void
  }
}

export function dispatchFocusedWindowCommand(
  chatWindow: CommandWindow | null,
  focusedWindow: CommandWindow | null,
  command: DesktopKeyboardCommand,
): boolean {
  if (!focusedWindow || focusedWindow.isDestroyed()) return false

  if (chatWindow && !chatWindow.isDestroyed() && focusedWindow === chatWindow) {
    focusedWindow.webContents.send(DESKTOP_KEYBOARD_COMMAND_CHANNEL, command)
    return true
  }

  if (command === desktopKeyboardCommands.closeCurrentWorkspaceTab) {
    focusedWindow.close()
    return true
  }

  return false
}
