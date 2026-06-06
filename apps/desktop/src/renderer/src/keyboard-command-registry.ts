import type { DesktopKeyboardCommand } from '../../shared/keyboard-commands'

export type KeyboardCommandHandler = () => boolean | void

export interface KeyboardCommandApi {
  onKeyboardCommand(cb: (command: DesktopKeyboardCommand) => void): (() => void) | void
}

export interface KeyboardCommandRegistry {
  register(command: DesktopKeyboardCommand, handler: KeyboardCommandHandler): () => void
  dispatch(command: DesktopKeyboardCommand): boolean
  connect(api: KeyboardCommandApi): () => void
}

export function createKeyboardCommandRegistry(): KeyboardCommandRegistry {
  const handlers = new Map<DesktopKeyboardCommand, Set<KeyboardCommandHandler>>()

  return {
    register(command, handler) {
      const commandHandlers = handlers.get(command) ?? new Set<KeyboardCommandHandler>()
      commandHandlers.add(handler)
      handlers.set(command, commandHandlers)
      return () => {
        commandHandlers.delete(handler)
        if (commandHandlers.size === 0) handlers.delete(command)
      }
    },

    dispatch(command) {
      const commandHandlers = handlers.get(command)
      if (!commandHandlers) return false
      let handled = false
      for (const handler of commandHandlers) {
        handled = handler() === true || handled
      }
      return handled
    },

    connect(api) {
      const unsubscribe = api.onKeyboardCommand((command) => {
        this.dispatch(command)
      })
      return typeof unsubscribe === 'function' ? unsubscribe : () => {}
    },
  }
}
