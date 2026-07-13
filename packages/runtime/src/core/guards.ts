import { posix, win32 } from 'node:path'

import { status } from '@grpc/grpc-js'

import { rpcError } from '../rpc'

const blockedNames = new Set([
  'NODE_OPTIONS',
  'BASH_ENV',
  'ENV',
  'PATH',
  'SHELL',
  'COMSPEC',
  'SYSTEMROOT',
  'WINDIR',
  'PATHEXT',
  'IFS',
  'MEMOH_RUNTIME_KEY',
])

const inheritedExactNames = new Set([
  'HOME',
  'USER',
  'LOGNAME',
  'TMPDIR',
  'TMP',
  'TEMP',
  'LANG',
  'LANGUAGE',
  'LC_ALL',
  'LC_CTYPE',
  'LC_MESSAGES',
  'TERM',
  'COLORTERM',
  'TZ',
  'USERNAME',
  'USERPROFILE',
  'HOMEDRIVE',
  'HOMEPATH',
  'APPDATA',
  'LOCALAPPDATA',
  'PROGRAMDATA',
  'SYSTEMDRIVE',
  'SYSTEMROOT',
  'WINDIR',
  'COMSPEC',
  'PATHEXT',
])

const validEnvironmentName = /^[A-Za-z_][A-Za-z0-9_]*$/

export function runtimeCapabilities(): Array<'fs' | 'exec' | 'workspace_scope'> {
  return ['fs', 'exec', 'workspace_scope']
}

export interface GuardedEnvironmentOptions {
  clean?: boolean
  unset?: readonly string[]
}

export function guardedEnvironment(
  requested: readonly string[] = [],
  options: GuardedEnvironmentOptions = {},
): NodeJS.ProcessEnv {
  const environment = options.clean ? {} : inheritedEnvironment(process.env)
  unsetEnvironment(environment, options.unset ?? [])
  for (const assignment of requested) {
    const separator = assignment.indexOf('=')
    if (separator <= 0 || assignment.includes('\0')) {
      throw rpcError(status.INVALID_ARGUMENT, 'environment entries must use NAME=value')
    }
    const name = assignment.slice(0, separator)
    const value = assignment.slice(separator + 1)
    if (!validEnvironmentName.test(name)) {
      throw rpcError(status.INVALID_ARGUMENT, `invalid environment variable name: ${name}`)
    }
    assertSafeEnvironmentName(name)
    environment[process.platform === 'win32' ? name.toUpperCase() : name] = value
  }
  return environment
}

function unsetEnvironment(environment: NodeJS.ProcessEnv, requested: readonly string[]): void {
  const exact = new Set<string>()
  const prefixes: string[] = []
  for (const item of requested) {
    const name = item.trim()
    const wildcard = name.endsWith('*')
    const value = wildcard ? name.slice(0, -1) : name
    if (!value || !validEnvironmentName.test(value) || (name.includes('*') && !wildcard)) {
      throw rpcError(status.INVALID_ARGUMENT, `invalid environment variable name: ${item}`)
    }
    if (wildcard) {
      prefixes.push(value)
    } else {
      exact.add(value)
    }
  }
  for (const name of Object.keys(environment)) {
    if (exact.has(name) || prefixes.some(prefix => name.startsWith(prefix))) {
      delete environment[name]
    }
  }
}

export function inheritedEnvironment(
  source: NodeJS.ProcessEnv,
  os: NodeJS.Platform = process.platform,
): NodeJS.ProcessEnv {
  const environment: NodeJS.ProcessEnv = {}
  let inheritedPath: string | undefined
  for (const [name, value] of Object.entries(source)) {
    if (value === undefined || value.includes('\0')) {
      continue
    }
    const normalized = name.toUpperCase()
    if (normalized === 'PATH') {
      inheritedPath = value
      continue
    }
    if (inheritedExactNames.has(normalized) || normalized.startsWith('LC_')) {
      environment[normalized] = value
    }
  }
  environment.PATH = safeInheritedPath(inheritedPath, os)
  environment.SHELL = os === 'win32' ? 'cmd.exe' : '/bin/sh'
  return environment
}

export function assertSafeEnvironmentName(name: string): void {
  const normalized = name.toUpperCase()
  if (
    blockedNames.has(normalized)
    || normalized.startsWith('LD_')
    || normalized.startsWith('DYLD_')
  ) {
    throw rpcError(status.PERMISSION_DENIED, `environment variable ${name} is not allowed`)
  }
}

function safeInheritedPath(value: string | undefined, os: NodeJS.Platform): string {
  if (!value || value.includes('\0')) {
    return defaultPath(os)
  }
  const paths = os === 'win32' ? win32 : posix
  const entries = value
    .split(paths.delimiter)
    .filter(entry => entry.length > 0 && paths.isAbsolute(entry))
  return entries.length > 0 ? [...new Set(entries)].join(paths.delimiter) : defaultPath(os)
}

function defaultPath(os: NodeJS.Platform): string {
  if (os === 'win32') {
    return String.raw`C:\Windows\System32;C:\Windows`
  }
  return os === 'darwin'
    ? '/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin'
    : '/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin'
}
