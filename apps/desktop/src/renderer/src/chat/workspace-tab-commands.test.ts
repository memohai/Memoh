import { describe, expect, it, vi } from 'vitest'
import { desktopKeyboardCommands } from '../../../shared/keyboard-commands'
import { handleWorkspaceKeyboardCommand, registerWorkspaceTabCommands } from './workspace-tab-commands'

describe('workspace tab commands', () => {
  it('closes the active workspace tab', () => {
    const store = {
      activeId: 'terminal:1',
      closeTab: vi.fn(),
    }

    expect(handleWorkspaceKeyboardCommand(desktopKeyboardCommands.closeCurrentWorkspaceTab, store)).toBe(true)
    expect(store.closeTab).toHaveBeenCalledWith('terminal:1')
  })

  it('leaves the workspace unchanged when there is no active tab', () => {
    const store = {
      activeId: null,
      closeTab: vi.fn(),
    }

    expect(handleWorkspaceKeyboardCommand(desktopKeyboardCommands.closeCurrentWorkspaceTab, store)).toBe(false)
    expect(store.closeTab).not.toHaveBeenCalled()
  })

  it('registers the close-tab command with a keyboard registry', () => {
    const handlers = new Map<string, () => boolean>()
    const unregister = vi.fn()
    const registry = {
      register: vi.fn((command: string, handler: () => boolean) => {
        handlers.set(command, handler)
        return unregister
      }),
    }
    const store = {
      activeId: 'browser:1',
      closeTab: vi.fn(),
    }

    const cleanup = registerWorkspaceTabCommands(registry, store)
    const handler = handlers.get(desktopKeyboardCommands.closeCurrentWorkspaceTab)
    if (!handler) throw new Error('close-tab handler was not registered')

    expect(registry.register).toHaveBeenCalledWith(desktopKeyboardCommands.closeCurrentWorkspaceTab, expect.any(Function))
    expect(handler()).toBe(true)
    expect(store.closeTab).toHaveBeenCalledWith('browser:1')
    cleanup()
    expect(unregister).toHaveBeenCalledOnce()
  })
})
