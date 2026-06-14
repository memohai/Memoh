import type { InjectionKey, Ref } from 'vue'
import type { HandlersFsFileInfo } from '@memohai/sdk'

// Shared state + callbacks for the Explorer tree, provided by files-pane (which
// owns the API calls, dialogs and selection) and consumed by the recursive
// file-tree / file-tree-node components — avoids prop-drilling through the
// recursion.
export interface FileTreeContext {
  // Read-only from the tree's side (files-pane owns the writes), so they accept
  // both plain refs and computed refs.
  canWrite: Readonly<Ref<boolean>>
  selectionMode: Readonly<Ref<boolean>>
  // Bumped to force every expanded folder to refetch its children (manual
  // refresh, or the agent mutating the workspace).
  refreshKey: Readonly<Ref<number>>
  // Path the tree should expand to and reveal (deep-link from openFilesAt).
  revealPath: Readonly<Ref<string | null>>
  // Path of the file currently open in the active editor tab (highlighted).
  activePath: Readonly<Ref<string | null>>
  rootPath: string
  listDirectory: (path: string) => Promise<HandlersFsFileInfo[]>
  isSelected: (path: string) => boolean
  toggleSelect: (entry: HandlersFsFileInfo, selected: boolean) => void
  openFile: (entry: HandlersFsFileInfo) => void
  requestDownload: (entry: HandlersFsFileInfo) => void
  requestRename: (entry: HandlersFsFileInfo) => void
  requestDelete: (entry: HandlersFsFileInfo) => void
  requestExtract: (entry: HandlersFsFileInfo) => void
  // Folder-scoped create/upload (toolbar variants target the workspace root).
  requestNewFolder: (entry: HandlersFsFileInfo) => void
  requestUpload: (entry: HandlersFsFileInfo) => void
}

export const FileTreeKey: InjectionKey<FileTreeContext> = Symbol('file-tree')
