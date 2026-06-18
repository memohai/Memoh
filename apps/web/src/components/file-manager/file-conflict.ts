// Pure helpers that back the file-viewer's "agent updated this file" chip and
// its save-baseline guard. Extracted so the corner cases (kind detection, line
// counts, save-conflict detection across debounce/load timing) can be tested
// without spinning up a Vue component.

export type FsToolKind = 'write' | 'edit' | 'apply_patch' | 'exec'

export interface FsChangeEventLike {
  at: number
  path?: string
  kind: FsToolKind
  toolCallId?: string
  sessionId?: string
  writeContent?: string
  editOldText?: string
  editNewText?: string
}

export interface ChipContext {
  agentName: string | null
  kind: FsToolKind | null
  occurredAt: number | null
  // For write: total line count of the new content.
  newLineCount: number | null
  // For edit: line counts in the old/new snippet so we can show +N / -M.
  addedLines: number | null
  removedLines: number | null
}

export const EMPTY_CHIP_CONTEXT: ChipContext = {
  agentName: null,
  kind: null,
  occurredAt: null,
  newLineCount: null,
  addedLines: null,
  removedLines: null,
}

function countLines(text: string | undefined | null): number | null {
  if (typeof text !== 'string') return null
  if (text === '') return 0
  return text.split('\n').length
}

export function deriveChipContext(
  event: FsChangeEventLike | null,
  agentName: string | null | undefined,
): ChipContext {
  const name = typeof agentName === 'string' && agentName.trim() !== ''
    ? agentName.trim()
    : null
  if (!event) {
    return { ...EMPTY_CHIP_CONTEXT, agentName: name }
  }
  return {
    agentName: name,
    kind: event.kind,
    occurredAt: event.at,
    newLineCount: countLines(event.writeContent),
    addedLines: countLines(event.editNewText),
    removedLines: countLines(event.editOldText),
  }
}

export interface SaveConflictArgs {
  // chatStore.fsChangedAt — last bumped timestamp (0 if never bumped).
  lastFsChangeAt: number
  // viewer-local: timestamp when the current content was last loaded from
  // server. 0 before the first successful load.
  lastLoadedAt: number
  // chatStore.affectsPath(filePath): whether the latest batch covers this
  // viewer's path (paths set hit OR wildcard).
  affects: boolean
}

// Returns true when the user pressing Save would overwrite a workspace change
// that arrived after we last fetched. Matches VS Code's pre-save mtime check.
export function detectSaveConflict(a: SaveConflictArgs): boolean {
  if (!a.affects) return false
  if (a.lastFsChangeAt <= 0) return false
  if (a.lastLoadedAt <= 0) return false
  return a.lastFsChangeAt > a.lastLoadedAt
}
