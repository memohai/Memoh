import { execFile, type ChildProcess } from 'node:child_process'

import { windowsSystemExecutable } from './core/shell'

interface LiveChild {
  process: ChildProcess
  pgid: number
}

export interface ChildSupervisorOptions {
  warn?: (message: string) => void
}

/** Tracks only processes started by one live Runtime connection. */
export class ChildSupervisor {
  private readonly warn: (message: string) => void
  private readonly live = new Map<number, LiveChild>()
  private readonly groupMonitors = new Map<number, NodeJS.Timeout>()
  private closing = false
  private closePromise: Promise<void> | undefined

  constructor(options: ChildSupervisorOptions = {}) {
    this.warn = options.warn ?? (() => undefined)
  }

  async register(child: ChildProcess): Promise<void> {
    if (!child.pid) {
      throw new Error('cannot register a child without a pid')
    }
    const pid = child.pid
    const record = { process: child, pgid: pid }
    this.live.set(pid, record)
    child.once('exit', () => this.handleChildExit(pid))
    if (this.closing) {
      await this.terminate(child)
      throw new Error('runtime child supervisor is closing')
    }
  }

  async terminate(child: ChildProcess): Promise<void> {
    if (!child.pid) {
      return
    }
    const record = this.live.get(child.pid)
    if (!record || record.process !== child) {
      return
    }
    if (process.platform === 'win32') {
      try {
        await terminateWindowsProcessTree(record.pgid)
      } catch (error) {
        if (processExists(record.pgid)) {
          this.warn(`failed to terminate runtime child ${child.pid}: ${safeError(error)}`)
          return
        }
      }
      if (!await waitForChildExit(child, 2_000)) {
        this.warn(`runtime child ${child.pid} did not exit after taskkill`)
        return
      }
      this.unregister(child.pid)
      return
    }
    try {
      killGroup(record.pgid)
    } catch (error) {
      this.warn(`failed to terminate runtime child ${child.pid}: ${safeError(error)}`)
      return
    }
    if (!await waitForProcessGroupExit(record.pgid, 2_000)) {
      this.warn(`runtime child ${child.pid} did not exit after SIGKILL`)
      return
    }
    this.unregister(child.pid)
  }

  close(): Promise<void> {
    this.closing = true
    this.closePromise ??= Promise.allSettled(
      [...this.live.values()].map(record => this.terminate(record.process)),
    ).then(() => undefined)
    return this.closePromise
  }

  private handleChildExit(pid: number): void {
    const record = this.live.get(pid)
    if (process.platform === 'win32') {
      this.unregister(pid)
      return
    }
    if (record && processGroupExists(record.pgid)) {
      this.monitorProcessGroup(pid)
      return
    }
    this.unregister(pid)
  }

  private monitorProcessGroup(pid: number): void {
    if (this.groupMonitors.has(pid) || !this.live.has(pid)) {
      return
    }
    const check = () => {
      this.groupMonitors.delete(pid)
      const record = this.live.get(pid)
      if (!record) {
        return
      }
      if (!processGroupExists(record.pgid)) {
        this.unregister(pid)
        return
      }
      const timer = setTimeout(check, 250)
      timer.unref()
      this.groupMonitors.set(pid, timer)
    }
    const timer = setTimeout(check, 250)
    timer.unref()
    this.groupMonitors.set(pid, timer)
  }

  private unregister(pid: number): void {
    this.live.delete(pid)
    const monitor = this.groupMonitors.get(pid)
    if (monitor) {
      clearTimeout(monitor)
      this.groupMonitors.delete(pid)
    }
  }
}

async function terminateWindowsProcessTree(pid: number): Promise<void> {
  if (!processExists(pid)) {
    return
  }
  const executable = windowsSystemExecutable('taskkill.exe')
  await new Promise<void>((resolve, reject) => {
    execFile(executable, ['/pid', String(pid), '/t', '/f'], { windowsHide: true }, error => {
      if (!error || !processExists(pid)) {
        resolve()
      } else {
        reject(error)
      }
    })
  })
}

function processExists(pid: number): boolean {
  try {
    process.kill(pid, 0)
    return true
  } catch {
    return false
  }
}

async function waitForChildExit(child: ChildProcess, timeout: number): Promise<boolean> {
  if (child.exitCode !== null || child.signalCode !== null) {
    return true
  }
  return new Promise(resolve => {
    let settled = false
    const finish = (exited: boolean) => {
      if (settled) {
        return
      }
      settled = true
      clearTimeout(timer)
      child.off('exit', onExit)
      resolve(exited)
    }
    const onExit = () => finish(true)
    const timer = setTimeout(() => finish(child.exitCode !== null || child.signalCode !== null), timeout)
    timer.unref()
    child.once('exit', onExit)
  })
}

function killGroup(pgid: number): void {
  try {
    process.kill(-pgid, 'SIGKILL')
  } catch (error) {
    if (!isNoSuchProcess(error)) {
      throw error
    }
  }
}

function processGroupExists(pgid: number): boolean {
  try {
    process.kill(-pgid, 0)
    return true
  } catch (error) {
    return isNodeError(error, 'EPERM')
  }
}

async function waitForProcessGroupExit(pgid: number, timeout: number): Promise<boolean> {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (!processGroupExists(pgid)) {
      return true
    }
    await new Promise(resolve => setTimeout(resolve, 25))
  }
  return !processGroupExists(pgid)
}

function isNoSuchProcess(error: unknown): boolean {
  return isNodeError(error, 'ESRCH')
}

function isNodeError(error: unknown, code: string): boolean {
  return typeof error === 'object' && error !== null && 'code' in error && error.code === code
}

function safeError(error: unknown): string {
  return (error instanceof Error ? error.message : String(error)).slice(0, 512)
}
