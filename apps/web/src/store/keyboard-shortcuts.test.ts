import { beforeEach, describe, expect, it } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useKeyboardShortcutsStore } from './keyboard-shortcuts'
import { appKeyboardCommands } from '@/lib/keyboard-commands'
import { comboFromBinding } from '@/lib/keyboard-combo'

class MemoryStorage implements Storage {
  private data = new Map<string, string>()
  get length() { return this.data.size }
  clear() { this.data.clear() }
  getItem(key: string) { return this.data.get(key) ?? null }
  setItem(key: string, value: string) { this.data.set(key, value) }
  removeItem(key: string) { this.data.delete(key) }
  key(index: number) { return Array.from(this.data.keys())[index] ?? null }
}

beforeEach(() => {
  (globalThis as unknown as { localStorage: Storage }).localStorage = new MemoryStorage()
  setActivePinia(createPinia())
})

describe('useKeyboardShortcutsStore', () => {
  it('returns the default table when no overrides exist', () => {
    const store = useKeyboardShortcutsStore()
    const save = store.effectiveBindings.find(b => b.command === appKeyboardCommands.saveActiveFile)
    expect(save).toMatchObject({ key: 's', mod: true })
    expect(store.isOverridden(appKeyboardCommands.saveActiveFile)).toBe(false)
  })

  it('overrides a binding key and reports it as overridden', () => {
    const store = useKeyboardShortcutsStore()
    const result = store.setBinding(appKeyboardCommands.saveActiveFile, 'Mod+Shift+s')

    expect(result.kind).toBe('none')
    expect(store.isOverridden(appKeyboardCommands.saveActiveFile)).toBe(true)
    const save = store.effectiveBindings.find(b => b.command === appKeyboardCommands.saveActiveFile)
    expect(save).toMatchObject({ key: 's', mod: true, shift: true })
  })

  it('persists the canonical combo string in localStorage', () => {
    const store = useKeyboardShortcutsStore()
    store.setBinding(appKeyboardCommands.toggleSidebar, 'mod+shift+B')
    expect(store.overrides[appKeyboardCommands.toggleSidebar]).toBe('Mod+Shift+b')
  })

  it('rejects invalid combo strings without writing an override', () => {
    const store = useKeyboardShortcutsStore()
    expect(store.setBinding(appKeyboardCommands.saveActiveFile, 'Mod+Shift').kind).toBe('invalid')
    expect(store.setBinding(appKeyboardCommands.saveActiveFile, '').kind).toBe('invalid')
    expect(store.isOverridden(appKeyboardCommands.saveActiveFile)).toBe(false)
  })

  it('blocks OS-reserved Mod+W/Q/T/N as reserved', () => {
    const store = useKeyboardShortcutsStore()
    for (const key of ['w', 'q', 't', 'n']) {
      const result = store.setBinding(appKeyboardCommands.saveActiveFile, `Mod+${key}`)
      expect(result.kind, key).toBe('reserved')
    }
    expect(store.isOverridden(appKeyboardCommands.saveActiveFile)).toBe(false)
  })

  it('reserved check ignores combos that include extra modifiers (Mod+Shift+W is fine)', () => {
    const store = useKeyboardShortcutsStore()
    expect(store.detectConflictFromString(appKeyboardCommands.saveActiveFile, 'Mod+Shift+w').kind).toBe('none')
  })

  it('blocks same-scope collisions and reports the colliding command', () => {
    const store = useKeyboardShortcutsStore()
    // saveActiveFile = Mod+s; try to bind toggleSidebar to Mod+s
    const result = store.setBinding(appKeyboardCommands.toggleSidebar, 'Mod+s')
    expect(result.kind).toBe('same-scope')
    expect(result.collidesWith).toBe(appKeyboardCommands.saveActiveFile)
    expect(store.isOverridden(appKeyboardCommands.toggleSidebar)).toBe(false)
  })

  it('treats cross-scope collisions as a soft warning that still applies', () => {
    const store = useKeyboardShortcutsStore()
    // mediaLightboxPrev = ArrowLeft (mediaLightbox scope). Bind global toggleSidebar to ArrowLeft.
    const result = store.setBinding(appKeyboardCommands.toggleSidebar, 'ArrowLeft')
    expect(result.kind).toBe('cross-scope')
    expect(result.collidesWith).toBe(appKeyboardCommands.mediaLightboxPrev)
    expect(store.isOverridden(appKeyboardCommands.toggleSidebar)).toBe(true)
  })

  it('resetBinding removes a single override', () => {
    const store = useKeyboardShortcutsStore()
    store.setBinding(appKeyboardCommands.saveActiveFile, 'Mod+Shift+s')
    store.resetBinding(appKeyboardCommands.saveActiveFile)
    expect(store.isOverridden(appKeyboardCommands.saveActiveFile)).toBe(false)
    const save = store.effectiveBindings.find(b => b.command === appKeyboardCommands.saveActiveFile)
    expect(comboFromBinding(save!)).toEqual({ mod: true, alt: false, shift: false, key: 's' })
  })

  it('resetAll clears every override', () => {
    const store = useKeyboardShortcutsStore()
    store.setBinding(appKeyboardCommands.saveActiveFile, 'Mod+Shift+s')
    store.setBinding(appKeyboardCommands.toggleSidebar, 'Mod+Shift+b')
    store.resetAll()
    expect(Object.keys(store.overrides)).toEqual([])
  })

  it('rebinding to the original default still wipes the override entry', () => {
    const store = useKeyboardShortcutsStore()
    store.setBinding(appKeyboardCommands.saveActiveFile, 'Mod+Shift+s')
    store.resetBinding(appKeyboardCommands.saveActiveFile)
    expect(store.overrides[appKeyboardCommands.saveActiveFile]).toBeUndefined()
  })

  it('overridden command can collide with another override after the swap', () => {
    const store = useKeyboardShortcutsStore()
    store.setBinding(appKeyboardCommands.saveActiveFile, 'Mod+Shift+k')
    const result = store.setBinding(appKeyboardCommands.toggleSidebar, 'Mod+Shift+k')
    expect(result.kind).toBe('same-scope')
    expect(result.collidesWith).toBe(appKeyboardCommands.saveActiveFile)
  })

  it('an override forces browser:intercept so the dispatcher claims the new combo', () => {
    const store = useKeyboardShortcutsStore()
    // closeCurrentWorkspaceTab default is Mod+W (browser passthrough). Rebind to Mod+Shift+k.
    store.setBinding(appKeyboardCommands.closeCurrentWorkspaceTab, 'Mod+Shift+k')
    const binding = store.effectiveBindings.find(b => b.command === appKeyboardCommands.closeCurrentWorkspaceTab)
    expect(binding?.browser).toBe('intercept')
  })

  it('garbage stored overrides do not poison the effective bindings', () => {
    (globalThis as unknown as { localStorage: Storage }).localStorage.setItem(
      'keyboard-shortcuts-overrides',
      JSON.stringify({ [appKeyboardCommands.saveActiveFile]: '!!!' }),
    )
    const store = useKeyboardShortcutsStore()
    const save = store.effectiveBindings.find(b => b.command === appKeyboardCommands.saveActiveFile)
    expect(save).toMatchObject({ key: 's', mod: true })
  })
})
