import type { KeyboardPlatform } from './keyboard-bindings'

export interface ParsedKeyCombo {
  /** Platform mod key: Command on macOS, Ctrl on Windows/Linux. */
  mod: boolean
  alt: boolean
  shift: boolean
  /** Single-character keys are lowercased; named keys (Escape, ArrowLeft, F1) keep their case. */
  key: string
}

const MODIFIER_KEY_NAMES = new Set([
  'Shift', 'Control', 'Alt', 'Meta',
  'CapsLock', 'NumLock', 'ScrollLock', 'AltGraph',
  'OS', 'Fn', 'FnLock',
])

export function isModifierKey(key: string): boolean {
  return MODIFIER_KEY_NAMES.has(key)
}

function canonicalKey(raw: string): string {
  if (raw === ' ' || raw === 'Space' || raw === 'Spacebar') return ' '
  return raw.length === 1 ? raw.toLowerCase() : raw
}

const MOD_TOKENS = new Set(['mod', 'cmd', 'command', 'ctrl', 'control', 'meta', 'super'])
const ALT_TOKENS = new Set(['alt', 'option', 'opt'])
const SHIFT_TOKENS = new Set(['shift'])

export function parseKeyCombo(input: string): ParsedKeyCombo | null {
  const raw = input.trim()
  if (!raw) return null
  const parts = raw.split('+').map(p => p.trim()).filter(Boolean)
  if (parts.length === 0) return null
  let mod = false
  let alt = false
  let shift = false
  let key: string | null = null
  for (const part of parts) {
    const low = part.toLowerCase()
    if (MOD_TOKENS.has(low)) {
      mod = true
    } else if (ALT_TOKENS.has(low)) {
      alt = true
    } else if (SHIFT_TOKENS.has(low)) {
      shift = true
    } else {
      if (key !== null) return null
      key = canonicalKey(part)
    }
  }
  if (key === null) return null
  return { mod, alt, shift, key }
}

export function formatKeyCombo(combo: ParsedKeyCombo): string {
  const parts: string[] = []
  if (combo.mod) parts.push('Mod')
  if (combo.alt) parts.push('Alt')
  if (combo.shift) parts.push('Shift')
  parts.push(combo.key === ' ' ? 'Space' : combo.key)
  return parts.join('+')
}

export function keyCombosEqual(a: ParsedKeyCombo, b: ParsedKeyCombo): boolean {
  return a.mod === b.mod && a.alt === b.alt && a.shift === b.shift && a.key === b.key
}

export interface KeyboardEventLike {
  key: string
  ctrlKey: boolean
  metaKey: boolean
  altKey: boolean
  shiftKey: boolean
}

export function keyComboFromEvent(event: KeyboardEventLike, isMac: boolean): ParsedKeyCombo | null {
  if (isModifierKey(event.key)) return null
  // On macOS the platform mod is Cmd (metaKey); ctrlKey is a distinct modifier
  // we don't model. Returning a combo with mod=false here would silently
  // downgrade Ctrl+S to plain 's' — once stored, the dispatcher would then
  // match every literal 's' keypress in any input. Reject the capture so the
  // user picks a Cmd/Alt/Shift-based combo instead.
  if (isMac && event.ctrlKey) return null
  return {
    mod: isMac ? event.metaKey : event.ctrlKey,
    alt: event.altKey,
    shift: event.shiftKey,
    key: canonicalKey(event.key),
  }
}

export function comboFromBinding(binding: { key: string; mod?: boolean; alt?: boolean; shift?: boolean }): ParsedKeyCombo {
  return {
    mod: binding.mod ?? false,
    alt: binding.alt ?? false,
    shift: binding.shift ?? false,
    key: canonicalKey(binding.key),
  }
}

const NAMED_KEY_LABELS_DEFAULT: Record<string, string> = {
  Escape: 'Esc',
  ArrowLeft: '←',
  ArrowRight: '→',
  ArrowUp: '↑',
  ArrowDown: '↓',
  ' ': 'Space',
}

const NAMED_KEY_LABELS_MAC: Record<string, string> = {
  ...NAMED_KEY_LABELS_DEFAULT,
  Escape: '⎋',
  Enter: '⏎',
  Backspace: '⌫',
  Delete: '⌦',
  Tab: '⇥',
}

export function displayKeyCombo(combo: ParsedKeyCombo, platform: KeyboardPlatform): string[] {
  const tokens: string[] = []
  if (combo.mod) tokens.push(platform === 'mac' ? '⌘' : 'Ctrl')
  if (combo.alt) tokens.push(platform === 'mac' ? '⌥' : 'Alt')
  if (combo.shift) tokens.push(platform === 'mac' ? '⇧' : 'Shift')
  const labels = platform === 'mac' ? NAMED_KEY_LABELS_MAC : NAMED_KEY_LABELS_DEFAULT
  const labelled = labels[combo.key]
  if (labelled !== undefined) {
    tokens.push(labelled)
  } else if (combo.key.length === 1) {
    tokens.push(combo.key.toUpperCase())
  } else {
    tokens.push(combo.key)
  }
  return tokens
}
