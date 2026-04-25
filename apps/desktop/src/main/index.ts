import { app, shell, BrowserWindow, ipcMain } from 'electron'
import { join } from 'node:path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import iconPng from '../../resources/icon.png?asset'

const CHAT_DEFAULTS = { width: 1280, height: 800, minWidth: 960, minHeight: 600 }
const SETTINGS_DEFAULTS = { width: 1080, height: 720, minWidth: 880, minHeight: 560 }

type WindowKind = 'chat' | 'settings'

let chatWindow: BrowserWindow | null = null
let settingsWindow: BrowserWindow | null = null

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

// On macOS we hide the system titlebar but keep the native traffic lights
// (`hiddenInset`). Renderers reserve space for them via a custom TopBar.
const macTitleBarOptions: Partial<Electron.BrowserWindowConstructorOptions>
  = process.platform === 'darwin'
    ? { titleBarStyle: 'hiddenInset', trafficLightPosition: { x: 14, y: 12 } }
    : {}

function createChatWindow(): BrowserWindow {
  const window = new BrowserWindow({
    ...CHAT_DEFAULTS,
    ...macTitleBarOptions,
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

function createSettingsWindow(parent: BrowserWindow | null): BrowserWindow {
  const window = new BrowserWindow({
    ...SETTINGS_DEFAULTS,
    ...macTitleBarOptions,
    show: false,
    autoHideMenuBar: true,
    title: 'Memoh · Settings',
    icon: iconPng,
    parent: parent ?? undefined,
    // Not modal — user can still interact with the chat window while settings is open.
    modal: false,
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
    settingsWindow = null
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
    settingsWindow = createSettingsWindow(chatWindow)
  }
  return settingsWindow
}

function focusWindow(window: BrowserWindow): void {
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}

app.whenReady().then(() => {
  electronApp.setAppUserModelId('ai.memoh.desktop')

  if (process.platform === 'darwin' && app.dock) {
    app.dock.setIcon(iconPng)
  }

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  ipcMain.handle('window:open-settings', () => {
    focusWindow(ensureWindow('settings'))
  })
  ipcMain.handle('window:close-self', (event) => {
    const sender = BrowserWindow.fromWebContents(event.sender)
    sender?.close()
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
