import type { InjectionKey } from 'vue'

export type OpenInFileManager = (path: string, isDir?: boolean) => void

export const openInFileManagerKey: InjectionKey<OpenInFileManager> = Symbol('openInFileManager')

// Opening a message attachment (a stored media asset, not a workspace path) as
// its own dock tab. Container files go through openInFileManager; uploaded
// attachments have no filesystem path, only a content hash / source URL, so they
// open through this channel instead. Provided by the workspace shell; absent when
// the chat renders outside a dock, in which case callers fall back to a download.
export interface OpenAssetPreviewArgs {
  // Stable tab identity: the asset's content hash once persisted, else a hash of
  // the source. Reusing the same key refocuses the existing tab.
  key: string
  // Tab title and the filename used for type/language detection.
  name: string
  // Owner bot for a persisted asset, so the tab can re-resolve a fresh auth URL
  // after a reload instead of holding a stale token.
  botId?: string
  // Preferred, re-resolvable source for a persisted asset.
  contentHash?: string
  // Direct source (data:/http URL) for an attachment that has no content hash yet.
  src?: string
}

export type OpenAssetPreview = (args: OpenAssetPreviewArgs) => void

export const openAssetPreviewKey: InjectionKey<OpenAssetPreview> = Symbol('openAssetPreview')
