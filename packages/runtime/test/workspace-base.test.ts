import { mkdtemp, rm, stat } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { basename, join } from 'node:path'

import { afterEach, describe, expect, it } from 'vitest'

import {
  ensureWorkspaceBase,
  managedWorkspaceBase,
  normalizeRuntimeServerUrl,
  selectWorkspaceBase,
} from '../src/workspace-base'

const temporaryDirectories: string[] = []
const firstKey = `mrk_${'a'.repeat(64)}`
const secondKey = `mrk_${'b'.repeat(64)}`

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(path => rm(path, { recursive: true, force: true })))
})

describe('managed workspace base', () => {
  it('is stable across equivalent server URL spellings without exposing the key', async () => {
    const home = await temporaryRoot()
    const https = managedWorkspaceBase('https://EXAMPLE.test/api/', firstKey, home)
    const websocket = managedWorkspaceBase('wss://example.test/api', firstKey, home)

    expect(https).toBe(websocket)
    expect(https).not.toContain(firstKey)
    expect(basename(https)).toMatch(/^[0-9a-f]{24}$/)
    expect(normalizeRuntimeServerUrl('ws://EXAMPLE.test:80/api/')).toBe('http://example.test/api')
  })

  it('separates different credentials and server base paths', async () => {
    const home = await temporaryRoot()
    const first = managedWorkspaceBase('https://example.test/api', firstKey, home)

    expect(managedWorkspaceBase('https://example.test/api', secondKey, home)).not.toBe(first)
    expect(managedWorkspaceBase('https://example.test/other', firstKey, home)).not.toBe(first)
  })

  it('prefers an explicit override over the managed default', async () => {
    const home = await temporaryRoot()
    const explicit = join(home, 'explicit')

    expect(selectWorkspaceBase({
      serverUrl: 'https://example.test', key: firstKey, workspaceBase: explicit,
    })).toBe(explicit)
    expect(selectWorkspaceBase({
      serverUrl: 'https://example.test', key: firstKey, homeDirectory: home,
    })).toBe(managedWorkspaceBase('https://example.test', firstKey, home))
  })

  it('creates a missing workspace base recursively with private group-readable mode', async () => {
    const home = await temporaryRoot()
    const target = join(home, 'nested', 'runtime')
    expect(await ensureWorkspaceBase(target)).toBe(target)

    const info = await stat(target)
    expect(info.isDirectory()).toBe(true)
    if (process.platform !== 'win32') {
      expect(info.mode & 0o777).toBe(0o750)
    }
  })
})

async function temporaryRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'memoh-runtime-base-'))
  temporaryDirectories.push(root)
  return root
}
