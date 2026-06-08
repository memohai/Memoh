import {
  appKeyboardCommands,
  isAppKeyboardCommand,
  type AppKeyboardCommand,
} from '../../../web/src/lib/keyboard-commands'
import { acceleratorForCommand } from '../../../web/src/lib/keyboard-bindings'

export const DESKTOP_KEYBOARD_COMMAND_CHANNEL = 'desktop:keyboard-command'

export { appKeyboardCommands, isAppKeyboardCommand, acceleratorForCommand }
export type { AppKeyboardCommand }
