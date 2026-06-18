import { describe, expect, it } from 'vitest'
import {
  deriveChipContext,
  detectSaveConflict,
  EMPTY_CHIP_CONTEXT,
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
