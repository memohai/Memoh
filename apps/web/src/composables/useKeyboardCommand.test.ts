import { describe, expect, it, vi } from 'vitest'
import { createApp, effectScope, type App } from 'vue'
import { appKeyboardCommands, createKeyboardCommandRegistry, type KeyboardCommandRegistry } from '@/lib/keyboard-commands'
import { KEYBOARD_REGISTRY, useKeyboardCommand } from './useKeyboardCommand'

// Run a composable inside both an app injection context and an effect scope, the
// same two contexts a component setup provides, so inject() and onScopeDispose()
// both work. Returns the scope so the test can dispose it (simulating unmount).
function runInScope(fn: () => void, registry?: KeyboardCommandRegistry) {
  const app: App = createApp({})
  if (registry) app.provide(KEYBOARD_REGISTRY, registry)
  const scope = effectScope()
  app.runWithContext(() => {
    scope.run(() => fn())
  })
  return scope
}

describe('useKeyboardCommand', () => {
  it('registers the handler so the command dispatches while mounted', () => {
    const registry = createKeyboardCommandRegistry()
    const handler = vi.fn(() => true)

    runInScope(() => useKeyboardCommand(appKeyboardCommands.saveActiveFile, handler), registry)

    expect(registry.dispatch(appKeyboardCommands.saveActiveFile)).toBe(true)
    expect(handler).toHaveBeenCalledOnce()
  })

  it('unregisters the handler when the scope is disposed (unmount)', () => {
    const registry = createKeyboardCommandRegistry()
    const handler = vi.fn(() => true)

    const scope = runInScope(() => useKeyboardCommand(appKeyboardCommands.saveActiveFile, handler), registry)
    scope.stop()

    expect(registry.dispatch(appKeyboardCommands.saveActiveFile)).toBe(false)
    expect(handler).not.toHaveBeenCalled()
  })

  it('is a no-op when no registry has been provided', () => {
    const handler = vi.fn(() => true)
    expect(() =>
      runInScope(() => useKeyboardCommand(appKeyboardCommands.saveActiveFile, handler)),
    ).not.toThrow()
  })
})
