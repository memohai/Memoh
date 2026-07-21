import { chmod, lstat, mkdir, mkdtemp, realpath, rm, stat, symlink, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { status } from '@grpc/grpc-js'
import { afterEach, describe, expect, it } from 'vitest'

import { HostPathResolver } from '../src/core/paths'
import { WorkspaceFileService } from '../src/core/fs'

const temporaryDirectories: string[] = []

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(path => rm(path, { recursive: true, force: true })))
})

describe('HostPathResolver', () => {
  it('uses the home directory for relative, tilde, and /data paths', async () => {
    const home = await temporaryRoot()
    const file = join(home, 'notes', 'hello.txt')
    await mkdir(join(home, 'notes'))
    await writeFile(file, 'hello')
    const resolver = await HostPathResolver.create(home)
    const canonical = await realpath(file)

    await expect(resolver.resolve('notes/hello.txt')).resolves.toBe(canonical)
    await expect(resolver.resolve('~/notes/hello.txt')).resolves.toBe(canonical)
    await expect(resolver.resolve('/data/notes/hello.txt')).resolves.toBe(canonical)
  })

  it('allows absolute paths and symlinks outside the home directory', async () => {
    const home = await temporaryRoot()
    const outside = await temporaryRoot()
    const outsideFile = join(outside, 'outside.txt')
    const link = join(home, 'outside-link')
    await writeFile(outsideFile, 'outside')
    await symlink(outside, link, process.platform === 'win32' ? 'junction' : 'dir')
    const resolver = await HostPathResolver.create(home)
    const canonical = await realpath(outsideFile)

    await expect(resolver.resolve(outsideFile)).resolves.toBe(canonical)
    await expect(resolver.resolve(join(link, 'outside.txt'))).resolves.toBe(canonical)
  })

  it('renames and deletes symlinks without changing their targets', async () => {
    const home = await temporaryRoot()
    const outside = await temporaryRoot()
    const link = join(home, 'outside-link')
    const renamed = join(home, 'renamed-link')
    await writeFile(join(outside, 'keep.txt'), 'outside')
    await symlink(outside, link, process.platform === 'win32' ? 'junction' : 'dir')
    const files = new WorkspaceFileService(await HostPathResolver.create(home))

    await files.rename({ old_path: link, new_path: renamed })
    await expect(lstat(renamed)).resolves.toMatchObject({})
    expect((await lstat(renamed)).isSymbolicLink()).toBe(true)

    await files.deleteFile({ path: renamed, recursive: true })
    await expect(lstat(renamed)).rejects.toMatchObject({ code: 'ENOENT' })
    await expect(stat(join(outside, 'keep.txt'))).resolves.toMatchObject({})
  })

  it('keeps invalid and missing paths as input errors instead of scope errors', async () => {
    const home = await temporaryRoot()
    const resolver = await HostPathResolver.create(home)

    await expect(resolver.resolve('\0')).rejects.toMatchObject({ code: status.INVALID_ARGUMENT })
    await expect(resolver.resolve(join(home, 'missing'))).rejects.toMatchObject({ code: status.NOT_FOUND })
  })

  it('reports host OS permission errors', async () => {
    if (process.platform === 'win32' || process.getuid?.() === 0) {
      return
    }
    const home = await temporaryRoot()
    const locked = join(home, 'locked')
    await mkdir(locked)
    await writeFile(join(locked, 'secret.txt'), 'secret')
    const files = new WorkspaceFileService(await HostPathResolver.create(home))

    await chmod(locked, 0)
    try {
      await expect(files.listDir({ path: locked })).rejects.toMatchObject({ code: status.PERMISSION_DENIED })
    } finally {
      await chmod(locked, 0o700)
    }
  })
})

async function temporaryRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'memoh-runtime-host-paths-'))
  temporaryDirectories.push(root)
  return root
}
