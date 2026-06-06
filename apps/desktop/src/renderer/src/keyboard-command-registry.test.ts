import { describe, expect, it, vi } from 'vitest'
import { desktopKeyboardCommands } from '../../shared/keyboard-commands'
import { createKeyboardCommandRegistry } from './keyboard-command-registry'

describe('keyboard command registry', () => {
  it('dispatches registered command handlers', () => {
    const registry = createKeyboardCommandRegistry()
    const handler = vi.fn(() => true)

    registry.register(desktopKeyboardCommands.closeCurrentWorkspaceTab, handler)

    expect(registry.dispatch(desktopKeyboardCommands.closeCurrentWorkspaceTab)).toBe(true)
    expect(handler).toHaveBeenCalledOnce()
  })

  it('unregisters command handlers', () => {
    const registry = createKeyboardCommandRegistry()
    const handler = vi.fn(() => true)

    const unregister = registry.register(desktopKeyboardCommands.closeCurrentWorkspaceTab, handler)
    unregister()

    expect(registry.dispatch(desktopKeyboardCommands.closeCurrentWorkspaceTab)).toBe(false)
    expect(handler).not.toHaveBeenCalled()
  })

  it('connects preload command events to the registry', () => {
    const listeners: Array<(command: string) => void> = []
    const unsubscribe = vi.fn()
    const api = {
      onKeyboardCommand: vi.fn((cb: (command: string) => void) => {
        listeners.push(cb)
        return unsubscribe
      }),
    }
    const registry = createKeyboardCommandRegistry()
    const handler = vi.fn(() => true)

    registry.register(desktopKeyboardCommands.closeCurrentWorkspaceTab, handler)
    const disconnect = registry.connect(api)
    const listener = listeners[0]
    if (!listener) throw new Error('keyboard command listener was not registered')
    listener(desktopKeyboardCommands.closeCurrentWorkspaceTab)
    disconnect()

    expect(api.onKeyboardCommand).toHaveBeenCalledOnce()
    expect(handler).toHaveBeenCalledOnce()
    expect(unsubscribe).toHaveBeenCalledOnce()
  })
})
