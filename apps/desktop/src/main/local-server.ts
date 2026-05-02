import { app, dialog } from 'electron'
import { is } from '@electron-toolkit/utils'
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { appendFileSync, copyFileSync, cpSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join, resolve } from 'node:path'

export const LOCAL_SERVER_PORT = 18731
export const LOCAL_SERVER_BASE_URL = `http://127.0.0.1:${LOCAL_SERVER_PORT}`

let startedProcess: ChildProcess | null = null
let serverReady = false
let serverError: string | null = null
let desktopAuthToken: string | null = null

export interface LocalServerStatus {
  baseUrl: string
  ready: boolean
  managed: boolean
  error?: string
}

function repoRoot(): string {
  if (is.dev) {
    return resolve(process.cwd(), '..', '..')
  }
  return resolve(app.getAppPath(), '..', '..')
}

function serverBinaryName(): string {
  return process.platform === 'win32' ? 'memoh-server.exe' : 'memoh-server'
}

function resourcePath(...segments: string[]): string {
  return join(process.resourcesPath, ...segments)
}

function serverCommand(): { command: string, args: string[], cwd: string, configPath: string } {
  if (is.dev) {
    const root = repoRoot()
    return {
      command: 'go',
      args: ['run', './cmd/agent', 'serve'],
      cwd: root,
      configPath: prepareConfig(root, join(root, 'conf', 'app.local.toml')),
    }
  }

  const cwd = app.getPath('userData')
  const binary = resourcePath('server', serverBinaryName())
  return {
    command: binary,
    args: ['serve'],
    cwd,
    configPath: prepareConfig(cwd, resourcePath('config', 'app.local.toml')),
  }
}

function currentServerCommand(): { command: string, args: string[], cwd: string, configPath: string } {
  return serverCommand()
}

function logPath(): string {
  return join(app.getPath('userData'), 'local-server.log')
}

function appendLog(message: string): void {
  try {
    appendFileSync(logPath(), `[${new Date().toISOString()}] ${message}\n`)
  } catch {
    // Logging must never block startup.
  }
}

function prepareConfig(cwd: string, sourcePath: string): string {
  mkdirSync(cwd, { recursive: true })
  const targetPath = join(cwd, 'config.toml')
  copyFileSync(sourcePath, targetPath)
  const home = app.getPath('home')
  const contents = applyLocalConfigDefaults(readFileSync(targetPath, 'utf8'), cwd, home)
  writeFileSync(targetPath, contents, { mode: 0o600 })
  return targetPath
}

function applyLocalConfigDefaults(contents: string, cwd: string, home: string): string {
  let next = contents.replaceAll('__HOME__', home)
  next = setTomlString(next, 'container', 'data_root', toAbsoluteConfigPath(cwd, 'data/local'))
  next = setTomlString(next, 'container', 'runtime_dir', toAbsoluteConfigPath(cwd, 'data/runtime'))
  next = setTomlString(next, 'local', 'metadata_root', toAbsoluteConfigPath(cwd, 'data/local/containers'))
  next = setTomlString(next, 'sqlite', 'path', toAbsoluteConfigPath(cwd, 'data/local/memoh.db'))
  next = setTomlString(next, 'registry', 'providers_dir', toAbsoluteConfigPath(cwd, 'conf/providers'))
  return setDockerHostIfEmpty(next, detectDockerHost(home))
}

function toAbsoluteConfigPath(cwd: string, value: string): string {
  if (value.startsWith('/')) {
    return value
  }
  return join(cwd, value)
}

function detectDockerHost(home: string): string {
  const envHost = process.env.DOCKER_HOST?.trim()
  if (envHost) {
    return envHost
  }
  const candidates = [
    join(home, '.docker', 'run', 'docker.sock'),
    '/var/run/docker.sock',
  ]
  for (const socketPath of candidates) {
    if (existsSync(socketPath)) {
      return `unix://${socketPath}`
    }
  }
  return ''
}

function setDockerHostIfEmpty(contents: string, dockerHost: string): string {
  if (!dockerHost) {
    return contents
  }
  const lines = contents.split(/\r?\n/)
  let inDocker = false
  let updated = false
  const next = lines.map((line) => {
    const trimmed = line.trim()
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      inDocker = trimmed === '[docker]'
      return line
    }
    if (!inDocker) {
      return line
    }
    const match = line.match(/^(\s*host\s*=\s*)"([^"]*)"(.*)$/)
    if (!match || match[2].trim() !== '') {
      return line
    }
    updated = true
    return `${match[1]}"${dockerHost}"${match[3]}`
  })
  if (updated) {
    appendLog(`detected Docker host: ${dockerHost}`)
  }
  return next.join('\n')
}

function setTomlString(contents: string, sectionName: string, key: string, value: string): string {
  const lines = contents.split(/\r?\n/)
  let inSection = false
  let updated = false
  const next = lines.map((line) => {
    const trimmed = line.trim()
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      inSection = trimmed === `[${sectionName}]`
      return line
    }
    if (!inSection) {
      return line
    }
    const match = line.match(new RegExp(`^(\\s*${key}\\s*=\\s*)"([^"]*)"(.*)$`))
    if (!match) {
      return line
    }
    updated = true
    return `${match[1]}"${value}"${match[3]}`
  })
  if (!updated) {
    appendLog(`config key not found: [${sectionName}].${key}`)
  }
  return next.join('\n')
}

function prepareRuntime(command: { cwd: string }): void {
  mkdirSync(join(command.cwd, 'data', 'local'), { recursive: true })
  prepareProviders(command.cwd)
  const targetRuntime = join(command.cwd, 'data', 'runtime')
  mkdirSync(targetRuntime, { recursive: true })

  if (is.dev) {
    const result = spawnSync('go', ['build', '-o', join(targetRuntime, 'bridge'), './cmd/bridge'], {
      cwd: command.cwd,
      stdio: 'inherit',
      env: {
        ...process.env,
        GOOS: 'linux',
        GOARCH: dockerBridgeArch(),
      },
    })
    if (result.status !== 0) {
      throw new Error('failed to build bridge runtime for local desktop server')
    }
    syncBridgeTemplates(command.cwd, targetRuntime)
    return
  }

  const bundledRuntime = resourcePath('runtime')
  if (!existsSync(bundledRuntime)) {
    throw new Error(`Bundled runtime not found: ${bundledRuntime}`)
  }
  rmSync(targetRuntime, { recursive: true, force: true })
  mkdirSync(targetRuntime, { recursive: true })
  cpSync(bundledRuntime, targetRuntime, { recursive: true })
}

function syncBridgeTemplates(cwd: string, targetRuntime: string): void {
  const templateSource = join(cwd, 'cmd', 'bridge', 'template')
  const templateTarget = join(targetRuntime, 'templates')
  if (!existsSync(templateSource)) {
    throw new Error(`Bridge templates not found: ${templateSource}`)
  }
  rmSync(templateTarget, { recursive: true, force: true })
  cpSync(templateSource, templateTarget, { recursive: true })
}

function dockerBridgeArch(): string {
  switch (process.arch) {
    case 'arm64':
      return 'arm64'
    case 'x64':
      return 'amd64'
    default:
      return process.arch
  }
}

function prepareProviders(cwd: string): void {
  if (is.dev) {
    return
  }
  const bundledProviders = resourcePath('providers')
  if (!existsSync(bundledProviders)) {
    throw new Error(`Bundled provider templates not found: ${bundledProviders}`)
  }
  const targetProviders = join(cwd, 'conf', 'providers')
  rmSync(targetProviders, { recursive: true, force: true })
  mkdirSync(targetProviders, { recursive: true })
  cpSync(bundledProviders, targetProviders, { recursive: true })
}

async function probeServer(): Promise<boolean> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 1000)
  try {
    const response = await fetch(`${LOCAL_SERVER_BASE_URL}/ping`, { signal: controller.signal })
    if (!response.ok) return false
    const payload = await response.json() as { status?: string, version?: string }
    return payload.status === 'ok' && typeof payload.version === 'string'
  } catch {
    return false
  } finally {
    clearTimeout(timeout)
  }
}

async function waitForServer(timeoutMs = 30_000): Promise<boolean> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    if (await probeServer()) return true
    await new Promise(resolve => setTimeout(resolve, 500))
  }
  return false
}

function spawnServer(): ChildProcess {
  const command = serverCommand()
  prepareRuntime(command)
  if (!is.dev && !existsSync(command.command)) {
    throw new Error(`Bundled server binary not found: ${command.command}`)
  }
  runMigrations(command)
  const child = spawn(command.command, command.args, {
    cwd: command.cwd,
    detached: true,
    stdio: is.dev ? 'ignore' : ['ignore', 'ignore', 'ignore'],
    env: {
      ...process.env,
      CONFIG_PATH: command.configPath,
    },
  })
  child.unref()
  return child
}

function runMigrations(command: { command: string, cwd: string, configPath: string }): void {
  const result = runServerCommand(command, ['migrate', 'up'])
  if (result.status === 0) {
    return
  }
  const output = `${result.stdout ?? ''}\n${result.stderr ?? ''}`
  if (output.includes('Dirty database version 2')) {
    appendLog('repairing dirty database version 2')
    const forceResult = runServerCommand(command, ['migrate', 'force', '2'])
    if (forceResult.status === 0) {
      const retryResult = runServerCommand(command, ['migrate', 'up'])
      if (retryResult.status === 0) {
        return
      }
      throw new Error(`local server migration failed after dirty repair: ${formatCommandFailure(retryResult)}`)
    }
    throw new Error(`local server migration dirty repair failed: ${formatCommandFailure(forceResult)}`)
  }
  throw new Error(`local server migration failed: ${formatCommandFailure(result)}`)
}

function runServerCommand(
  command: { command: string, cwd: string, configPath: string },
  serverArgs: string[],
): ReturnType<typeof spawnSync> {
  const args = is.dev ? ['run', './cmd/agent', ...serverArgs] : serverArgs
  const result = spawnSync(command.command, args, {
    cwd: command.cwd,
    encoding: 'utf8',
    env: {
      ...process.env,
      CONFIG_PATH: command.configPath,
    },
  })
  appendLog(`$ ${command.command} ${args.join(' ')}\nstatus=${String(result.status)} error=${result.error?.message ?? ''}\nstdout:\n${result.stdout ?? ''}\nstderr:\n${result.stderr ?? ''}`)
  return result
}

function formatCommandFailure(result: ReturnType<typeof spawnSync>): string {
  if (result.error) {
    return result.error.message
  }
  const stderr = String(result.stderr ?? '').trim()
  const stdout = String(result.stdout ?? '').trim()
  return stderr || stdout || `exit status ${String(result.status)}`
}

export async function ensureLocalServer(): Promise<LocalServerStatus> {
  if (await probeServer()) {
    serverReady = true
    serverError = null
    return getLocalServerStatus()
  }

  try {
    startedProcess = spawnServer()
    if (!(await waitForServer())) {
      throw new Error(`Local server did not become ready on ${LOCAL_SERVER_BASE_URL}`)
    }
    serverReady = true
    serverError = null
    await ensureDesktopAuthToken()
  } catch (error) {
    serverReady = false
    serverError = error instanceof Error ? error.message : String(error)
    dialog.showErrorBox('Memoh server failed to start', `${serverError}\n\nLog: ${logPath()}`)
  }
  return getLocalServerStatus()
}

export async function getDesktopAuthToken(): Promise<string> {
  if (!serverReady) {
    await ensureLocalServer()
  }
  if (!desktopAuthToken) {
    await ensureDesktopAuthToken()
  }
  return desktopAuthToken ?? ''
}

export function getLocalServerStatus(): LocalServerStatus {
  return {
    baseUrl: LOCAL_SERVER_BASE_URL,
    ready: serverReady,
    managed: startedProcess != null,
    error: serverError ?? undefined,
  }
}

export function defaultWorkspacePath(displayName: string): string {
  const raw = displayName.trim() || 'bot'
  const safe = raw.replace(/[^A-Za-z0-9._-]+/g, '-').replace(/^[.-]+|[.-]+$/g, '') || 'bot'
  return join(app.getPath('home'), '.memoh', 'workspaces', safe)
}

async function ensureDesktopAuthToken(): Promise<void> {
  if (desktopAuthToken) {
    return
  }
  const command = currentServerCommand()
  const admin = readAdminCredentials(command.configPath)
  const response = await fetch(`${LOCAL_SERVER_BASE_URL}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(admin),
  })
  if (!response.ok) {
    const text = await response.text().catch(() => '')
    throw new Error(`desktop auto login failed: HTTP ${response.status} ${text}`)
  }
  const payload = await response.json() as { access_token?: string }
  if (!payload.access_token) {
    throw new Error('desktop auto login failed: response did not include access_token')
  }
  desktopAuthToken = payload.access_token
}

function readAdminCredentials(configPath: string): { username: string, password: string } {
  const raw = readFileSync(configPath, 'utf8')
  let inAdmin = false
  let username = ''
  let password = ''
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (trimmed.startsWith('[') && trimmed.endsWith(']')) {
      inAdmin = trimmed === '[admin]'
      continue
    }
    if (!inAdmin || trimmed === '' || trimmed.startsWith('#')) {
      continue
    }
    const match = trimmed.match(/^([A-Za-z0-9_]+)\s*=\s*"(.*)"\s*$/)
    if (!match) {
      continue
    }
    if (match[1] === 'username') {
      username = match[2]
    }
    if (match[1] === 'password') {
      password = match[2]
    }
  }
  if (!username || !password) {
    throw new Error(`desktop auto login failed: missing [admin] username/password in ${configPath}`)
  }
  return { username, password }
}
