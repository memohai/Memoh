import type { AppKeyboardCommand, KeyboardCommandRegistry } from './keyboard-commands'
import { detectPlatform, resolveBindingKey, type KeyboardPlatform } from './keyboard-bindings'

export interface BrowserKeyboardShortcutEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  shiftKey: boolean
  preventDefault(): void
}

/** Matching-relevant subset of a KeyboardBinding. */
export interface BrowserKeyboardShortcutBinding {
  command: AppKeyboardCommand
  key: string
  mac?: string
  win?: string
  linux?: string
  mod?: boolean
  alt?: boolean
  shift?: boolean
}

interface BrowserKeyboardShortcutTarget {
  addEventListener(type: 'keydown', listener: (event: KeyboardEvent) => void): void
  removeEventListener(type: 'keydown', listener: (event: KeyboardEvent) => void): void
}

function normalizeKey(key: string): string {
  return key.length === 1 ? key.toLowerCase() : key
}

function modifierMatches(actual: boolean, expected = false): boolean {
  return actual === expected
}

// `mod` is the platform command key: Command on macOS, Ctrl on Windows/Linux.
// It is not "meta or ctrl". On macOS Ctrl must be absent for a mod binding
// and vice versa, so Cmd+Ctrl+S does not satisfy a Cmd+S binding.
function modMatches(event: BrowserKeyboardShortcutEvent, wantsMod: boolean | undefined, isMac: boolean): boolean {
  if (!wantsMod) return !event.metaKey && !event.ctrlKey
  return isMac
    ? event.metaKey && !event.ctrlKey
    : event.ctrlKey && !event.metaKey
}

function bindingMatchesEvent(
  binding: BrowserKeyboardShortcutBinding,
  event: BrowserKeyboardShortcutEvent,
  platform: KeyboardPlatform,
): boolean {
  return normalizeKey(event.key) === normalizeKey(resolveBindingKey(binding, platform))
    && modMatches(event, binding.mod, platform === 'mac')
    && modifierMatches(event.altKey, binding.alt)
    && modifierMatches(event.shiftKey, binding.shift)
}

/**
 * Match a keydown against the given bindings and dispatch the first hit. The
   * matcher acts on exactly the bindings it is handed. Deciding which combos are
 * browser-owned (passthrough) is the caller's job, done via selectWebBindings.
 * preventDefault is only called when a handler actually claimed the command, so
 * unhandled keys fall through to the browser/OS.
 */
export function handleBrowserKeyboardShortcut(
  event: BrowserKeyboardShortcutEvent,
  registry: Pick<KeyboardCommandRegistry, 'dispatch'>,
  bindings: BrowserKeyboardShortcutBinding[],
  platform: KeyboardPlatform = detectPlatform(),
): boolean {
  for (const binding of bindings) {
    if (!bindingMatchesEvent(binding, event, platform)) continue
    const handled = registry.dispatch(binding.command)
    if (!handled) return false
    event.preventDefault()
    return true
  }
  return false
}

export function connectBrowserKeyboardShortcuts(
  registry: Pick<KeyboardCommandRegistry, 'dispatch'>,
  bindings: BrowserKeyboardShortcutBinding[],
  target: BrowserKeyboardShortcutTarget | undefined = typeof window === 'undefined' ? undefined : window,
): () => void {
  if (!target || bindings.length === 0) return () => {}
  const platform = detectPlatform()
  const listener = (event: KeyboardEvent) => {
    handleBrowserKeyboardShortcut(event, registry, bindings, platform)
  }
  target.addEventListener('keydown', listener)
  return () => {
    target.removeEventListener('keydown', listener)
  }
}
