import { describe, expect, it } from 'vitest'
import {
  DESKTOP_KEYBOARD_COMMAND_CHANNEL,
  desktopKeyboardCommands,
  isDesktopKeyboardCommand,
} from './keyboard-commands'

describe('keyboard command protocol', () => {
  it('defines a stable command channel and validates command ids', () => {
    expect(DESKTOP_KEYBOARD_COMMAND_CHANNEL).toBe('desktop:keyboard-command')
    expect(isDesktopKeyboardCommand(desktopKeyboardCommands.closeCurrentWorkspaceTab)).toBe(true)
    expect(isDesktopKeyboardCommand('workspace-tab:close-current')).toBe(false)
    expect(isDesktopKeyboardCommand(null)).toBe(false)
  })
})
