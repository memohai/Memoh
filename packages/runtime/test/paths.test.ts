import { mkdtemp, mkdir, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { status } from '@grpc/grpc-js'
import { afterEach, describe, expect, it } from 'vitest'

import { PathGuard } from '../src/core/paths'

const temporaryDirectories: string[] = []

afterEach(async () => {
  const { rm } = await import('node:fs/promises')
  await Promise.all(temporaryDirectories.splice(0).map(path => rm(path, { recursive: true, force: true })))
})

describe('PathGuard', () => {
  it('rejects traversal, prefix collisions, and symlink escapes', async () => {
    const parent = await temporaryRoot()
    const root = join(parent, 'work')
    const collision = join(parent, 'work-secret')
    const outside = join(parent, 'outside')
    await mkdir(root)
    await mkdir(collision)
    await mkdir(outside)
    await writeFile(join(collision, 'secret'), 'nope')
    await writeFile(join(outside, 'secret'), 'nope')
    await symlink(outside, join(root, 'escape'), process.platform === 'win32' ? 'junction' : 'dir')
    const guard = await PathGuard.create(root)

    await expect(guard.resolve('/data/../work-secret/secret')).rejects.toMatchObject({ code: status.PERMISSION_DENIED })
    await expect(guard.resolve(join(collision, 'secret'))).rejects.toMatchObject({ code: status.PERMISSION_DENIED })
    await expect(guard.resolve('/data/escape/secret')).rejects.toMatchObject({ code: status.PERMISSION_DENIED })
  })
})

async function temporaryRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'memoh-runtime-paths-'))
  temporaryDirectories.push(root)
  return root
}
