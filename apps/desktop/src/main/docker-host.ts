import { spawnSync } from 'node:child_process'
import { existsSync, readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

export interface DockerHostDetectionOptions {
  env?: NodeJS.ProcessEnv
  exists?: (path: string) => boolean
  inspectDockerContext?: () => string
  platform?: NodeJS.Platform
  readDir?: (path: string) => string[]
  readTextFile?: (path: string) => string | undefined
}

export function detectDockerHost(home: string, options: DockerHostDetectionOptions = {}): string {
  const env = options.env ?? process.env
  const platform = options.platform ?? process.platform
  const exists = options.exists ?? existsSync
  const envHost = normalizeDockerHost(env.DOCKER_HOST)
  if (envHost) {
    return envHost
  }

  const contextHost = detectDockerContextHost(home, { ...options, env })
  if (contextHost) {
    return contextHost
  }

  const inspectedHost = normalizeDockerHost((options.inspectDockerContext ?? inspectDockerContextHost)())
  if (inspectedHost) {
    return inspectedHost
  }

  const candidates = dockerSocketCandidates(home, platform, env)
  for (const socketPath of candidates) {
    if (exists(socketPath)) {
      return `unix://${socketPath}`
    }
  }
  return ''
}

function detectDockerContextHost(home: string, options: DockerHostDetectionOptions): string {
  const contextName = normalizeDockerContextName(options.env?.DOCKER_CONTEXT)
    || dockerCurrentContextFromConfig(home, options)
  if (!contextName || contextName === 'default') {
    return ''
  }
  return dockerHostFromContextMetadata(home, contextName, options)
}

function dockerCurrentContextFromConfig(home: string, options: DockerHostDetectionOptions): string {
  const raw = readDockerTextFile(join(dockerConfigDir(home, options), 'config.json'), options)
  if (!raw) {
    return ''
  }
  try {
    const parsed = JSON.parse(raw) as { currentContext?: unknown }
    return normalizeDockerContextName(parsed.currentContext)
  } catch {
    return ''
  }
}

function dockerHostFromContextMetadata(home: string, contextName: string, options: DockerHostDetectionOptions): string {
  const metaRoot = join(dockerConfigDir(home, options), 'contexts', 'meta')
  for (const entry of readDockerDir(metaRoot, options)) {
    const raw = readDockerTextFile(join(metaRoot, entry, 'meta.json'), options)
    if (!raw) {
      continue
    }
    try {
      const parsed = JSON.parse(raw) as {
        Endpoints?: { docker?: { Host?: unknown } }
        Name?: unknown
      }
      if (parsed.Name !== contextName) {
        continue
      }
      return normalizeDockerHost(parsed.Endpoints?.docker?.Host)
    } catch {
      continue
    }
  }
  return ''
}

function dockerConfigDir(home: string, options: DockerHostDetectionOptions): string {
  return options.env?.DOCKER_CONFIG?.trim() || join(home, '.docker')
}

function inspectDockerContextHost(): string {
  const result = spawnSync('docker', ['context', 'inspect', '--format', '{{ .Endpoints.docker.Host }}'], {
    encoding: 'utf8',
    stdio: ['ignore', 'pipe', 'ignore'],
    timeout: 2000,
  })
  if (result.error || result.status !== 0) {
    return ''
  }
  return result.stdout
}

function dockerSocketCandidates(home: string, platform: NodeJS.Platform, env: NodeJS.ProcessEnv): string[] {
  if (platform === 'win32') {
    return []
  }
  if (platform === 'darwin') {
    return [
      '/var/run/docker.sock',
      join(home, '.orbstack', 'run', 'docker.sock'),
      join(home, '.colima', 'default', 'docker.sock'),
      join(home, '.docker', 'run', 'docker.sock'),
    ]
  }
  const candidates = ['/var/run/docker.sock']
  const runtimeDir = env.XDG_RUNTIME_DIR?.trim()
  if (runtimeDir) {
    candidates.push(join(runtimeDir, 'docker.sock'))
  }
  candidates.push(
    join(home, '.docker', 'desktop', 'docker.sock'),
    join(home, '.colima', 'default', 'docker.sock'),
  )
  return candidates
}

function readDockerTextFile(path: string, options: DockerHostDetectionOptions): string | undefined {
  if (options.readTextFile) {
    return options.readTextFile(path)
  }
  try {
    return readFileSync(path, 'utf8')
  } catch {
    return undefined
  }
}

function readDockerDir(path: string, options: DockerHostDetectionOptions): string[] {
  if (options.readDir) {
    return options.readDir(path)
  }
  try {
    return readdirSync(path)
  } catch {
    return []
  }
}

function normalizeDockerContextName(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizeDockerHost(value: unknown): string {
  if (typeof value !== 'string') {
    return ''
  }
  const host = value.split(/\r?\n/).map(line => line.trim()).find(Boolean) ?? ''
  if (!host || host === '<no value>' || host === '<nil>' || host === 'null') {
    return ''
  }
  return host
}
