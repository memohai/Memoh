import { app, dialog } from 'electron'
import { spawn, spawnSync, type ChildProcess } from 'node:child_process'
import { cpSync, existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { ensureEmbeddedQdrant, getEmbeddedQdrantStatus } from './qdrant'
import {
  appendLineLog,
  isProcessAlive,
  readManagedPid,
  stopManagedPid,
  writeManagedPid,
  type ManagedPid,
} from './daemon'
import { bundledGStreamerEnv } from './gstreamer'
import { desktopResourcePath, desktopServerWorkDir, repoRoot } from './paths'

export const LOCAL_SERVER_PORT = 18731
export const LOCAL_SERVER_BASE_URL = `http://127.0.0.1:${LOCAL_SERVER_PORT}`

let startedProcess: ChildProcess | null = null
let serverReady = false
let serverError: string | null = null
let desktopAuthToken: string | null = null
let preparedServerCommand: ServerCommand | null = null

export interface LocalServerStatus {
  baseUrl: string
  ready: boolean
  managed: boolean
  error?: string
  qdrant?: {
    grpcBaseUrl: string
    httpBaseUrl: string
    ready: boolean
  }
}

interface ServerCommand {
  command: string
  args: string[]
  cwd: string
  configPath: string
  sourceRoot?: string
}

interface ServerIdentity {
  version: string
  commitHash: string
}

interface PingPayload extends ServerIdentity {
  status?: string
}

function serverBinaryName(): string {
  return process.platform === 'win32' ? 'memoh-server.exe' : 'memoh-server'
}

function devServerBinaryPath(cwd: string): string {
  return join(cwd, 'bin', serverBinaryName())
}

function buildDevServerBinary(root: string, cwd: string): string {
  const binary = devServerBinaryPath(cwd)
  mkdirSync(join(cwd, 'bin'), { recursive: true })
  const result = spawnSync('go', ['build', '-o', binary, './cmd/agent'], {
    cwd: root,
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    throw new Error('failed to build local desktop server binary')
  }
  return binary
}

function serverCommand(qdrantGrpcBaseUrl?: string): ServerCommand {
  if (!app.isPackaged) {
    const root = repoRoot()
    const cwd = desktopServerWorkDir()
    return {
      command: buildDevServerBinary(root, cwd),
      args: ['serve'],
      cwd,
      configPath: prepareConfig(
        cwd,
        join(root, 'conf', 'app.local.toml'),
        qdrantGrpcBaseUrl,
        join(root, 'conf', 'providers'),
      ),
      sourceRoot: root,
    }
  }

  const cwd = app.getPath('userData')
  return {
    command: desktopResourcePath('server', serverBinaryName()),
    args: ['serve'],
    cwd,
    configPath: prepareConfig(
      cwd,
      desktopResourcePath('config', 'app.local.toml'),
      qdrantGrpcBaseUrl,
      toAbsoluteConfigPath(cwd, 'conf/providers'),
    ),
  }
}

function currentServerCommand(): ServerCommand {
  if (preparedServerCommand) {
    return preparedServerCommand
  }
  return serverCommand(getEmbeddedQdrantStatus()?.grpcBaseUrl)
}

function logPath(): string {
  return join(app.getPath('userData'), 'local-server.log')
}

function pidPath(): string {
  return join(app.getPath('userData'), 'local-server.pid.json')
}

function appendLog(message: string): void {
  appendLineLog(logPath(), message)
}

function prepareConfig(cwd: string, sourcePath: string, qdrantGrpcBaseUrl: string | undefined, providersDir: string): string {
  mkdirSync(cwd, { recursive: true })
  const home = app.getPath('home')
  const source = readFileSync(sourcePath, 'utf8')
  const contents = applyLocalConfigDefaults(source, cwd, home, providersDir, qdrantGrpcBaseUrl)
  const targetPath = join(cwd, 'config.toml')
  writeFileSync(targetPath, contents, { mode: 0o600 })
  return targetPath
}

function applyLocalConfigDefaults(
  contents: string,
  cwd: string,
  home: string,
  providersDir: string,
  qdrantGrpcBaseUrl?: string,
): string {
  let next = contents.replaceAll('__HOME__', home)
  next = setTomlString(next, 'container', 'data_root', toAbsoluteConfigPath(cwd, 'data/local'))
  next = setTomlString(next, 'container', 'runtime_dir', toAbsoluteConfigPath(cwd, 'data/runtime'))
  next = setTomlString(next, 'local', 'metadata_root', toAbsoluteConfigPath(cwd, 'data/local/containers'))
  next = setTomlString(next, 'sqlite', 'path', toAbsoluteConfigPath(cwd, 'data/local/memoh.db'))
  next = setTomlString(next, 'registry', 'providers_dir', providersDir)
  if (qdrantGrpcBaseUrl) {
    next = setTomlString(next, 'qdrant', 'base_url', qdrantGrpcBaseUrl)
  }
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
    throw new Error(`config key not found: [${sectionName}].${key}`)
  }
  return next.join('\n')
}

function prepareRuntime(command: ServerCommand): void {
  mkdirSync(join(command.cwd, 'data', 'local'), { recursive: true })
  prepareProviders(command.cwd)
  const targetRuntime = join(command.cwd, 'data', 'runtime')
  mkdirSync(targetRuntime, { recursive: true })

  if (!app.isPackaged) {
    const root = command.sourceRoot ?? repoRoot()
    const result = spawnSync('go', ['build', '-o', join(targetRuntime, 'bridge'), './cmd/bridge'], {
      cwd: root,
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
    syncBridgeTemplates(root, targetRuntime)
    return
  }

  const bundledRuntime = desktopResourcePath('runtime')
  if (!existsSync(bundledRuntime)) {
    throw new Error(`Bundled runtime not found: ${bundledRuntime}`)
  }
  rmSync(targetRuntime, { recursive: true, force: true })
  mkdirSync(targetRuntime, { recursive: true })
  cpSync(bundledRuntime, targetRuntime, { recursive: true })
}

function syncBridgeTemplates(root: string, targetRuntime: string): void {
  const templateSource = join(root, 'cmd', 'bridge', 'template')
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
  if (!app.isPackaged) {
    return
  }
  const bundledProviders = desktopResourcePath('providers')
  if (!existsSync(bundledProviders)) {
    throw new Error(`Bundled provider templates not found: ${bundledProviders}`)
  }
  const targetProviders = join(cwd, 'conf', 'providers')
  rmSync(targetProviders, { recursive: true, force: true })
  mkdirSync(targetProviders, { recursive: true })
  cpSync(bundledProviders, targetProviders, { recursive: true })
}

async function probeServer(): Promise<PingPayload | null> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 1000)
  try {
    const response = await fetch(`${LOCAL_SERVER_BASE_URL}/ping`, { signal: controller.signal })
    if (!response.ok) return null
    const payload = await response.json() as { status?: string, version?: string, commit_hash?: string }
    if (payload.status !== 'ok' || typeof payload.version !== 'string') return null
    return {
      status: payload.status,
      version: payload.version,
      commitHash: payload.commit_hash ?? '',
    }
  } catch {
    return null
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

function spawnServer(command: ServerCommand): ChildProcess {
  prepareRuntime(command)
  if (!existsSync(command.command)) {
    throw new Error(`Bundled server binary not found: ${command.command}`)
  }
  runMigrations(command)
  const child = spawn(command.command, command.args, {
    cwd: command.cwd,
    detached: true,
    stdio: 'ignore',
    env: serverEnv(command),
  })
  child.unref()
  child.once('error', (error) => {
    appendLog(`managed local server process error: ${error.message}`)
  })
  child.once('exit', (code, signal) => {
    appendLog(`managed local server exited code=${String(code)} signal=${String(signal)}`)
    if (startedProcess === child) {
      startedProcess = null
      serverReady = false
      desktopAuthToken = null
    }
  })
  if (typeof child.pid === 'number') {
    writeManagedServerPid({
      pid: child.pid,
      command: `${command.command} ${command.args.join(' ')}`,
      startedAt: new Date().toISOString(),
    })
  }
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
  const result = spawnSync(command.command, serverArgs, {
    cwd: command.cwd,
    encoding: 'utf8',
    env: serverEnv(command),
  })
  appendLog(`$ ${command.command} ${serverArgs.join(' ')}\nstatus=${String(result.status)} error=${result.error?.message ?? ''}\nstdout:\n${result.stdout ?? ''}\nstderr:\n${result.stderr ?? ''}`)
  return result
}

function serverEnv(command: { configPath: string }): NodeJS.ProcessEnv {
  return {
    ...process.env,
    ...bundledGStreamerEnv(),
    CONFIG_PATH: command.configPath,
  }
}

function formatCommandFailure(result: ReturnType<typeof spawnSync>): string {
  if (result.error) {
    return result.error.message
  }
  const stderr = String(result.stderr ?? '').trim()
  const stdout = String(result.stdout ?? '').trim()
  return stderr || stdout || `exit status ${String(result.status)}`
}

function bundledServerIdentity(command: { command: string, cwd: string, configPath: string }): ServerIdentity {
  const result = runServerCommand(command, ['version'])
  if (result.status !== 0) {
    throw new Error(`failed to inspect bundled server version: ${formatCommandFailure(result)}`)
  }
  return parseVersionOutput(String(result.stdout ?? ''))
}

function parseVersionOutput(output: string): ServerIdentity {
  const line = output.trim().split(/\r?\n/).find(Boolean) ?? ''
  const match = line.match(/^memoh-server\s+([^\s(]+)(?:\s+\(([^)]+)\))?/)
  if (!match) {
    return { version: '', commitHash: '' }
  }
  return { version: match[1] ?? '', commitHash: match[2] ?? '' }
}

function sameServerIdentity(existing: ServerIdentity, bundled: ServerIdentity): boolean {
  if (bundled.commitHash) {
    return existing.commitHash === bundled.commitHash
  }
  if (bundled.version) {
    return existing.version === bundled.version
  }
  return true
}

function identityLabel(identity: ServerIdentity): string {
  return identity.commitHash ? `${identity.version} (${identity.commitHash})` : identity.version || 'unknown'
}

function writeManagedServerPid(info: ManagedPid): void {
  writeManagedPid(pidPath(), logPath(), info)
}

function readManagedServerPid(): ManagedPid | null {
  return readManagedPid(pidPath())
}

export async function stopManagedServer(): Promise<boolean> {
  const stopped = await stopManagedPid({
    pidPath: pidPath(),
    logPath: logPath(),
    label: 'managed local server',
  })
  if (stopped || !hasLiveManagedServer()) {
    startedProcess = null
    serverReady = false
    desktopAuthToken = null
  }
  return stopped
}

function hasLiveManagedServer(): boolean {
  const info = readManagedServerPid()
  return !!info && isProcessAlive(info.pid)
}

export async function ensureLocalServer(): Promise<LocalServerStatus> {
  try {
    const identityCommand = serverCommand()
    preparedServerCommand = identityCommand
    const bundledIdentity = bundledServerIdentity(identityCommand)
    const existing = await probeServer()
    if (existing) {
      if (hasLiveManagedServer()) {
        appendLog('restarting managed local server to attach embedded Qdrant')
        await stopManagedServer()
      } else {
        const sameIdentity = sameServerIdentity(existing, bundledIdentity)
        throw new Error(`Local server on ${LOCAL_SERVER_BASE_URL} is already running${sameIdentity ? '' : ` (${identityLabel(existing)})`}, but it is not managed by this desktop. Stop it and reopen Memoh so desktop memory uses the embedded Qdrant.`)
      }
    }

    const qdrant = await ensureEmbeddedQdrant()
    const command = serverCommand(qdrant.grpcBaseUrl)
    preparedServerCommand = command
    startedProcess = spawnServer(command)
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
  const qdrant = getEmbeddedQdrantStatus()
  return {
    baseUrl: LOCAL_SERVER_BASE_URL,
    ready: serverReady,
    managed: startedProcess != null || hasLiveManagedServer(),
    error: serverError ?? undefined,
    qdrant: qdrant
      ? {
          grpcBaseUrl: qdrant.grpcBaseUrl,
          httpBaseUrl: qdrant.httpBaseUrl,
          ready: qdrant.ready,
        }
      : undefined,
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
