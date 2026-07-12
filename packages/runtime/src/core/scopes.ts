import { status, type Metadata } from '@grpc/grpc-js'
import { TextDecoder } from 'node:util'

import { rpcError } from '../rpc'
import { PathGuard } from './paths'

export const workspaceIDMetadataKey = 'x-memoh-workspace-id'
export const workspacePathMetadataKey = 'x-memoh-workspace-path-bin'

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i
const utf8Decoder = new TextDecoder('utf-8', { fatal: true })

export class WorkspaceScopeRegistry {
  private readonly scopes = new Map<string, Promise<PathGuard>>()

  constructor(private readonly base: PathGuard) {}

  hasScope(metadata: Metadata): boolean {
    return metadata.get(workspaceIDMetadataKey).length > 0
      || metadata.get(workspacePathMetadataKey).length > 0
  }

  resolve(metadata: Metadata): Promise<PathGuard> {
    const workspaceID = singleMetadataString(metadata, workspaceIDMetadataKey)
    const workspacePath = binaryMetadataString(metadata, workspacePathMetadataKey)
    if (!uuidPattern.test(workspaceID) || workspaceID !== workspaceID.toLowerCase()) {
      throw rpcError(status.PERMISSION_DENIED, 'valid workspace scope metadata is required')
    }
    const normalizedPath = normalizeWorkspacePath(workspacePath)
    let scope = this.scopes.get(normalizedPath)
    if (!scope) {
      scope = this.create(normalizedPath)
      this.scopes.set(normalizedPath, scope)
      void scope.catch(() => this.scopes.delete(normalizedPath))
    }
    return scope
  }

  private async create(workspacePath: string): Promise<PathGuard> {
    const root = await this.base.ensureDirectory(workspacePath)
    return PathGuard.create(root, [root])
  }
}

export function normalizeWorkspacePath(value: string): string {
  const path = value.trim()
  if (!path || Buffer.byteLength(path, 'utf8') > 1_024 || path.includes('\0') || path.includes('\\')) {
    throw rpcError(status.PERMISSION_DENIED, 'workspace path metadata is invalid')
  }
  if (path === '.') {
    return path
  }
  if (path.startsWith('/') || path.endsWith('/') || path.includes('//')) {
    throw rpcError(status.PERMISSION_DENIED, 'workspace path metadata must be relative')
  }
  const segments = path.split('/')
  if (segments.some(segment => !segment || segment === '.' || segment === '..')) {
    throw rpcError(status.PERMISSION_DENIED, 'workspace path metadata must not traverse its base')
  }
  return segments.join('/')
}

function singleMetadataString(metadata: Metadata, key: string): string {
  const values = metadata.get(key)
  if (values.length !== 1 || typeof values[0] !== 'string') {
    throw rpcError(status.PERMISSION_DENIED, 'workspace scope metadata is required')
  }
  return values[0]
}

function binaryMetadataString(metadata: Metadata, key: string): string {
  const values = metadata.get(key)
  if (values.length !== 1 || !Buffer.isBuffer(values[0])) {
    throw rpcError(status.PERMISSION_DENIED, 'workspace path metadata is required')
  }
  try {
    return utf8Decoder.decode(values[0])
  } catch {
    throw rpcError(status.PERMISSION_DENIED, 'workspace path metadata is not valid UTF-8')
  }
}
