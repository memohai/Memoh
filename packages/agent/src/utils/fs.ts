import type { AuthFetcher } from '../types'

// ---------- types ----------

export interface FSFileInfo {
  name: string
  path: string
  size: number
  mode: string
  modTime: string
  isDir: boolean
}

export interface FSListResponse {
  path: string
  entries: FSFileInfo[]
}

export interface FSReadResponse {
  path: string
  content: string
  size: number
}

export interface FSWriteParams {
  path: string
  content: string
}

export interface FSUploadResponse {
  path: string
  size: number
}

export interface FSMkdirParams {
  path: string
}

export interface FSDeleteParams {
  path: string
  recursive?: boolean
}

export interface FSRenameParams {
  oldPath: string
  newPath: string
}

export interface FSOkResponse {
  ok: boolean
}

export interface FSClientOptions {
  fetch: AuthFetcher
  botId: string
}

// ---------- helpers ----------

const encodeQuery = (path: string) => encodeURIComponent(path)

const ensureOk = async (response: Response, action: string): Promise<void> => {
  if (!response.ok) {
    const text = await response.text().catch(() => '')
    throw new Error(`fs ${action} failed (${response.status}): ${text}`)
  }
}

// ---------- public API ----------

/**
 * Creates a set of filesystem utility functions that operate on a bot's
 * container via the REST file-manager API.
 *
 * All functions use `AuthFetcher` so auth headers are injected automatically.
 */
export const createFS = ({ fetch, botId }: FSClientOptions) => {
  const base = `/bots/${botId}/container/fs`

  /** Get file or directory metadata. */
  const stat = async (path: string): Promise<FSFileInfo> => {
    const response = await fetch(`${base}?path=${encodeQuery(path)}`)
    await ensureOk(response, 'stat')
    return response.json() as Promise<FSFileInfo>
  }

  /** List directory contents. */
  const list = async (path: string): Promise<FSListResponse> => {
    const response = await fetch(`${base}/list?path=${encodeQuery(path)}`)
    await ensureOk(response, 'list')
    return response.json() as Promise<FSListResponse>
  }

  /** Read a file as text. */
  const read = async (path: string): Promise<FSReadResponse> => {
    const response = await fetch(`${base}/read?path=${encodeQuery(path)}`)
    await ensureOk(response, 'read')
    return response.json() as Promise<FSReadResponse>
  }

  /** Download a file as a binary `Response` (stream-ready). */
  const download = async (path: string): Promise<Response> => {
    const response = await fetch(`${base}/download?path=${encodeQuery(path)}`)
    await ensureOk(response, 'download')
    return response
  }

  /** Write text content to a file (creates parent dirs automatically). */
  const write = async (params: FSWriteParams): Promise<FSOkResponse> => {
    const response = await fetch(`${base}/write`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    await ensureOk(response, 'write')
    return response.json() as Promise<FSOkResponse>
  }

  /** Upload a binary file via multipart/form-data. */
  const upload = async (
    path: string,
    file: Blob | File,
    fileName?: string,
  ): Promise<FSUploadResponse> => {
    const form = new FormData()
    form.append('path', path)
    form.append('file', file, fileName ?? (file instanceof File ? file.name : 'upload'))
    const response = await fetch(`${base}/upload`, {
      method: 'POST',
      body: form,
    })
    await ensureOk(response, 'upload')
    return response.json() as Promise<FSUploadResponse>
  }

  /** Create a directory (and parents). */
  const mkdir = async (params: FSMkdirParams): Promise<FSOkResponse> => {
    const response = await fetch(`${base}/mkdir`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    await ensureOk(response, 'mkdir')
    return response.json() as Promise<FSOkResponse>
  }

  /** Delete a file or directory. */
  const remove = async (params: FSDeleteParams): Promise<FSOkResponse> => {
    const response = await fetch(`${base}/delete`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    await ensureOk(response, 'delete')
    return response.json() as Promise<FSOkResponse>
  }

  /** Rename or move a file / directory. */
  const rename = async (params: FSRenameParams): Promise<FSOkResponse> => {
    const response = await fetch(`${base}/rename`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(params),
    })
    await ensureOk(response, 'rename')
    return response.json() as Promise<FSOkResponse>
  }

  /** Check whether a path exists. */
  const exists = async (path: string): Promise<boolean> => {
    try {
      await stat(path)
      return true
    } catch {
      return false
    }
  }

  /** Read a file and return only the text content string. */
  const readText = async (path: string): Promise<string> => {
    const result = await read(path)
    return result.content
  }

  /** Shorthand: write text content to a path. */
  const writeText = async (path: string, content: string): Promise<FSOkResponse> => {
    return write({ path, content })
  }

  return {
    stat,
    list,
    read,
    readText,
    download,
    write,
    writeText,
    upload,
    mkdir,
    remove,
    rename,
    exists,
  }
}

export type FSClient = ReturnType<typeof createFS>

