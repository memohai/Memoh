import { app, shell, BrowserWindow, ipcMain } from 'electron'
import { join } from 'node:path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import iconPng from '../../resources/icon.png?asset'
import { defaultWorkspacePath, ensureLocalServer, getLocalServerStatus } from './local-server'

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

  window.on('ready-to-show', () => {
    window.setParentWindow(null)
    window.show()
  })
  window.on('closed', () => {
    pendingSettingsNavigate.delete(window.webContents.id)
    settingsWindow = null
  })

  // Drain any queued navigate target as soon as the renderer is ready to
  // receive IPC messages. Reusing `did-finish-load` keeps both fresh
  // cold-starts and in-place refreshes working without extra coordination.
  window.webContents.on('did-finish-load', () => {
    const target = pendingSettingsNavigate.get(window.webContents.id)
    if (!target) return
    pendingSettingsNavigate.delete(window.webContents.id)
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

app.whenReady().then(async () => {
  electronApp.setAppUserModelId('ai.memoh.desktop')
  await ensureLocalServer()

  if (process.platform === 'darwin' && app.dock) {
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
  ipcMain.handle('desktop:default-workspace-path', (_event, rawDisplayName: unknown) => {
    return defaultWorkspacePath(typeof rawDisplayName === 'string' ? rawDisplayName : '')
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
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
