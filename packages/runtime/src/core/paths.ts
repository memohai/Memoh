import { constants } from 'node:fs'
import { access, lstat, mkdir, realpath, stat } from 'node:fs/promises'
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path'

import { status } from '@grpc/grpc-js'

import { mapNodeError, nodeErrorCode, rpcError } from '../rpc'

interface AllowedRoot {
  configured: string
  canonical: string
}

export interface ResolvePathOptions {
  allowMissing?: boolean
  requireDirectory?: boolean
}

export class PathGuard {
  readonly workspaceRoot: string
  private readonly roots: AllowedRoot[]

  private constructor(workspaceRoot: string, roots: AllowedRoot[]) {
    this.workspaceRoot = workspaceRoot
    this.roots = roots
  }

  static async create(workspaceRoot: string, allowedRoots: readonly string[] = [workspaceRoot]): Promise<PathGuard> {
    if (!workspaceRoot.trim() || !isAbsolute(workspaceRoot)) {
      throw new Error('workspace root must be an absolute path')
    }
    if (allowedRoots.length === 0) {
      throw new Error('at least one allowed root is required')
    }

    const roots: AllowedRoot[] = []
    for (const root of allowedRoots) {
      if (!root.trim() || !isAbsolute(root)) {
        throw new Error('allowed roots must be absolute paths')
      }
      const configured = resolve(root)
      const canonical = await realpath(configured)
      const info = await stat(canonical)
      if (!info.isDirectory()) {
        throw new Error(`allowed root is not a directory: ${root}`)
      }
      roots.push({ configured, canonical })
    }

    const configuredWorkspace = resolve(workspaceRoot)
    const canonicalWorkspace = await realpath(configuredWorkspace)
    if (!roots.some(root => contains(root.canonical, canonicalWorkspace))) {
      throw new Error('workspace root must be contained by an allowed root')
    }
    return new PathGuard(canonicalWorkspace, roots)
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
    const lexicalRoot = this.findLexicalRoot(candidate)
    if (!lexicalRoot) {
      throw rpcError(status.PERMISSION_DENIED, 'path is outside the allowed roots')
    }

    const { ancestor, missingSegments } = await nearestExistingAncestor(candidate)
    if (missingSegments.length > 0 && !options.allowMissing) {
      throw rpcError(status.NOT_FOUND, 'path does not exist')
    }

    await this.assertNoSymlinkComponents(lexicalRoot, ancestor)
    const canonicalAncestor = await realpath(ancestor).catch(error => {
      throw mapNodeError(error, 'realpath')
    })
    const canonicalRoot = this.findCanonicalRoot(canonicalAncestor)
    if (!canonicalRoot) {
      throw rpcError(status.PERMISSION_DENIED, 'path resolves outside the allowed roots')
    }

    const resolvedPath = resolve(canonicalAncestor, ...missingSegments)
    if (!contains(canonicalRoot.canonical, resolvedPath)) {
      throw rpcError(status.PERMISSION_DENIED, 'path resolves outside the allowed roots')
    }

    if (missingSegments.length === 0) {
      const target = await lstat(candidate).catch(error => {
        throw mapNodeError(error, 'lstat')
      })
      if (target.isSymbolicLink()) {
        throw rpcError(status.PERMISSION_DENIED, 'symbolic links are not allowed')
      }
      if (options.requireDirectory && !target.isDirectory()) {
        throw rpcError(status.INVALID_ARGUMENT, 'path is not a directory')
      }
      const canonicalTarget = await realpath(candidate)
      if (!contains(canonicalRoot.canonical, canonicalTarget)) {
        throw rpcError(status.PERMISSION_DENIED, 'path resolves outside the allowed roots')
      }
      return canonicalTarget
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
    const root = this.findCanonicalRoot(target)
    if (!root) {
      throw rpcError(status.PERMISSION_DENIED, 'directory is outside the allowed roots')
    }
    const rel = relative(root.canonical, target)
    let current = root.canonical
    if (!rel) {
      return current
    }
    for (const segment of rel.split(sep)) {
      if (!segment || segment === '.' || segment === '..') {
        throw rpcError(status.PERMISSION_DENIED, 'invalid directory segment')
      }
      current = join(current, segment)
      try {
        await mkdir(current, { mode: 0o750 })
      } catch (error) {
        if (nodeErrorCode(error) !== 'EEXIST') {
          throw mapNodeError(error, 'mkdir')
        }
      }
      const info = await lstat(current).catch(error => {
        throw mapNodeError(error, 'lstat')
      })
      if (info.isSymbolicLink() || !info.isDirectory()) {
        throw rpcError(status.PERMISSION_DENIED, 'directory path contains a symbolic link or non-directory')
      }
      const canonical = await realpath(current)
      if (!contains(root.canonical, canonical)) {
        throw rpcError(status.PERMISSION_DENIED, 'directory resolves outside the allowed roots')
      }
      current = canonical
    }
    return current
  }

  private mapBridgePath(value: string): string {
    const bridgePath = value.replaceAll('\\', '/')
    if (bridgePath === '/' || bridgePath === '/data') {
      return this.workspaceRoot
    }
    if (bridgePath.startsWith('/data/')) {
      const segments = bridgePath.slice('/data/'.length).split('/').filter(Boolean)
      return resolve(this.workspaceRoot, ...segments)
    }
    if (isAbsolute(value)) {
      return resolve(value)
    }
    return resolve(this.workspaceRoot, value)
  }

  private findLexicalRoot(candidate: string): AllowedRoot | undefined {
    return this.roots.find(root => contains(root.configured, candidate) || contains(root.canonical, candidate))
  }

  private findCanonicalRoot(candidate: string): AllowedRoot | undefined {
    return this.roots.find(root => contains(root.canonical, candidate))
  }

  private async assertNoSymlinkComponents(root: AllowedRoot, candidate: string): Promise<void> {
    const base = contains(root.canonical, candidate) ? root.canonical : root.configured
    const rel = relative(base, candidate)
    if (rel.startsWith('..') || isAbsolute(rel)) {
      throw rpcError(status.PERMISSION_DENIED, 'path is outside the allowed roots')
    }
    let current = base
    for (const segment of rel.split(sep).filter(Boolean)) {
      current = join(current, segment)
      try {
        const info = await lstat(current)
        if (info.isSymbolicLink()) {
          throw rpcError(status.PERMISSION_DENIED, 'symbolic links are not allowed')
        }
      } catch (error) {
        if (nodeErrorCode(error) === 'ENOENT') {
          return
        }
        throw error
      }
    }
  }
}

export function contains(root: string, candidate: string): boolean {
  const rel = relative(root, candidate)
  return rel === '' || (!rel.startsWith(`..${sep}`) && rel !== '..' && !isAbsolute(rel))
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
