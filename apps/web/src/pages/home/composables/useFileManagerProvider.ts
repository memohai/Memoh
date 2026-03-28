import type { InjectionKey } from 'vue'

export type OpenInFileManager = (path: string, isDir?: boolean) => void

export const openInFileManagerKey: InjectionKey<OpenInFileManager> = Symbol('openInFileManager')
