import { createHash } from 'node:crypto'
import { mkdir } from 'node:fs/promises'
import { homedir } from 'node:os'
import { resolve } from 'node:path'

const managedWorkspaceHashLength = 24

export interface WorkspaceBaseSelection {
  serverUrl: string
  key: string
  workspaceBase?: string
  homeDirectory?: string
}

/**
 * Returns a stable, non-secret directory for one server/runtime credential.
 * HTTP and WebSocket spellings of the same endpoint intentionally share it.
 */
export function managedWorkspaceBase(
  serverUrl: string,
  key: string,
  homeDirectory = homedir(),
): string {
  const identity = `${normalizeRuntimeServerUrl(serverUrl)}\0${key.trim()}`
  const digest = createHash('sha256').update(identity, 'utf8').digest('hex').slice(0, managedWorkspaceHashLength)
  return resolve(homeDirectory, '.memoh', 'runtime-workspaces', digest)
}

export function selectWorkspaceBase(selection: WorkspaceBaseSelection): string {
  const explicit = selection.workspaceBase
  if (explicit?.trim()) {
    return resolve(explicit.trim())
  }
  return managedWorkspaceBase(selection.serverUrl, selection.key, selection.homeDirectory)
}

export async function ensureWorkspaceBase(workspaceBase: string): Promise<string> {
  const absolute = resolve(workspaceBase)
  await mkdir(absolute, { recursive: true, mode: 0o750 })
  return absolute
}

export function normalizeRuntimeServerUrl(serverUrl: string): string {
  const url = new URL(serverUrl.trim())
  if (url.protocol === 'ws:') {
    url.protocol = 'http:'
  } else if (url.protocol === 'wss:') {
    url.protocol = 'https:'
  }
  url.pathname = url.pathname.replace(/\/+$/, '') || '/'
  url.search = ''
  url.hash = ''
  return url.href
}
