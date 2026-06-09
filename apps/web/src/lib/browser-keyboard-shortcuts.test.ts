import { describe, expect, it, vi } from 'vitest'
import { appKeyboardCommands, type KeyboardCommandRegistry } from './keyboard-commands'
import { handleBrowserKeyboardShortcut, type BrowserKeyboardShortcutBinding } from './browser-keyboard-shortcuts'

function createKeyboardEventLike(options: {
  key: string
  metaKey?: boolean
  ctrlKey?: boolean
  altKey?: boolean
  shiftKey?: boolean
}) {
  return {
    key: options.key,
    metaKey: options.metaKey ?? false,
    ctrlKey: options.ctrlKey ?? false,
    altKey: options.altKey ?? false,
    shiftKey: options.shiftKey ?? false,
    preventDefault: vi.fn(),
  }
}

function createRegistry(handled = true): Pick<KeyboardCommandRegistry, 'dispatch'> {
  return {
    dispatch: vi.fn(() => handled),
  }
}

const saveBinding: BrowserKeyboardShortcutBinding = {
  command: appKeyboardCommands.saveActiveFile,
  key: 's',
  mod: true,
}

describe('browser keyboard shortcuts matcher', () => {
  it('matches a mod binding via Cmd on macOS', () => {
    const registry = createRegistry(true)
    const event = createKeyboardEventLike({ key: 's', metaKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [saveBinding], 'mac')).toBe(true)
    expect(registry.dispatch).toHaveBeenCalledWith(appKeyboardCommands.saveActiveFile)
    expect(event.preventDefault).toHaveBeenCalledOnce()
  })

  it('matches a mod binding via Ctrl on Windows/Linux', () => {
    const registry = createRegistry(true)
    const winEvent = createKeyboardEventLike({ key: 's', ctrlKey: true })
    const linuxEvent = createKeyboardEventLike({ key: 's', ctrlKey: true })

    expect(handleBrowserKeyboardShortcut(winEvent, registry, [saveBinding], 'win')).toBe(true)
    expect(handleBrowserKeyboardShortcut(linuxEvent, registry, [saveBinding], 'linux')).toBe(true)
  })

  it('on macOS, Ctrl does NOT satisfy a mod binding (mod is Cmd, not "meta or ctrl")', () => {
    const registry = createRegistry()
    const event = createKeyboardEventLike({ key: 's', ctrlKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [saveBinding], 'mac')).toBe(false)
    expect(registry.dispatch).not.toHaveBeenCalled()
  })

  it('on Windows/Linux, Meta does NOT satisfy a mod binding', () => {
    const registry = createRegistry()
    const event = createKeyboardEventLike({ key: 's', metaKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [saveBinding], 'linux')).toBe(false)
    expect(registry.dispatch).not.toHaveBeenCalled()
  })

  it('requires the opposite platform key absent (Cmd+Ctrl+S is not Cmd+S)', () => {
    const registry = createRegistry()
    const event = createKeyboardEventLike({ key: 's', metaKey: true, ctrlKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [saveBinding], 'mac')).toBe(false)
    expect(registry.dispatch).not.toHaveBeenCalled()
  })

  it('leaves browser defaults alone when the matched command is unhandled', () => {
    const registry = createRegistry(false)
    const event = createKeyboardEventLike({ key: 's', metaKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [saveBinding], 'mac')).toBe(false)
    expect(registry.dispatch).toHaveBeenCalledWith(appKeyboardCommands.saveActiveFile)
    expect(event.preventDefault).not.toHaveBeenCalled()
  })

  it('does nothing when given no bindings (passthrough exclusion happens at selection)', () => {
    const registry = createRegistry()
    const event = createKeyboardEventLike({ key: 'w', metaKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [], 'mac')).toBe(false)
    expect(registry.dispatch).not.toHaveBeenCalled()
    expect(event.preventDefault).not.toHaveBeenCalled()
  })

  it('has no internal reserved-key list: a mod+w binding passed in is dispatched', () => {
    const registry = createRegistry(true)
    const event = createKeyboardEventLike({ key: 'w', metaKey: true })

    expect(handleBrowserKeyboardShortcut(event, registry, [{
      command: appKeyboardCommands.closeCurrentWorkspaceTab,
      key: 'w',
      mod: true,
    }], 'mac')).toBe(true)
    expect(registry.dispatch).toHaveBeenCalledWith(appKeyboardCommands.closeCurrentWorkspaceTab)
    expect(event.preventDefault).toHaveBeenCalledOnce()
  })

  it('does not match when a required modifier is absent', () => {
    const registry = createRegistry()
    const event = createKeyboardEventLike({ key: 's' })

    expect(handleBrowserKeyboardShortcut(event, registry, [saveBinding], 'mac')).toBe(false)
    expect(registry.dispatch).not.toHaveBeenCalled()
  })

  it('matches the per-platform key override for the active platform', () => {
    const registry = createRegistry(true)
    const binding: BrowserKeyboardShortcutBinding = {
      command: appKeyboardCommands.closeCurrentWorkspaceTab,
      key: 'w',
      win: 'F4',
      mod: true,
    }
    const f4 = createKeyboardEventLike({ key: 'F4', ctrlKey: true })
    const w = createKeyboardEventLike({ key: 'w', ctrlKey: true })

    expect(handleBrowserKeyboardShortcut(f4, registry, [binding], 'win')).toBe(true)
    // On Windows the base 'w' no longer matches because the override took effect.
    expect(handleBrowserKeyboardShortcut(w, createRegistry(), [binding], 'win')).toBe(false)
  })
})
