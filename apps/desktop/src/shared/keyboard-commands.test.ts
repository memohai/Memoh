import { describe, expect, it } from 'vitest'
import {
  DESKTOP_KEYBOARD_COMMAND_CHANNEL,
  appKeyboardCommands,
  isAppKeyboardCommand,
} from './keyboard-commands'

describe('desktop keyboard command transport', () => {
  it('defines a stable desktop IPC channel and reuses app command validation', () => {
    expect(DESKTOP_KEYBOARD_COMMAND_CHANNEL).toBe('desktop:keyboard-command')
    expect(isAppKeyboardCommand(appKeyboardCommands.closeCurrentWorkspaceTab)).toBe(true)
    expect(isAppKeyboardCommand('workspace-tab:close-current')).toBe(false)
    expect(isAppKeyboardCommand(null)).toBe(false)
  })
})
