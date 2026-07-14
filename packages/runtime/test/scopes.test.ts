import { mkdir, mkdtemp, realpath, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { Metadata, status } from '@grpc/grpc-js'
import { afterEach, describe, expect, it } from 'vitest'

import { PathGuard } from '../src/core/paths'
import {
  WorkspaceScopeRegistry,
  workspaceIDMetadataKey,
  workspacePathMetadataKey,
} from '../src/core/scopes'

const temporaryDirectories: string[] = []
const workspaceID = '11111111-1111-4111-8111-111111111111'

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(path => rm(path, { recursive: true, force: true })))
})

describe('WorkspaceScopeRegistry', () => {
  it('allows an explicitly shared workspace base path', async () => {
    const root = await temporaryRoot()
    const registry = new WorkspaceScopeRegistry(await PathGuard.create(root))
    const scope = await registry.resolve(scopeMetadata('.'))
    expect(scope.workspaceRoot).toBe(await realpath(root))
  })

  it('keeps relative paths in the mounted workspace while allowing absolute paths inside the runtime base', async () => {
    const root = await temporaryRoot()
    const mountRoot = join(root, 'bots', 'primary')
    const sharedFile = join(root, 'shared.txt')
    const outsideRoot = await temporaryRoot()
    const outsideFile = join(outsideRoot, 'private.txt')
    await mkdir(mountRoot, { recursive: true })
    await Promise.all([
      writeFile(join(mountRoot, 'bot.txt'), 'bot'),
      writeFile(sharedFile, 'shared'),
      writeFile(outsideFile, 'private'),
    ])

    const registry = new WorkspaceScopeRegistry(await PathGuard.create(root))
    const scope = await registry.resolve(scopeMetadata('bots/primary'))
    const canonicalBotFile = await realpath(join(mountRoot, 'bot.txt'))
    const canonicalSharedFile = await realpath(sharedFile)
    const canonicalOutsideFile = await realpath(outsideFile)

    expect(scope.workspaceRoot).toBe(await realpath(mountRoot))
    await expect(scope.resolve('bot.txt')).resolves.toBe(canonicalBotFile)
    await expect(scope.resolve('/data/bot.txt')).resolves.toBe(canonicalBotFile)
    await expect(scope.resolve(canonicalSharedFile)).resolves.toBe(canonicalSharedFile)
    await expect(scope.resolve(canonicalOutsideFile)).rejects.toMatchObject({ code: status.PERMISSION_DENIED })
  })

  it('requires the binary workspace path metadata type', async () => {
    const root = await temporaryRoot()
    const registry = new WorkspaceScopeRegistry(await PathGuard.create(root))
    const wrongType = {
      get(key: string) {
        if (key === workspaceIDMetadataKey) {
          return [workspaceID]
        }
        if (key === workspacePathMetadataKey) {
          return ['shared/project']
        }
        return []
      },
    } as unknown as Metadata

    expect(() => registry.resolve(wrongType)).toThrow(expect.objectContaining({ code: status.PERMISSION_DENIED }))
  })
})

function scopeMetadata(path: string): Metadata {
  const metadata = new Metadata()
  metadata.set(workspaceIDMetadataKey, workspaceID)
  metadata.set(workspacePathMetadataKey, Buffer.from(path, 'utf8'))
  return metadata
}

async function temporaryRoot(): Promise<string> {
  const root = await mkdtemp(join(tmpdir(), 'memoh-runtime-scope-'))
  temporaryDirectories.push(root)
  return root
}
