import { describe, expect, it } from 'vitest'
import {
  canApplyExternalReload,
  deriveChipContext,
  detectSaveConflict,
  EMPTY_CHIP_CONTEXT,
  resolveChipButtons,
  resolveSaveBehavior,
  type FsChangeEventLike,
} from './file-conflict'

describe('deriveChipContext', () => {
  it('returns the empty shape (with agentName) when no event is provided', () => {
    expect(deriveChipContext(null, 'Bot')).toEqual({
      ...EMPTY_CHIP_CONTEXT,
      agentName: 'Bot',
    })
  })

  it('trims and normalizes blank agent names to null', () => {
    expect(deriveChipContext(null, '').agentName).toBe(null)
    expect(deriveChipContext(null, '   ').agentName).toBe(null)
    expect(deriveChipContext(null, undefined).agentName).toBe(null)
    expect(deriveChipContext(null, null).agentName).toBe(null)
    expect(deriveChipContext(null, '  My Bot  ').agentName).toBe('My Bot')
  })

  it('reports newLineCount for a write event from its writeContent', () => {
    const event: FsChangeEventLike = {
      at: 1000,
      kind: 'write',
      writeContent: 'line one\nline two\nline three',
    }
    const ctx = deriveChipContext(event, 'Bot')
    expect(ctx.kind).toBe('write')
    expect(ctx.occurredAt).toBe(1000)
    expect(ctx.newLineCount).toBe(3)
    expect(ctx.addedLines).toBe(null)
    expect(ctx.removedLines).toBe(null)
  })

  it('reports a single-line file as newLineCount=1 (no trailing newline)', () => {
    const ctx = deriveChipContext({ at: 1, kind: 'write', writeContent: 'one line' }, 'Bot')
    expect(ctx.newLineCount).toBe(1)
  })

  it('reports newLineCount=0 for an empty write (truncate to empty)', () => {
    const ctx = deriveChipContext({ at: 1, kind: 'write', writeContent: '' }, 'Bot')
    expect(ctx.newLineCount).toBe(0)
  })

  it('reports addedLines/removedLines for an edit event from old/new text', () => {
    const event: FsChangeEventLike = {
      at: 1000,
      kind: 'edit',
      editOldText: 'a\nb',
      editNewText: 'A\nB\nC',
    }
    const ctx = deriveChipContext(event, 'Bot')
    expect(ctx.kind).toBe('edit')
    expect(ctx.addedLines).toBe(3)
    expect(ctx.removedLines).toBe(2)
    expect(ctx.newLineCount).toBe(null)
  })

  it('handles edit events with missing old or new text gracefully', () => {
    const ctx = deriveChipContext({ at: 1, kind: 'edit', editNewText: 'x' }, 'Bot')
    expect(ctx.addedLines).toBe(1)
    expect(ctx.removedLines).toBe(null)
  })
})

describe('detectSaveConflict', () => {
  it('returns false when the latest fs change doesn\'t affect this path', () => {
    expect(detectSaveConflict({
      lastFsChangeAt: 2000,
      lastLoadedAt: 1000,
      affects: false,
    })).toBe(false)
  })

  it('returns false when nothing has ever bumped fsChangedAt', () => {
    expect(detectSaveConflict({
      lastFsChangeAt: 0,
      lastLoadedAt: 1000,
      affects: true,
    })).toBe(false)
  })

  it('returns false when the file has never been loaded (defensive)', () => {
    expect(detectSaveConflict({
      lastFsChangeAt: 2000,
      lastLoadedAt: 0,
      affects: true,
    })).toBe(false)
  })

  it('returns false when the bump happened before our last load', () => {
    expect(detectSaveConflict({
      lastFsChangeAt: 1000,
      lastLoadedAt: 2000,
      affects: true,
    })).toBe(false)
  })

  it('returns false when the bump happened at the same tick as our load', () => {
    expect(detectSaveConflict({
      lastFsChangeAt: 2000,
      lastLoadedAt: 2000,
      affects: true,
    })).toBe(false)
  })

  it('returns true when the bump happened after our last load and affects us', () => {
    expect(detectSaveConflict({
      lastFsChangeAt: 3000,
      lastLoadedAt: 2000,
      affects: true,
    })).toBe(true)
  })
})

describe('canApplyExternalReload', () => {
  it('allows an automatic reload when the clean buffer stayed unchanged while reading', () => {
    expect(canApplyExternalReload({
      contentAtRequestStart: 'base',
      originalContentAtRequestStart: 'base',
      currentContent: 'base',
      currentOriginalContent: 'base',
    })).toBe(true)
  })

  it('blocks an automatic reload when the user typed while the read was pending', () => {
    expect(canApplyExternalReload({
      contentAtRequestStart: 'base',
      originalContentAtRequestStart: 'base',
      currentContent: 'base + user edit',
      currentOriginalContent: 'base',
    })).toBe(false)
  })

  it('blocks an automatic reload when the saved baseline changed while the read was pending', () => {
    expect(canApplyExternalReload({
      contentAtRequestStart: 'base',
      originalContentAtRequestStart: 'base',
      currentContent: 'saved elsewhere',
      currentOriginalContent: 'saved elsewhere',
    })).toBe(false)
  })

  it('allows an explicit user reload even when it overwrites a dirty buffer', () => {
    expect(canApplyExternalReload({
      contentAtRequestStart: 'base',
      originalContentAtRequestStart: 'base',
      currentContent: 'mine',
      currentOriginalContent: 'base',
      force: true,
    })).toBe(true)
  })

  it('refuses auto-reload over a dirty buffer even when no mutation occurred during the read', () => {
    // Steady-dirty case: the buffer was dirty before the read, no concurrent
    // typing happened, and force isn't set. The third clause
    // (currentContent === currentOriginalContent) is the only thing keeping
    // auto-reload from silently discarding the user's edits — pin it.
    expect(canApplyExternalReload({
      contentAtRequestStart: 'base + mine',
      originalContentAtRequestStart: 'base',
      currentContent: 'base + mine',
      currentOriginalContent: 'base',
    })).toBe(false)
  })

  it('allows an explicit user reload even when the saved baseline shifted during the read', () => {
    // A concurrent save (or any out-of-band rewrite) shifts both currentContent
    // and currentOriginalContent off the captured snapshots; force=true
    // short-circuits the equality checks and still applies the new disk read.
    // Pinning this prevents a regression that re-adds a baseline-drift guard
    // before the force short-circuit.
    expect(canApplyExternalReload({
      contentAtRequestStart: 'base',
      originalContentAtRequestStart: 'base',
      currentContent: 'saved elsewhere',
      currentOriginalContent: 'saved elsewhere',
      force: true,
    })).toBe(true)
  })
})

describe('resolveChipButtons', () => {
  it('shows Compare + Reload (no save) when available + clean text', () => {
    const buttons = resolveChipButtons({ diskState: 'available', isText: true, isDirty: false })
    expect(buttons.map(b => b.kind)).toEqual(['compare', 'reload'])
    expect(buttons[1]).toMatchObject({ kind: 'reload', labelKey: 'reload' })
  })

  it('shows Compare + Reload + Save anyway when available + dirty text', () => {
    const buttons = resolveChipButtons({ diskState: 'available', isText: true, isDirty: true })
    expect(buttons.map(b => b.kind)).toEqual(['compare', 'reload', 'forceSave'])
    expect(buttons[2]).toMatchObject({ kind: 'forceSave', labelKey: 'saveAnyway' })
  })

  it('hides Compare for non-text (no diff to render)', () => {
    const buttons = resolveChipButtons({ diskState: 'available', isText: false, isDirty: false })
    expect(buttons.map(b => b.kind)).toEqual(['reload'])
  })

  it('shows only Save to restore when deleted + text (any dirty)', () => {
    const cleanButtons = resolveChipButtons({ diskState: 'deleted', isText: true, isDirty: false })
    expect(cleanButtons).toHaveLength(1)
    expect(cleanButtons[0]).toMatchObject({ kind: 'forceSave', labelKey: 'saveToRestore' })

    const dirtyButtons = resolveChipButtons({ diskState: 'deleted', isText: true, isDirty: true })
    expect(dirtyButtons).toHaveLength(1)
    expect(dirtyButtons[0]).toMatchObject({ kind: 'forceSave', labelKey: 'saveToRestore' })
  })

  it('shows no buttons when deleted + non-text (image — no buffer to write back)', () => {
    expect(resolveChipButtons({ diskState: 'deleted', isText: false, isDirty: false }))
      .toEqual([])
  })

  it('shows Try again + Save anyway when stale + dirty text', () => {
    const buttons = resolveChipButtons({ diskState: 'stale', isText: true, isDirty: true })
    expect(buttons.map(b => b.kind)).toEqual(['reload', 'forceSave'])
    expect(buttons[0]).toMatchObject({ kind: 'reload', labelKey: 'tryAgain' })
    expect(buttons[1]).toMatchObject({ kind: 'forceSave', labelKey: 'saveAnyway' })
  })

  it('shows only Try again when stale + clean (nothing to save)', () => {
    const buttons = resolveChipButtons({ diskState: 'stale', isText: true, isDirty: false })
    expect(buttons.map(b => b.kind)).toEqual(['reload'])
    expect(buttons[0]).toMatchObject({ kind: 'reload', labelKey: 'tryAgain' })
  })

  it('hides Compare in stale state (we don\'t know what is on disk)', () => {
    const buttons = resolveChipButtons({ diskState: 'stale', isText: true, isDirty: false })
    expect(buttons.map(b => b.kind)).not.toContain('compare')
  })

  it('shows only Try again when stale + non-text (no force-save on binaries)', () => {
    const clean = resolveChipButtons({ diskState: 'stale', isText: false, isDirty: false })
    expect(clean.map(b => b.kind)).toEqual(['reload'])
    expect(clean[0]).toMatchObject({ kind: 'reload', labelKey: 'tryAgain' })

    // isDirty on a non-text buffer should not unlock Save anyway — the
    // dirty-flag is essentially unreachable for images, but the contract
    // still has to suppress the action button.
    const dirty = resolveChipButtons({ diskState: 'stale', isText: false, isDirty: true })
    expect(dirty.map(b => b.kind)).toEqual(['reload'])
  })

  it('shows only Reload when available + non-text + dirty (no Save anyway on binaries)', () => {
    const buttons = resolveChipButtons({ diskState: 'available', isText: false, isDirty: true })
    expect(buttons.map(b => b.kind)).toEqual(['reload'])
    expect(buttons[0]).toMatchObject({ kind: 'reload', labelKey: 'reload' })
  })
})

describe('resolveSaveBehavior', () => {
  const base = {
    readonly: false,
    saving: false,
    isDirty: true,
    force: false,
    diskState: 'available' as const,
  }

  it('noop when readonly', () => {
    expect(resolveSaveBehavior({ ...base, readonly: true })).toEqual({
      outcome: 'noop',
      bypassConflictGuard: false,
    })
  })

  it('block when already saving', () => {
    expect(resolveSaveBehavior({ ...base, saving: true })).toEqual({
      outcome: 'block',
      bypassConflictGuard: false,
    })
  })

  it('proceed with guard on the normal dirty-save path', () => {
    expect(resolveSaveBehavior(base)).toEqual({
      outcome: 'proceed',
      bypassConflictGuard: false,
    })
  })

  it('proceed without guard when force=true (explicit Save anyway)', () => {
    expect(resolveSaveBehavior({ ...base, force: true })).toEqual({
      outcome: 'proceed',
      bypassConflictGuard: true,
    })
  })

  it('noop when clean + available + no force (nothing to do)', () => {
    expect(resolveSaveBehavior({ ...base, isDirty: false })).toEqual({
      outcome: 'noop',
      bypassConflictGuard: false,
    })
  })

  it('proceed without guard when diskState=deleted (recreate semantics)', () => {
    // Even with a clean buffer we recreate the file — VS Code's ORPHAN save
    // behavior.
    expect(resolveSaveBehavior({ ...base, isDirty: false, diskState: 'deleted' })).toEqual({
      outcome: 'proceed',
      bypassConflictGuard: true,
    })

    expect(resolveSaveBehavior({ ...base, isDirty: true, diskState: 'deleted' })).toEqual({
      outcome: 'proceed',
      bypassConflictGuard: true,
    })
  })

  it('readonly takes precedence over deleted (cannot write a read-only file even to recreate)', () => {
    expect(resolveSaveBehavior({ ...base, readonly: true, diskState: 'deleted' })).toEqual({
      outcome: 'noop',
      bypassConflictGuard: false,
    })
  })

  it('block takes precedence over deleted (in-flight save still blocks)', () => {
    expect(resolveSaveBehavior({ ...base, saving: true, diskState: 'deleted' })).toEqual({
      outcome: 'block',
      bypassConflictGuard: false,
    })
  })

  it('proceed in stale state when dirty (allow user to push their version)', () => {
    expect(resolveSaveBehavior({ ...base, diskState: 'stale' })).toEqual({
      outcome: 'proceed',
      bypassConflictGuard: false,
    })
    expect(resolveSaveBehavior({ ...base, diskState: 'stale', force: true })).toEqual({
      outcome: 'proceed',
      bypassConflictGuard: true,
    })
  })

  it('noop in stale state when clean (we don\'t recreate, just nothing to do)', () => {
    expect(resolveSaveBehavior({ ...base, isDirty: false, diskState: 'stale' })).toEqual({
      outcome: 'noop',
      bypassConflictGuard: false,
    })
  })
})
