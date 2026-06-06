import { describe, expect, it, vi } from 'vitest'
import { closeCurrentWorkspaceTab, registerWorkspaceTabCommands } from './workspace-tab-commands'

describe('workspace tab commands', () => {
  it('closes the active workspace tab', () => {
    const store = {
      activeId: 'terminal:1',
      closeTab: vi.fn(),
    }

    expect(closeCurrentWorkspaceTab(store)).toBe(true)
    expect(store.closeTab).toHaveBeenCalledWith('terminal:1')
  })

  it('leaves the workspace unchanged when there is no active tab', () => {
    const store = {
      activeId: null,
      closeTab: vi.fn(),
    }

    expect(closeCurrentWorkspaceTab(store)).toBe(false)
    expect(store.closeTab).not.toHaveBeenCalled()
  })

  it('registers the desktop close-tab command', () => {
    const listeners: Array<() => void> = []
    const api = {
      onCloseCurrentWorkspaceTab: vi.fn((cb: () => void) => {
        listeners.push(cb)
      }),
    }
    const store = {
      activeId: 'browser:1',
      closeTab: vi.fn(),
    }

    registerWorkspaceTabCommands(api, store)
    const listener = listeners[0]
    if (!listener) throw new Error('close-tab listener was not registered')
    listener()

    expect(api.onCloseCurrentWorkspaceTab).toHaveBeenCalledOnce()
    expect(store.closeTab).toHaveBeenCalledWith('browser:1')
  })
})
