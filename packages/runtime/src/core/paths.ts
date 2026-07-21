import { constants } from 'node:fs'
import { access, lstat, mkdir, realpath, stat } from 'node:fs/promises'
import { dirname, isAbsolute, resolve, sep } from 'node:path'

import { status } from '@grpc/grpc-js'

import { mapNodeError, nodeErrorCode, rpcError } from '../rpc'

export interface ResolvePathOptions {
  allowMissing?: boolean
  requireDirectory?: boolean
  followFinalSymlink?: boolean
}

export class HostPathResolver {
  readonly defaultDirectory: string

  private constructor(defaultDirectory: string) {
    this.defaultDirectory = defaultDirectory
  }

  static async create(defaultDirectory: string): Promise<HostPathResolver> {
    if (!defaultDirectory.trim() || !isAbsolute(defaultDirectory)) {
      throw new Error('default directory must be an absolute path')
    }
    const canonical = await realpath(resolve(defaultDirectory))
    const info = await stat(canonical)
    if (!info.isDirectory()) {
      throw new Error('default directory must be a directory')
    }
    return new HostPathResolver(canonical)
  }

  async resolve(requestedPath: string, options: ResolvePathOptions = {}): Promise<string> {
    if (requestedPath.includes('\0')) {
      throw rpcError(status.INVALID_ARGUMENT, 'path contains NUL')
    }
    const value = requestedPath.trim()
    if (!value) {
      throw rpcError(status.INVALID_ARGUMENT, 'path is required')
    }

    const candidate = this.mapBridgePath(value)

    const { ancestor, missingSegments } = await nearestExistingAncestor(candidate)
    if (missingSegments.length > 0 && !options.allowMissing) {
      throw rpcError(status.NOT_FOUND, 'path does not exist')
    }

    let canonicalAncestor = await realpath(ancestor).catch(error => {
      throw mapNodeError(error, 'realpath')
    })
    if (missingSegments.length === 0 && options.followFinalSymlink === false) {
      const info = await lstat(candidate).catch(error => {
        throw mapNodeError(error, 'lstat')
      })
      if (info.isSymbolicLink()) {
        canonicalAncestor = candidate
      }
    }

    const resolvedPath = resolve(canonicalAncestor, ...missingSegments)

    if (missingSegments.length === 0) {
      const target = await stat(resolvedPath).catch(error => {
        throw mapNodeError(error, 'stat')
      })
      if (options.requireDirectory && !target.isDirectory()) {
        throw rpcError(status.INVALID_ARGUMENT, 'path is not a directory')
      }
      return resolvedPath
    }

    if (options.requireDirectory) {
      throw rpcError(status.NOT_FOUND, 'directory does not exist')
    }
    return resolvedPath
  }

  async prepareWrite(requestedPath: string): Promise<string> {
    const initial = await this.resolve(requestedPath, { allowMissing: true })
    await this.ensureDirectory(dirname(initial))
    const checked = await this.resolve(initial, { allowMissing: true })
    if (checked !== initial) {
      throw rpcError(status.PERMISSION_DENIED, 'path changed while it was being checked')
    }
    return checked
  }

  async revalidate(resolvedPath: string, options: ResolvePathOptions = {}): Promise<string> {
    const checked = await this.resolve(resolvedPath, options)
    if (checked !== resolvedPath) {
      throw rpcError(status.PERMISSION_DENIED, 'path changed while it was being checked')
    }
    return checked
  }

  async ensureDirectory(requestedPath: string): Promise<string> {
    const target = await this.resolve(requestedPath, { allowMissing: true })
    try {
      await mkdir(target, { recursive: true, mode: 0o750 })
    } catch (error) {
      if (nodeErrorCode(error) !== 'EEXIST') {
        throw mapNodeError(error, 'mkdir')
      }
    }
    return this.resolve(target, { requireDirectory: true })
  }

  private mapBridgePath(value: string): string {
    const bridgePath = value.replaceAll('\\', '/')
    if (bridgePath === '~' || bridgePath === '/data') {
      return this.defaultDirectory
    }
    if (bridgePath.startsWith('~/')) {
      return resolve(this.defaultDirectory, bridgePath.slice(2))
    }
    if (bridgePath.startsWith('/data/')) {
      return resolve(this.defaultDirectory, bridgePath.slice('/data/'.length))
    }
    if (isAbsolute(value)) {
      return resolve(value)
    }
    return resolve(this.defaultDirectory, value)
  }
}

async function nearestExistingAncestor(candidate: string): Promise<{ ancestor: string, missingSegments: string[] }> {
  const missingSegments: string[] = []
  let current = candidate
  for (;;) {
    try {
      await access(current, constants.F_OK)
      return { ancestor: current, missingSegments }
    } catch (error) {
      if (nodeErrorCode(error) !== 'ENOENT' && nodeErrorCode(error) !== 'ENOTDIR') {
        throw mapNodeError(error, 'access')
      }
      const parent = dirname(current)
      if (parent === current) {
        throw mapNodeError(error, 'access')
      }
      missingSegments.unshift(current.slice(parent.length + (parent.endsWith(sep) ? 0 : 1)))
      current = parent
    }
  }
}
