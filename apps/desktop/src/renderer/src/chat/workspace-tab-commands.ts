import {
  desktopKeyboardCommands,
  type DesktopKeyboardCommand,
} from '../../../shared/keyboard-commands'
import type { KeyboardCommandRegistry } from '../keyboard-command-registry'

export interface WorkspaceTabCommandStore {
  activeId: string | null
  closeTab(id: string): void
}

export function handleWorkspaceKeyboardCommand(
  command: DesktopKeyboardCommand,
  store: WorkspaceTabCommandStore,
): boolean {
  if (command !== desktopKeyboardCommands.closeCurrentWorkspaceTab) return false
  const activeId = store.activeId
  if (!activeId) return false
  store.closeTab(activeId)
  return true
}

export function registerWorkspaceTabCommands(
  registry: Pick<KeyboardCommandRegistry, 'register'>,
  store: WorkspaceTabCommandStore,
): () => void {
  return registry.register(desktopKeyboardCommands.closeCurrentWorkspaceTab, () =>
    handleWorkspaceKeyboardCommand(desktopKeyboardCommands.closeCurrentWorkspaceTab, store),
  )
}
