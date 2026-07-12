import { isAbsolute } from 'node:path'

import { validateRuntimeKey } from './key'

export interface RuntimeClientConfig {
  serverUrl: string
  key: string
  workspaceBase: string
  insecureLocalhost?: boolean
}

export function validateConfig(config: RuntimeClientConfig): void {
  if (!config.serverUrl?.trim()) {
    throw new Error('serverUrl is required')
  }
  if (!config.workspaceBase?.trim() || !isAbsolute(config.workspaceBase)) {
    throw new Error('workspaceBase must be an absolute path')
  }
  let serverUrl: URL
  try {
    serverUrl = new URL(config.serverUrl)
  } catch {
    throw new Error('serverUrl must be a valid absolute URL')
  }
  if (!['http:', 'https:', 'ws:', 'wss:'].includes(serverUrl.protocol)) {
    throw new Error('serverUrl must use http, https, ws, or wss')
  }
  if (serverUrl.username || serverUrl.password) {
    throw new Error('serverUrl must not contain credentials')
  }
  if (serverUrl.search || serverUrl.hash) {
    throw new Error('serverUrl must not contain a query string or fragment')
  }
  if (config.insecureLocalhost !== undefined && typeof config.insecureLocalhost !== 'boolean') {
    throw new Error('insecureLocalhost must be a boolean')
  }
  validateRuntimeKey(config.key)
}
