import { app, dialog } from 'electron'
import { is } from '@electron-toolkit/utils'
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { appendFileSync, cpSync, existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs'
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
      configPath: join(root, 'conf', 'app.local.toml'),
    }
  }

  const cwd = app.getPath('userData')
  const binary = resourcePath('server', serverBinaryName())
  return {
    command: binary,
    args: ['serve'],
    cwd,
    configPath: resourcePath('config', 'app.local.toml'),
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

function prepareRuntime(command: { cwd: string }): void {
  mkdirSync(join(command.cwd, 'data', 'local'), { recursive: true })
  prepareProviders(command.cwd)
  const targetRuntime = join(command.cwd, 'data', 'runtime')
  mkdirSync(targetRuntime, { recursive: true })

  if (is.dev) {
    const result = spawnSync('go', ['build', '-o', join(targetRuntime, 'bridge'), './cmd/bridge'], {
      cwd: command.cwd,
      stdio: 'inherit',
    })
    if (result.status !== 0) {
      throw new Error('failed to build bridge runtime for local desktop server')
    }
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
