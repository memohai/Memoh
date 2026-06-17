export interface FileSaveState {
  readonly: boolean
  isText: boolean
  isDirty: boolean
  saving: boolean
}

/**
 * Whether the file viewer should claim the save-active-file command. When it
 * returns false the command falls through. In the browser that means the native
 * "save page" behavior is left intact, matching the viewer's pre-unification
 * behavior of only intercepting Cmd/Ctrl+S for editable, dirty text files.
 */
export function isFileSaveEligible(state: FileSaveState): boolean {
  return !state.readonly && state.isText && state.isDirty && !state.saving
}
