// CLI integration: detect, install, and uninstall the bundled `memoh`
// command-line tool from the user's PATH. Pairs with cmd/memoh and
// internal/tui/local/ on the Go side.
//
// Per-platform install strategy:
//   macOS  : /usr/local/bin/memoh symlink (osascript + admin prompt)
//   Linux  : ~/.local/bin/memoh symlink (no admin needed)
//   Windows: HKCU\Environment PATH gets the resources/cli dir
//            (no admin needed; broadcasted via setx so new shells pick it up)

import { app } from 'electron'
import { execFile, spawnSync, type ExecFileException } from 'node:child_process'
import { existsSync, mkdirSync, readFileSync, realpathSync, symlinkSync, unlinkSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { promisify } from 'node:util'
import { desktopResourcePath, repoRoot } from './paths'

const execFileAsync = promisify(execFile)

export type CliState = 'not-installed' | 'installed-current' | 'installed-stale' | 'installed-foreign'

export interface CliStatus {
  state: CliState
  source: string
  target: string | null
  error?: string
}

export interface CliPrefs {
  dontAskAgain?: boolean
}

function cliBinaryName(): string {
  return process.platform === 'win32' ? 'memoh.exe' : 'memoh'
}

// Dev binary lives next to the dev memoh-server build (see
// `desktopServerWorkDir` in local-server.ts). Keeping them as siblings
// matches the packaged Resources/{server,cli} layout so the CLI's own
// `os.Executable()` based path-walking still finds the server in
// future dev tooling.
function devCliBinaryPath(): string {
  return join(app.getPath('userData'), 'local-server', 'bin', cliBinaryName())
}

function buildDevCliBinary(): string {
  const binary = devCliBinaryPath()
  mkdirSync(dirname(binary), { recursive: true })
  const result = spawnSync('go', ['build', '-o', binary, './cmd/memoh'], {
    cwd: repoRoot(),
    stdio: 'inherit',
  })
  if (result.status !== 0) {
    throw new Error('failed to build dev CLI binary (go build ./cmd/memoh exited non-zero)')
  }
  return binary
}

// Pure path resolver — does not touch the filesystem. In dev the
// returned path may not yet exist; callers that need the binary on
// disk (e.g. installCli) must call ensureBundledCli first.
export function bundledCliPath(): string {
  if (!app.isPackaged) {
    return devCliBinaryPath()
  }
  return desktopResourcePath('cli', cliBinaryName())
}

// Ensure the bundled binary exists. In packaged builds this is a
// no-op (the binary was baked in by `prepare-local-server.mjs`);
// in dev it shells out to `go build ./cmd/memoh` on demand so the
// menu's `Install Command Line Tool…` action works without forcing
// the developer to remember a separate prepare step.
function ensureBundledCli(): string {
  const path = bundledCliPath()
  if (existsSync(path)) return path
  if (!app.isPackaged) {
    return buildDevCliBinary()
  }
  throw new Error(`bundled CLI not found at ${path}`)
}

function prefsPath(): string {
  return join(app.getPath('userData'), 'cli-prefs.json')
}

export function readCliPrefs(): CliPrefs {
  try {
    return JSON.parse(readFileSync(prefsPath(), 'utf8')) as CliPrefs
  } catch {
    return {}
  }
}

export function writeCliPrefs(prefs: CliPrefs): void {
  try {
    mkdirSync(dirname(prefsPath()), { recursive: true })
    writeFileSync(prefsPath(), JSON.stringify(prefs, null, 2), { mode: 0o600 })
  } catch (error) {
    console.error('failed to write cli prefs', error)
  }
}

async function installedCliPath(): Promise<string | null> {
  const target = installTarget()
  if (target.symlinkPath && existsSync(target.symlinkPath)) {
    return target.symlinkPath
  }

  const cmd = process.platform === 'win32' ? 'where' : 'which'
  try {
    const { stdout } = await execFileAsync(cmd, ['memoh'])
    return stdout.split(/\r?\n/).map(line => line.trim()).find(line => line !== '') ?? null
  } catch {
    return null
  }
}

export async function detectCliState(): Promise<CliStatus> {
  const source = bundledCliPath()
  const located = await installedCliPath()
  if (!located) {
    return { state: 'not-installed', source, target: null }
  }
  let realLocated: string
  let realSource: string
  try {
    realLocated = realpathSync(located)
  } catch {
    realLocated = located
  }
  try {
    realSource = realpathSync(source)
  } catch {
    realSource = source
  }
  if (realLocated === realSource) {
    return { state: 'installed-current', source, target: located }
  }
  // The PATH entry is a memoh binary, but doesn't point at this app.
  // It might be a stale symlink from a previous app version, or a
  // foreign install (manual go build, homebrew, etc.).
  if (realLocated.includes('/Memoh.app/') || realLocated.includes('\\Memoh\\')) {
    return { state: 'installed-stale', source, target: located }
  }
  return { state: 'installed-foreign', source, target: located }
}

interface InstallTarget {
  symlinkPath: string | null
  windowsPathDir: string | null
}

function installTarget(): InstallTarget {
  switch (process.platform) {
    case 'darwin':
      return { symlinkPath: '/usr/local/bin/memoh', windowsPathDir: null }
    case 'win32':
      return { symlinkPath: null, windowsPathDir: dirname(bundledCliPath()) }
    default: {
      const home = app.getPath('home')
      return { symlinkPath: join(home, '.local', 'bin', 'memoh'), windowsPathDir: null }
    }
  }
}

export async function installCli(): Promise<void> {
  const source = ensureBundledCli()
  const target = installTarget()
  if (target.symlinkPath) {
    await installSymlink(source, target.symlinkPath)
    return
  }
  if (target.windowsPathDir) {
    // On Windows we add the *directory* to PATH, so the binary must
    // be the one inside Resources/cli — for dev mode that means we
    // copy the freshly-built binary into a stable folder before
    // exposing it.
    await installWindowsPath(dirname(source))
    return
  }
  throw new Error(`unsupported platform: ${process.platform}`)
}

async function installSymlink(source: string, symlinkPath: string): Promise<void> {
  const targetDir = dirname(symlinkPath)
  if (process.platform === 'darwin') {
    // /usr/local/bin requires admin on Apple Silicon and is owned by
    // root by default. Defer to AppleScript so the user gets the
    // standard system password prompt.
    const escSource = source.replaceAll('"', '\\"')
    const escTarget = symlinkPath.replaceAll('"', '\\"')
    const escTargetDir = targetDir.replaceAll('"', '\\"')
    const script = `do shell script "mkdir -p '${escTargetDir}' && ln -sf '${escSource}' '${escTarget}'" with administrator privileges`
    try {
      await execFileAsync('osascript', ['-e', script])
    } catch (error) {
      throw new Error(`failed to install /usr/local/bin/memoh: ${describeExecError(error)}`)
    }
    return
  }
  // Linux: ~/.local/bin is user-owned, no admin needed.
  try {
    mkdirSync(targetDir, { recursive: true })
    if (existsSync(symlinkPath)) {
      unlinkSync(symlinkPath)
    }
    symlinkSync(source, symlinkPath)
  } catch (error) {
    throw new Error(`failed to install symlink at ${symlinkPath}: ${error instanceof Error ? error.message : String(error)}`)
  }
}

async function installWindowsPath(cliDir: string): Promise<void> {
  // setx writes to HKCU\Environment without elevation and emits the
  // WM_SETTINGCHANGE broadcast for us. The 1024-char setx limit is
  // not a concern here — we append a single dir.
  const current = await readWindowsUserPath()
  const segments = current.split(';').map(s => s.trim()).filter(Boolean)
  if (segments.some(s => resolveCaseInsensitive(s) === resolveCaseInsensitive(cliDir))) {
    return
  }
  segments.push(cliDir)
  const next = segments.join(';')
  try {
    await execFileAsync('setx', ['Path', next])
  } catch (error) {
    throw new Error(`failed to update PATH: ${describeExecError(error)}`)
  }
}

async function readWindowsUserPath(): Promise<string> {
  try {
    const { stdout } = await execFileAsync('reg', ['query', 'HKCU\\Environment', '/v', 'Path'])
    const match = stdout.match(/Path\s+REG_(?:EXPAND_)?SZ\s+(.+)/)
    return match ? match[1].trim() : ''
  } catch {
    return ''
  }
}

function resolveCaseInsensitive(path: string): string {
  if (process.platform === 'win32') return path.toLowerCase()
  return path
}

export async function uninstallCli(): Promise<void> {
  const target = installTarget()
  if (target.symlinkPath) {
    await uninstallSymlink(target.symlinkPath)
    return
  }
  if (target.windowsPathDir) {
    await uninstallWindowsPath(target.windowsPathDir)
    return
  }
}

async function uninstallSymlink(symlinkPath: string): Promise<void> {
  if (!existsSync(symlinkPath)) return
  if (process.platform === 'darwin') {
    const esc = symlinkPath.replaceAll('"', '\\"')
    const script = `do shell script "rm -f '${esc}'" with administrator privileges`
    try {
      await execFileAsync('osascript', ['-e', script])
    } catch (error) {
      throw new Error(`failed to remove ${symlinkPath}: ${describeExecError(error)}`)
    }
    return
  }
  try {
    unlinkSync(symlinkPath)
  } catch {
    // Already gone or permission denied; surfacing a hard error here
    // is more annoying than helpful.
  }
}

async function uninstallWindowsPath(cliDir: string): Promise<void> {
  const current = await readWindowsUserPath()
  const target = resolveCaseInsensitive(cliDir)
  const segments = current.split(';').map(s => s.trim()).filter(Boolean).filter(s => resolveCaseInsensitive(s) !== target)
  try {
    await execFileAsync('setx', ['Path', segments.join(';')])
  } catch (error) {
    throw new Error(`failed to update PATH: ${describeExecError(error)}`)
  }
}

function describeExecError(error: unknown): string {
  if (typeof error === 'object' && error !== null) {
    const exec = error as ExecFileException & { stderr?: string }
    if (exec.stderr) return exec.stderr.trim()
    if (exec.message) return exec.message
  }
  return String(error)
}

// On Linux, ~/.local/bin is the conventional install target but is not
// guaranteed to be on PATH. Surface a hint the renderer can display so
// users know to add it once.
export function linuxPathHint(): string | null {
  if (process.platform !== 'darwin' && process.platform !== 'win32') {
    const home = app.getPath('home')
    const target = join(home, '.local', 'bin')
    const path = process.env.PATH ?? ''
    const segments = path.split(':').map(s => resolve(s)).filter(Boolean)
    if (!segments.includes(resolve(target))) {
      return `Add ${target} to your shell PATH (e.g. \`export PATH="$HOME/.local/bin:$PATH"\` in ~/.bashrc / ~/.zshrc).`
    }
  }
  return null
}
