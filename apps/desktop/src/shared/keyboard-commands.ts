export const DESKTOP_KEYBOARD_COMMAND_CHANNEL = 'desktop:keyboard-command'

export const desktopKeyboardCommands = {
  closeCurrentWorkspaceTab: 'close-current-workspace-tab',
} as const

export type DesktopKeyboardCommand =
  typeof desktopKeyboardCommands[keyof typeof desktopKeyboardCommands]

const desktopKeyboardCommandValues = new Set<string>(Object.values(desktopKeyboardCommands))

export function isDesktopKeyboardCommand(value: unknown): value is DesktopKeyboardCommand {
  return typeof value === 'string' && desktopKeyboardCommandValues.has(value)
}
