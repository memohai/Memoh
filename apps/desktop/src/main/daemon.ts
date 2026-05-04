import { appendFileSync, readFileSync, unlinkSync, writeFileSync } from 'node:fs'

export interface ManagedPid {
  pid: number
  command: string
  startedAt: string
}

export function appendLineLog(logPath: string, message: string): void {
  try {
    appendFileSync(logPath, `[${new Date().toISOString()}] ${message}\n`)
  } catch {
    // Logging must never block startup or shutdown.
  }
}

export function readManagedPid(pidPath: string): ManagedPid | null {
  try {
    const payload = JSON.parse(readFileSync(pidPath, 'utf8')) as Partial<ManagedPid>
    if (typeof payload.pid !== 'number' || payload.pid <= 0) return null
    return {
      pid: payload.pid,
      command: typeof payload.command === 'string' ? payload.command : '',
      startedAt: typeof payload.startedAt === 'string' ? payload.startedAt : '',
    }
  } catch {
    return null
  }
}

export function writeManagedPid(pidPath: string, logPath: string, info: ManagedPid): void {
  try {
    writeFileSync(pidPath, JSON.stringify(info, null, 2), { mode: 0o600 })
  } catch (error) {
    appendLineLog(logPath, `failed to write pid file: ${error instanceof Error ? error.message : String(error)}`)
  }
}

export function isProcessAlive(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

export async function waitForProcessExit(pid: number, timeoutMs = 5_000): Promise<boolean> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < timeoutMs) {
    if (!isProcessAlive(pid)) return true
    await new Promise(resolve => setTimeout(resolve, 200))
  }
  return false
}

export async function stopManagedPid(options: {
  pidPath: string
  logPath: string
  label: string
  timeoutMs?: number
}): Promise<boolean> {
  const info = readManagedPid(options.pidPath)
  if (!info || !isProcessAlive(info.pid)) {
    try {
      unlinkSync(options.pidPath)
    } catch {
      // Stale pid files are harmless.
    }
    return false
  }

  appendLineLog(options.logPath, `stopping ${options.label} pid=${info.pid}`)
  try {
    process.kill(info.pid, 'SIGTERM')
  } catch (error) {
    appendLineLog(options.logPath, `failed to terminate ${options.label}: ${error instanceof Error ? error.message : String(error)}`)
    return false
  }

  if (!(await waitForProcessExit(info.pid, options.timeoutMs))) {
    appendLineLog(options.logPath, `${options.label} did not exit after SIGTERM, killing pid=${info.pid}`)
    try {
      process.kill(info.pid, 'SIGKILL')
    } catch (error) {
      appendLineLog(options.logPath, `failed to kill ${options.label}: ${error instanceof Error ? error.message : String(error)}`)
      return false
    }
    await waitForProcessExit(info.pid, options.timeoutMs)
  }

  try {
    unlinkSync(options.pidPath)
  } catch {
    // Stale pid files are harmless.
  }
  return true
}
