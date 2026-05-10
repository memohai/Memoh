import { app, dialog, Menu, shell, BrowserWindow, ipcMain, type MenuItemConstructorOptions } from 'electron'
import { join } from 'node:path'
import { existsSync, renameSync } from 'node:fs'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import iconPng from '../../resources/icon.png?asset'
import { stopEmbeddedQdrant } from './qdrant'
import { defaultWorkspacePath, ensureLocalServer, getDesktopAuthToken, getLocalServerStatus, stopManagedServer } from './local-server'
import {
  detectCliState,
  installCli,
  linuxPathHint,
  readCliPrefs,
  uninstallCli,
  writeCliPrefs,
  type CliStatus,
} from './cli-integration'

// Migration: prior to v0.8.x productName was implicitly the package `name`
// (`@memohai/desktop`), so userData lived at `~/Library/Application
// Support/@memohai/desktop/` on macOS (and analogous paths on other OSes).
// Pinning productName to `Memoh` switches the userData root to `…/Memoh/`.
// We rename the old directory in place once, before Electron caches the path.
// CLI shipped alongside desktop relies on this stable layout.
function migrateLegacyUserDataDirectory(): void {
  const home = app.getPath('home')
  let legacy: string | null = null
  let modern: string | null = null
  switch (process.platform) {
    case 'darwin': {
      const base = join(home, 'Library', 'Application Support')
      legacy = join(base, '@memohai', 'desktop')
      modern = join(base, 'Memoh')
      break
    }
    case 'win32': {
      const appData = process.env.APPDATA || join(home, 'AppData', 'Roaming')
      legacy = join(appData, '@memohai', 'desktop')
      modern = join(appData, 'Memoh')
      break
    }
    default: {
      const xdg = process.env.XDG_CONFIG_HOME || join(home, '.config')
      legacy = join(xdg, '@memohai', 'desktop')
      modern = join(xdg, 'Memoh')
      break
    }
  }
  if (!legacy || !modern) return
  if (existsSync(modern) || !existsSync(legacy)) return
  try {
    renameSync(legacy, modern)
  } catch (error) {
    console.error('failed to migrate userData directory', { from: legacy, to: modern, error })
  }
}

// Must run before anything resolves `app.getPath('userData')`.
app.setName('Memoh')
migrateLegacyUserDataDirectory()

const CHAT_DEFAULTS = { width: 1280, height: 800, minWidth: 960, minHeight: 600 }
const SETTINGS_DEFAULTS = { width: 1080, height: 720, minWidth: 880, minHeight: 560 }

type WindowKind = 'chat' | 'settings'

let chatWindow: BrowserWindow | null = null
let settingsWindow: BrowserWindow | null = null

// Pending settings-navigate target keyed by webContents id. Set by the
// `window:open-settings` IPC when the settings window has not finished
// loading yet (cold start, refresh, etc.) and drained by the per-window
// `did-finish-load` listener attached at creation time. Storing on a Map
// rather than a closure variable lets us stay correct if a future change
// ever introduces multiple settings windows.
const pendingSettingsNavigate = new Map<number, string>()
let stoppingLocalProcesses = false

async function stopLocalProcesses(): Promise<void> {
  await stopManagedServer()
  await stopEmbeddedQdrant()
}

app.on('before-quit', (event) => {
  if (stoppingLocalProcesses) return
  stoppingLocalProcesses = true
  event.preventDefault()
  void stopLocalProcesses()
    .catch((error) => {
      console.error('failed to stop local desktop processes', error)
    })
    .finally(() => app.quit())
})

app.on('will-quit', () => {
  if (stoppingLocalProcesses) return
  stoppingLocalProcesses = true
  void stopLocalProcesses().catch((error) => {
    console.error('failed to stop local desktop processes', error)
  })
})

function applyExternalLinkHandler(window: BrowserWindow): void {
  window.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
}

function loadRendererEntry(window: BrowserWindow, entry: 'index' | 'settings'): void {
  const base = process.env.ELECTRON_RENDERER_URL
  if (is.dev && base) {
    window.loadURL(`${base}/${entry}.html`)
    return
  }
  window.loadFile(join(__dirname, `../renderer/${entry}.html`))
}

// `electron-vite` emits the preload bundle as `index.mjs` because the
// package is ESM (`"type": "module"`). Electron silently no-ops if this
// path doesn't exist — keeping the file name in sync with the build
// output is what wires the IPC bridge into the renderer.
const PRELOAD_FILE = '../preload/index.mjs'

// On macOS we hide the system titlebar but keep the native traffic lights.
// A transparent window background prevents the hidden titlebar area from
// flashing or retaining the default white backing above the renderer.
function macWindowChromeOptions(tabbingIdentifier: string): Partial<Electron.BrowserWindowConstructorOptions> {
  if (process.platform !== 'darwin') return {}
  return {
    titleBarStyle: 'hidden',
    trafficLightPosition: { x: 14, y: 12 },
    transparent: true,
    backgroundColor: '#00000000',
    tabbingIdentifier,
  }
}

function createChatWindow(): BrowserWindow {
  const window = new BrowserWindow({
    ...CHAT_DEFAULTS,
    ...macWindowChromeOptions('memoh-chat'),
    show: false,
    autoHideMenuBar: true,
    title: 'Memoh',
    icon: iconPng,
    webPreferences: {
      preload: join(__dirname, PRELOAD_FILE),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  window.on('ready-to-show', () => {
    window.show()
  })
  window.on('closed', () => {
    chatWindow = null
  })

  applyExternalLinkHandler(window)
  loadRendererEntry(window, 'index')
  return window
}

function createSettingsWindow(): BrowserWindow {
  const window = new BrowserWindow({
    ...SETTINGS_DEFAULTS,
    ...macWindowChromeOptions('memoh-settings'),
    show: false,
    autoHideMenuBar: true,
    title: 'Memoh · Settings',
    icon: iconPng,
    webPreferences: {
      preload: join(__dirname, PRELOAD_FILE),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })
  window.setParentWindow(null)
  const webContentsId = window.webContents.id

  window.on('ready-to-show', () => {
    if (window.isDestroyed()) return
    window.setParentWindow(null)
    window.show()
  })
  window.on('closed', () => {
    pendingSettingsNavigate.delete(webContentsId)
    settingsWindow = null
  })

  // Drain any queued navigate target as soon as the renderer is ready to
  // receive IPC messages. Reusing `did-finish-load` keeps both fresh
  // cold-starts and in-place refreshes working without extra coordination.
  window.webContents.on('did-finish-load', () => {
    const target = pendingSettingsNavigate.get(webContentsId)
    if (!target) return
    if (window.isDestroyed()) return
    pendingSettingsNavigate.delete(webContentsId)
    window.webContents.send('settings:navigate', target)
  })

  applyExternalLinkHandler(window)
  loadRendererEntry(window, 'settings')
  return window
}

function ensureWindow(kind: WindowKind): BrowserWindow {
  if (kind === 'chat') {
    if (!chatWindow || chatWindow.isDestroyed()) chatWindow = createChatWindow()
    return chatWindow
  }
  if (!settingsWindow || settingsWindow.isDestroyed()) {
    settingsWindow = createSettingsWindow()
  }
  return settingsWindow
}

function focusWindow(window: BrowserWindow): void {
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}

function dispatchSettingsNavigate(window: BrowserWindow, target: string): void {
  // If the renderer hasn't booted yet (cold start) or is mid-reload, we
  // can't push the navigate event straight away — buffer it for the
  // `did-finish-load` listener to drain. Otherwise send immediately so
  // warm clicks feel instant.
  if (window.webContents.isLoading()) {
    pendingSettingsNavigate.set(window.webContents.id, target)
    return
  }
  window.webContents.send('settings:navigate', target)
}

// CLI install / menu helpers — kept above the whenReady block so the
// Promise chain can call them without forward-declaration noise.

async function runCliInstallCheck(): Promise<void> {
  // In dev (mise run desktop:dev / electron-vite dev) we skip the
  // auto-prompt entirely. The CLI binary is built lazily by
  // `installCli()` via `go build ./cmd/memoh`, so it works if the
  // developer explicitly clicks the menu, but we don't nag on every
  // hot-reload.
  if (!app.isPackaged) return

  let status: CliStatus
  try {
    status = await detectCliState()
  } catch (error) {
    console.error('failed to detect cli state', error)
    return
  }
  if (status.state === 'installed-current') return
  if (status.state === 'installed-stale') {
    try {
      await installCli()
      await rebuildAppMenu()
    } catch (error) {
      console.error('silent cli reinstall failed', error)
    }
    return
  }
  const prefs = readCliPrefs()
  if (prefs.dontAskAgain) return
  if (status.state === 'installed-foreign') return // never overwrite a non-Memoh memoh

  const detail = process.platform === 'win32'
    ? 'A `memoh` directory will be added to your user PATH (no admin required). Open a new terminal afterwards.'
    : process.platform === 'darwin'
      ? 'macOS will prompt for your administrator password to create /usr/local/bin/memoh.'
      : `A symlink will be created at ${join(app.getPath('home'), '.local', 'bin', 'memoh')}.${linuxPathHint() ? ' ' + linuxPathHint() : ''}`

  const result = await dialog.showMessageBox({
    type: 'question',
    buttons: ['Install', 'Skip', 'Don\u2019t ask again'],
    defaultId: 0,
    cancelId: 1,
    title: 'Install Memoh CLI?',
    message: 'Install the `memoh` command-line tool?',
    detail,
    noLink: true,
  })
  if (result.response === 0) {
    try {
      await installCli()
      await rebuildAppMenu()
      await dialog.showMessageBox({
        type: 'info',
        message: 'Memoh CLI installed.',
        detail: 'Run `memoh --help` in a new terminal to get started.',
      })
    } catch (error) {
      await dialog.showMessageBox({
        type: 'error',
        message: 'Failed to install Memoh CLI',
        detail: error instanceof Error ? error.message : String(error),
      })
    }
  } else if (result.response === 2) {
    writeCliPrefs({ ...prefs, dontAskAgain: true })
  }
}

async function rebuildAppMenu(): Promise<void> {
  let cliStatus: CliStatus | null = null
  try {
    cliStatus = await detectCliState()
  } catch {
    cliStatus = null
  }
  const isInstalled = cliStatus?.state === 'installed-current'
  const cliMenuItem: MenuItemConstructorOptions = {
    label: isInstalled ? 'Reinstall Command Line Tool…' : 'Install Command Line Tool…',
    click: async () => {
      try {
        await installCli()
        await rebuildAppMenu()
        await dialog.showMessageBox({
          type: 'info',
          message: 'Memoh CLI installed.',
          detail: 'Run `memoh --help` in a new terminal to get started.',
        })
      } catch (error) {
        await dialog.showMessageBox({
          type: 'error',
          message: 'Failed to install Memoh CLI',
          detail: error instanceof Error ? error.message : String(error),
        })
      }
    },
  }
  const uninstallItem: MenuItemConstructorOptions = {
    label: 'Uninstall Command Line Tool',
    enabled: isInstalled,
    click: async () => {
      try {
        await uninstallCli()
        await rebuildAppMenu()
      } catch (error) {
        await dialog.showMessageBox({
          type: 'error',
          message: 'Failed to uninstall Memoh CLI',
          detail: error instanceof Error ? error.message : String(error),
        })
      }
    },
  }

  const template: MenuItemConstructorOptions[] = []
  if (process.platform === 'darwin') {
    template.push({
      label: app.name,
      submenu: [
        { role: 'about' },
        { type: 'separator' },
        cliMenuItem,
        uninstallItem,
        { type: 'separator' },
        { role: 'services' },
        { type: 'separator' },
        { role: 'hide' },
        { role: 'hideOthers' },
        { role: 'unhide' },
        { type: 'separator' },
        { role: 'quit' },
      ],
    })
  }
  template.push(
    {
      label: 'Edit',
      submenu: [
        { role: 'undo' },
        { role: 'redo' },
        { type: 'separator' },
        { role: 'cut' },
        { role: 'copy' },
        { role: 'paste' },
        { role: 'selectAll' },
      ],
    },
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'forceReload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'resetZoom' },
        { role: 'zoomIn' },
        { role: 'zoomOut' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
    {
      label: 'Window',
      submenu: [
        { role: 'minimize' },
        { role: 'close' },
      ],
    },
  )
  if (process.platform !== 'darwin') {
    template.push({
      label: 'Tools',
      submenu: [cliMenuItem, uninstallItem],
    })
  }

  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

app.whenReady().then(async () => {
  electronApp.setAppUserModelId('ai.memoh.desktop')
  await ensureLocalServer()

  if (process.platform === 'darwin' && app.dock && is.dev) {
    app.dock.setIcon(iconPng)
  }

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  ipcMain.handle('window:open-settings', (_event, rawTarget: unknown) => {
    const window = ensureWindow('settings')
    focusWindow(window)
    const target = typeof rawTarget === 'string' && rawTarget.startsWith('/settings')
      ? rawTarget
      : null
    if (target) dispatchSettingsNavigate(window, target)
  })
  ipcMain.handle('window:close-self', (event) => {
    const sender = BrowserWindow.fromWebContents(event.sender)
    sender?.close()
  })
  ipcMain.handle('desktop:server-status', () => getLocalServerStatus())
  ipcMain.handle('desktop:api-base-url', () => getLocalServerStatus().baseUrl)
  ipcMain.handle('desktop:auth-token', () => getDesktopAuthToken())
  ipcMain.handle('desktop:default-workspace-path', (_event, rawDisplayName: unknown) => {
    return defaultWorkspacePath(typeof rawDisplayName === 'string' ? rawDisplayName : '')
  })
  ipcMain.handle('desktop:cli-status', () => detectCliState())
  ipcMain.handle('desktop:cli-install', async () => {
    await installCli()
    return detectCliState()
  })
  ipcMain.handle('desktop:cli-uninstall', async () => {
    await uninstallCli()
    return detectCliState()
  })

  // Cross-window Pinia Colada query-cache invalidation. Each renderer owns
  // an independent in-memory cache (separate Vue/Pinia instances per
  // BrowserWindow), so a mutation in the settings window can't directly
  // refresh the chat window's bot list. The renderer wraps
  // `queryCache.invalidateQueries` so that every local invalidation also
  // posts the (serializable) filter here; we fan it back out to every other
  // BrowserWindow's webContents, which then re-applies the same
  // invalidation against its local cache. The sender is excluded so we
  // don't echo back into the originating window.
  ipcMain.handle('desktop:broadcast-invalidate', (event, payload: unknown) => {
    const senderId = event.sender.id
    for (const target of BrowserWindow.getAllWindows()) {
      if (target.isDestroyed()) continue
      if (target.webContents.id === senderId) continue
      target.webContents.send('desktop:invalidate', payload)
    }
  })

  chatWindow = createChatWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) {
      chatWindow = createChatWindow()
    }
  })

  await rebuildAppMenu()
  void runCliInstallCheck()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
