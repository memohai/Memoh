import { describe, expect, it } from 'vitest'
import { isFileSaveEligible } from './file-save-command'

const editableDirty = { readonly: false, isText: true, isDirty: true, saving: false }

describe('isFileSaveEligible', () => {
  it('is eligible for an editable, dirty text file that is not currently saving', () => {
    expect(isFileSaveEligible(editableDirty)).toBe(true)
  })

  it('is not eligible when the file is readonly', () => {
    expect(isFileSaveEligible({ ...editableDirty, readonly: true })).toBe(false)
  })

  it('is not eligible for non-text files', () => {
    expect(isFileSaveEligible({ ...editableDirty, isText: false })).toBe(false)
  })

  it('is not eligible when there are no unsaved changes', () => {
    expect(isFileSaveEligible({ ...editableDirty, isDirty: false })).toBe(false)
  })

  it('is not eligible while a save is already in flight', () => {
    expect(isFileSaveEligible({ ...editableDirty, saving: true })).toBe(false)
  })
})
