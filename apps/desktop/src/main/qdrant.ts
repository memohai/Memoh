import { app } from 'electron'
import { spawn, type ChildProcess } from 'node:child_process'
import { chmodSync, closeSync, existsSync, mkdirSync, openSync, readFileSync, writeFileSync } from 'node:fs'
import { createServer, type AddressInfo, type Server } from 'node:net'
import { join } from 'node:path'
import {
  appendLineLog,
  readManagedPid,
  stopManagedPid,
  writeManagedPid,
} from './daemon'
import { desktopResourcePath } from './paths'

export interface EmbeddedQdrantStatus {
  grpcBaseUrl: string
  httpBaseUrl: string
  ready: boolean
  managed: boolean
  error?: string
}

interface QdrantPorts {
  httpPort: number
  grpcPort: number
}

interface PortReservation {
  port: number
  close: () => Promise<void>
}

let qdrantProcess: ChildProcess | null = null
let qdrantStatus: EmbeddedQdrantStatus | null = null

function qdrantRootPath(): string {
  return join(app.getPath('userData'), 'qdrant')
}

function qdrantStoragePath(): string {
  return join(qdrantRootPath(), 'storage')
}

function qdrantConfigPath(): string {
  return join(qdrantRootPath(), 'config.yaml')
}

function qdrantLogPath(): string {
  return join(qdrantRootPath(), 'qdrant.log')
}

function qdrantPidPath(): string {
  return join(qdrantRootPath(), 'qdrant.pid.json')
}

function qdrantPortsPath(): string {
  return join(qdrantRootPath(), 'ports.json')
}

function appendQdrantLog(message: string): void {
  appendLineLog(qdrantLogPath(), message)
}

function qdrantTarget(): string {
  return `${process.platform}-${process.arch}`
}

function qdrantBinaryName(): string {
  return process.platform === 'win32' ? 'qdrant.exe' : 'qdrant'
}

function bundledQdrantBinaryPath(): string {
  return desktopResourcePath('qdrant', qdrantTarget(), qdrantBinaryName())
}

function validatePorts(value: Partial<QdrantPorts> | null | undefined): QdrantPorts | null {
  if (!value) return null
  const { httpPort, grpcPort } = value
  if (!Number.isInteger(httpPort) || !Number.isInteger(grpcPort)) return null
  if (typeof httpPort !== 'number' || typeof grpcPort !== 'number') return null
  if (httpPort <= 0 || grpcPort <= 0) return null
  if (httpPort > 65535 || grpcPort > 65535) return null
  if (httpPort === grpcPort) return null
  return {
    httpPort,
    grpcPort,
  }
}

function readQdrantPorts(): QdrantPorts | null {
  try {
    return validatePorts(JSON.parse(readFileSync(qdrantPortsPath(), 'utf8')) as Partial<QdrantPorts>)
  } catch {
    return null
  }
}

function writeQdrantPorts(ports: QdrantPorts): void {
  mkdirSync(qdrantRootPath(), { recursive: true })
  writeFileSync(qdrantPortsPath(), `${JSON.stringify(ports, null, 2)}\n`, { mode: 0o600 })
}

function qdrantUrls(ports: QdrantPorts): Pick<EmbeddedQdrantStatus, 'grpcBaseUrl' | 'httpBaseUrl'> {
  return {
    grpcBaseUrl: `http://127.0.0.1:${ports.grpcPort}`,
    httpBaseUrl: `http://127.0.0.1:${ports.httpPort}`,
  }
}

function yamlString(value: string): string {
  return JSON.stringify(value.replaceAll('\\', '/'))
}

function writeQdrantConfig(ports: QdrantPorts): void {
  mkdirSync(qdrantRootPath(), { recursive: true })
  mkdirSync(qdrantStoragePath(), { recursive: true })
  const contents = [
    'storage:',
    `  storage_path: ${yamlString(qdrantStoragePath())}`,
    'service:',
    '  host: "127.0.0.1"',
    `  http_port: ${ports.httpPort}`,
    `  grpc_port: ${ports.grpcPort}`,
    'cluster:',
    '  enabled: false',
    '',
  ].join('\n')
  writeFileSync(qdrantConfigPath(), contents, { mode: 0o600 })
}

function isAddrInUse(error: unknown): boolean {
  return typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    (error as NodeJS.ErrnoException).code === 'EADDRINUSE'
}

function reservePort(port: number): Promise<PortReservation> {
  return new Promise((resolve, reject) => {
    const server = createServer()
    const cleanup = (): void => {
      server.removeAllListeners('error')
      server.removeAllListeners('listening')
    }
    server.once('error', (error) => {
      cleanup()
      reject(error)
    })
    server.once('listening', () => {
      cleanup()
      server.unref()
      const address = server.address() as AddressInfo
      resolve({
        port: address.port,
        close: () => closeServer(server),
      })
    })
    server.listen({ host: '127.0.0.1', port })
  })
}

function closeServer(server: Server): Promise<void> {
  return new Promise((resolve, reject) => {
    server.close((error) => {
      if (error) {
        reject(error)
        return
      }
      resolve()
    })
  })
}

async function reserveQdrantPorts(ports?: QdrantPorts): Promise<{ ports: QdrantPorts, release: () => Promise<void> }> {
  let http: PortReservation | null = null
  let grpc: PortReservation | null = null
  try {
    http = await reservePort(ports?.httpPort ?? 0)
    grpc = await reservePort(ports?.grpcPort ?? 0)
    return {
      ports: {
        httpPort: http.port,
        grpcPort: grpc.port,
      },
      release: async () => {
        await Promise.all([http?.close(), grpc?.close()])
      },
    }
  } catch (error) {
    await Promise.allSettled([http?.close(), grpc?.close()])
    throw error
  }
}

async function probeQdrant(httpBaseUrl: string): Promise<boolean> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 1000)
  try {
    const response = await fetch(`${httpBaseUrl}/healthz`, { signal: controller.signal })
    return response.ok
  } catch {
    return false
  } finally {
    clearTimeout(timeout)
  }
}

async function waitForQdrant(httpBaseUrl: string, child: ChildProcess, timeoutMs = 30_000): Promise<boolean> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    if (await probeQdrant(httpBaseUrl)) return true
    if (child.exitCode !== null || child.signalCode !== null) return false
    await new Promise(resolve => setTimeout(resolve, 300))
  }
  return false
}

function qdrantLogMentionsAddrInUse(): boolean {
  try {
    const tail = readFileSync(qdrantLogPath(), 'utf8').slice(-16_384)
    return /EADDRINUSE|Address already in use|os error 48|os error 98|Only one usage of each socket address/i.test(tail)
  } catch {
    return false
  }
}

function ensureBundledQdrantBinary(): string {
  const binaryPath = bundledQdrantBinaryPath()
  if (!existsSync(binaryPath)) {
    throw new Error(`Bundled Qdrant binary not found for ${qdrantTarget()}: ${binaryPath}. Run pnpm --filter @memohai/desktop prepare:qdrant.`)
  }
  if (process.platform !== 'win32') {
    try {
      chmodSync(binaryPath, 0o755)
    } catch (error) {
      appendQdrantLog(`failed to chmod Qdrant binary: ${error instanceof Error ? error.message : String(error)}`)
    }
  }
  return binaryPath
}

async function spawnQdrant(binaryPath: string, ports: QdrantPorts, releaseReservation: () => Promise<void>): Promise<ChildProcess> {
  let reservationReleased = false
  try {
    writeQdrantConfig(ports)
    appendQdrantLog(`starting embedded Qdrant http=${ports.httpPort} grpc=${ports.grpcPort}`)
    const logFd = openSync(qdrantLogPath(), 'a')
    try {
      await releaseReservation()
      reservationReleased = true
      const child = spawn(binaryPath, ['--config-path', qdrantConfigPath(), '--disable-telemetry'], {
        cwd: qdrantRootPath(),
        detached: true,
        stdio: ['ignore', logFd, logFd],
        env: {
          ...process.env,
          QDRANT__TELEMETRY_DISABLED: 'true',
        },
      })
      child.unref()
      child.once('error', (error) => {
        appendQdrantLog(`embedded Qdrant process error: ${error.message}`)
      })
      child.once('exit', (code, signal) => {
        appendQdrantLog(`embedded Qdrant exited code=${String(code)} signal=${String(signal)}`)
        if (qdrantProcess === child) {
          qdrantProcess = null
          qdrantStatus = qdrantStatus
            ? { ...qdrantStatus, ready: false, managed: false, error: `Qdrant exited code=${String(code)} signal=${String(signal)}` }
            : null
        }
      })
      if (typeof child.pid === 'number') {
        writeManagedPid(qdrantPidPath(), qdrantLogPath(), {
          pid: child.pid,
          command: `${binaryPath} --config-path ${qdrantConfigPath()} --disable-telemetry`,
          startedAt: new Date().toISOString(),
        })
      }
      return child
    } finally {
      closeSync(logFd)
    }
  } catch (error) {
    if (!reservationReleased) {
      await releaseReservation().catch((releaseError: unknown) => {
        appendQdrantLog(`failed to release Qdrant port reservation: ${releaseError instanceof Error ? releaseError.message : String(releaseError)}`)
      })
    }
    throw error
  }
}

async function launchQdrant(binaryPath: string, ports: QdrantPorts, releaseReservation: () => Promise<void>): Promise<EmbeddedQdrantStatus> {
  const urls = qdrantUrls(ports)
  const child = await spawnQdrant(binaryPath, ports, releaseReservation)
  qdrantProcess = child
  qdrantStatus = {
    ...urls,
    ready: false,
    managed: true,
  }
  if (await waitForQdrant(urls.httpBaseUrl, child)) {
    qdrantStatus = {
      ...urls,
      ready: true,
      managed: true,
    }
    return qdrantStatus
  }

  await stopManagedPid({
    pidPath: qdrantPidPath(),
    logPath: qdrantLogPath(),
    label: 'embedded Qdrant',
  })
  qdrantProcess = null
  if (qdrantLogMentionsAddrInUse()) {
    const error = new Error('embedded Qdrant port is already in use') as NodeJS.ErrnoException
    error.code = 'EADDRINUSE'
    throw error
  }
  throw new Error(`Embedded Qdrant did not become ready on ${urls.httpBaseUrl}`)
}

async function launchWithNewPorts(binaryPath: string): Promise<EmbeddedQdrantStatus> {
  let lastError: unknown
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const reservation = await reserveQdrantPorts()
    writeQdrantPorts(reservation.ports)
    try {
      return await launchQdrant(binaryPath, reservation.ports, reservation.release)
    } catch (error) {
      lastError = error
      if (!isAddrInUse(error)) throw error
      appendQdrantLog(`fresh Qdrant ports collided, retrying: ${error instanceof Error ? error.message : String(error)}`)
    }
  }
  throw lastError instanceof Error ? lastError : new Error(String(lastError))
}

async function launchWithPersistedOrNewPorts(binaryPath: string, persistedPorts: QdrantPorts | null): Promise<EmbeddedQdrantStatus> {
  if (!persistedPorts) {
    return launchWithNewPorts(binaryPath)
  }

  let reservation: Awaited<ReturnType<typeof reserveQdrantPorts>>
  try {
    reservation = await reserveQdrantPorts(persistedPorts)
  } catch (error) {
    if (!isAddrInUse(error)) throw error
    appendQdrantLog(`persisted Qdrant ports are busy, selecting new ports: ${error instanceof Error ? error.message : String(error)}`)
    return launchWithNewPorts(binaryPath)
  }

  try {
    return await launchQdrant(binaryPath, persistedPorts, reservation.release)
  } catch (error) {
    if (!isAddrInUse(error)) throw error
    appendQdrantLog(`persisted Qdrant ports collided during start, selecting new ports: ${error instanceof Error ? error.message : String(error)}`)
    return launchWithNewPorts(binaryPath)
  }
}

export async function ensureEmbeddedQdrant(): Promise<EmbeddedQdrantStatus> {
  if (qdrantStatus?.ready) {
    return qdrantStatus
  }

  const binaryPath = ensureBundledQdrantBinary()
  const persistedPorts = readQdrantPorts()
  const urls = persistedPorts ? qdrantUrls(persistedPorts) : null
  const pid = readManagedPid(qdrantPidPath())
  if (pid && urls && await probeQdrant(urls.httpBaseUrl)) {
    qdrantStatus = {
      ...urls,
      ready: true,
      managed: true,
    }
    return qdrantStatus
  }

  if (pid) {
    await stopManagedPid({
      pidPath: qdrantPidPath(),
      logPath: qdrantLogPath(),
      label: 'embedded Qdrant',
    })
  }

  try {
    return await launchWithPersistedOrNewPorts(binaryPath, persistedPorts)
  } catch (error) {
    qdrantStatus = urls
      ? {
          ...urls,
          ready: false,
          managed: false,
          error: error instanceof Error ? error.message : String(error),
        }
      : {
          grpcBaseUrl: '',
          httpBaseUrl: '',
          ready: false,
          managed: false,
          error: error instanceof Error ? error.message : String(error),
        }
    throw error
  }
}

export async function stopEmbeddedQdrant(): Promise<boolean> {
  const stopped = await stopManagedPid({
    pidPath: qdrantPidPath(),
    logPath: qdrantLogPath(),
    label: 'embedded Qdrant',
  })
  qdrantProcess = null
  qdrantStatus = qdrantStatus
    ? { ...qdrantStatus, ready: false, managed: false }
    : null
  return stopped
}

export function getEmbeddedQdrantStatus(): EmbeddedQdrantStatus | null {
  return qdrantStatus
}
