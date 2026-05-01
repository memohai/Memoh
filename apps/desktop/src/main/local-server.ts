import { app, dialog } from 'electron'
import { is } from '@electron-toolkit/utils'
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { cpSync, existsSync, mkdirSync, rmSync } from 'node:fs'
import { join, resolve } from 'node:path'

export const LOCAL_SERVER_PORT = 18731
export const LOCAL_SERVER_BASE_URL = `http://127.0.0.1:${LOCAL_SERVER_PORT}`

let startedProcess: ChildProcess | null = null
let serverReady = false
let serverError: string | null = null

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

function prepareRuntime(command: { cwd: string }): void {
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
    stdio: 'ignore',
    env: {
      ...process.env,
      CONFIG_PATH: command.configPath,
    },
  })
  child.unref()
  return child
}

function runMigrations(command: { command: string, cwd: string, configPath: string }): void {
  const args = is.dev ? ['run', './cmd/agent', 'migrate', 'up'] : ['migrate', 'up']
  const result = spawnSync(command.command, args, {
    cwd: command.cwd,
    stdio: is.dev ? 'inherit' : 'ignore',
    env: {
      ...process.env,
      CONFIG_PATH: command.configPath,
    },
  })
  if (result.status !== 0) {
    throw new Error('local server migration failed')
  }
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
  } catch (error) {
    serverReady = false
    serverError = error instanceof Error ? error.message : String(error)
    dialog.showErrorBox('Memoh server failed to start', serverError)
  }
  return getLocalServerStatus()
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
