import {
  app,
  Menu,
  shell,
  BrowserWindow,
  ipcMain,
  nativeImage,
  Tray,
  screen,
  type BrowserWindowConstructorOptions,
  type MenuItemConstructorOptions,
} from 'electron'
import { join } from 'node:path'
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import iconPng from '../../resources/icon.png?asset'
import trayIconPng from '../../resources/tray-icon.png?asset'
import { acceleratorForCommand, appKeyboardCommands, type AppKeyboardCommand } from '../shared/keyboard-commands'
import { dispatchFocusedWindowCommand } from './window-commands'
import { dispatchRendererNavigate } from './window-navigation'
import { maybeSelfInstallMacOS } from './self-install'

const DESKTOP_PRODUCT_NAME = 'Memoh'
const DEFAULT_BASE_URL = is.dev ? 'http://localhost:18080' : 'http://localhost:8080'
const guardedExternalLinkWebContents = new WeakSet<Electron.WebContents>()

// Must run before anything resolves `app.getPath('userData')`.
app.setName(DESKTOP_PRODUCT_NAME)

const CHAT_DEFAULTS = { width: 1280, height: 800, minWidth: 960, minHeight: 600 }
type WindowKind = 'chat'
type WindowDefaults = {
  width: number
  height: number
  minWidth: number
  minHeight: number
}
type StoredWindowState = {
  width?: number
  height?: number
  maximized?: boolean
}
type StoredWindowStates = Partial<Record<WindowKind, StoredWindowState>>
type TrayBot = {
  id: string
  displayName: string
}
type TraySettingsItem = {
  label: string
  target: string
}

let chatWindow: BrowserWindow | null = null
let appTray: Tray | null = null
let isQuitting = false
let windowStatesCache: StoredWindowStates | null = null

function windowStatePath(): string {
  return join(app.getPath('userData'), 'window-state.json')
}

function readWindowStates(): StoredWindowStates {
  if (windowStatesCache) return windowStatesCache
  const path = windowStatePath()
  if (!existsSync(path)) {
    windowStatesCache = {}
    return windowStatesCache
  }
  try {
    const parsed = JSON.parse(readFileSync(path, 'utf8')) as StoredWindowStates
    windowStatesCache = typeof parsed === 'object' && parsed !== null ? parsed : {}
  } catch (error) {
    console.error('failed to read desktop window state', error)
    windowStatesCache = {}
  }
  return windowStatesCache
}

function writeWindowState(kind: WindowKind, state: StoredWindowState): void {
  const states = {
    ...readWindowStates(),
    [kind]: state,
  }
  windowStatesCache = states
  mkdirSync(app.getPath('userData'), { recursive: true })
  writeFileSync(windowStatePath(), `${JSON.stringify(states, null, 2)}\n`, { mode: 0o600 })
}

function clampWindowDimension(raw: unknown, fallback: number, minimum: number, maximum: number): number {
  const value = typeof raw === 'number' && Number.isFinite(raw) ? Math.round(raw) : fallback
  return Math.min(Math.max(value, minimum), maximum)
}

function rememberedWindowOptions(kind: WindowKind, defaults: WindowDefaults): BrowserWindowConstructorOptions {
  const state = readWindowStates()[kind] ?? {}
  const area = screen.getPrimaryDisplay().workAreaSize
  const maxWidth = Math.max(defaults.minWidth, area.width)
  const maxHeight = Math.max(defaults.minHeight, area.height)
  return {
    ...defaults,
    width: clampWindowDimension(state.width, defaults.width, defaults.minWidth, maxWidth),
    height: clampWindowDimension(state.height, defaults.height, defaults.minHeight, maxHeight),
  }
}

function attachWindowStatePersistence(window: BrowserWindow, kind: WindowKind, defaults: WindowDefaults): void {
  let timer: ReturnType<typeof setTimeout> | null = null
  const save = (): void => {
    if (window.isDestroyed()) return
    const bounds = window.isMaximized() ? window.getNormalBounds() : window.getBounds()
    const area = screen.getDisplayMatching(window.getBounds()).workAreaSize
    const maxWidth = Math.max(defaults.minWidth, area.width)
    const maxHeight = Math.max(defaults.minHeight, area.height)
    writeWindowState(kind, {
      width: clampWindowDimension(bounds.width, defaults.width, defaults.minWidth, maxWidth),
      height: clampWindowDimension(bounds.height, defaults.height, defaults.minHeight, maxHeight),
      maximized: window.isMaximized(),
    })
  }
  const scheduleSave = (): void => {
    if (timer) clearTimeout(timer)
    timer = setTimeout(save, 300)
  }

  window.on('resize', scheduleSave)
  window.on('maximize', save)
  window.on('unmaximize', save)
  window.on('close', save)
  window.on('closed', () => {
    if (timer) clearTimeout(timer)
  })
}

function restoreWindowMaximized(window: BrowserWindow, kind: WindowKind): void {
  if (readWindowStates()[kind]?.maximized) {
    window.maximize()
  }
}

function normalizeBaseUrl(raw: string): string {
  let value = raw.trim()
  if (!value) {
    throw new Error('Server URL is required')
  }
  if (!/^[a-z][a-z\d+.-]*:\/\//i.test(value)) {
    const localHost = /^(localhost|127\.|0\.0\.0\.0|\[::1\])(?::|\/|$)/i.test(value)
    value = `${localHost ? 'http' : 'https'}://${value}`
  }
  const url = new URL(value)
  if (url.protocol !== 'http:' && url.protocol !== 'https:') {
    throw new Error('Server URL must use http or https')
  }
  url.hash = ''
  url.search = ''
  return url.toString().replace(/\/$/, '')
}

function getDesktopApiBaseUrl(): string {
  const configured =
    process.env.MEMOH_DESKTOP_BASE_URL?.trim()
    || process.env.MEMOH_WEB_PROXY_TARGET?.trim()
    || process.env.VITE_API_URL?.trim()
    || DEFAULT_BASE_URL
  return normalizeBaseUrl(configured)
}

function getDesktopServerStatus() {
  const baseUrl = getDesktopApiBaseUrl()
  return {
    baseUrl,
    ready: baseUrl !== '',
    managed: false,
  }
}

app.on('before-quit', () => {
  isQuitting = true
})

function applyExternalLinkHandler(window: BrowserWindow): void {
  attachExternalLinkGuards(window.webContents)
}

function normalizeExternalUrl(rawURL: unknown): { url: string, protocol: string, supported: boolean } {
  const url = typeof rawURL === 'string' ? rawURL.trim() : ''
  let protocol = ''
  try {
    protocol = new URL(url).protocol
  } catch {
    protocol = ''
  }
  return {
    url,
    protocol,
    supported: ['http:', 'https:', 'mailto:'].includes(protocol),
  }
}

function attachExternalLinkGuards(webContents: Electron.WebContents): void {
  if (guardedExternalLinkWebContents.has(webContents)) return
  guardedExternalLinkWebContents.add(webContents)

  webContents.setWindowOpenHandler(({ url }) => {
    const external = normalizeExternalUrl(url)
    if (!external.supported) {
      console.warn('blocked unsupported window.open URL', external.url || url)
      return { action: 'deny' }
    }
    void shell.openExternal(external.url).catch((error) => {
      console.error('failed to open external URL', external.url, error)
    })
    return { action: 'deny' }
  })

  webContents.on('will-navigate', (event, url) => {
    const external = normalizeExternalUrl(url)
    if (external.protocol === 'about:' || (!external.supported && external.protocol !== 'file:')) {
      console.warn('blocked unsupported navigation URL', external.url || url)
      event.preventDefault()
    }
  })
}

async function openExternalUrl(rawURL: unknown): Promise<void> {
  const external = normalizeExternalUrl(rawURL)
  if (!external.supported) {
    const blockedURL = external.url || String(rawURL ?? '')
    console.warn('blocked unsupported external URL', blockedURL)
    throw new Error(`Unsupported external URL: ${blockedURL || 'empty URL'}`)
  }
  await shell.openExternal(external.url)
}

function loadRendererEntry(window: BrowserWindow, entry: 'index'): void {
  const base = process.env.ELECTRON_RENDERER_URL
  if (is.dev && base) {
    window.loadURL(`${base}/${entry}.html`)
    return
  }
  window.loadFile(join(__dirname, `../renderer/${entry}.html`))
}

function createTrayIcon(): Electron.NativeImage {
  const image = nativeImage.createFromPath(trayIconPng)
  if (process.platform === 'darwin') {
    const trayImage = image.resize({ width: 18, height: 18 })
    trayImage.setTemplateImage(true)
    return trayImage
  }
  return image.resize({ width: 24, height: 24 })
}

function revealChatWindow(): void {
  const window = ensureWindow('chat')
  focusWindow(window)
}

function openBotWorkspace(botId: string): void {
  const id = botId.trim()
  if (!id) return
  const window = ensureWindow('chat')
  focusWindow(window)
  const target = `/bot/${encodeURIComponent(id)}`
  dispatchRendererNavigate(window, target)
}

const SETTINGS_TRAY_ITEMS: TraySettingsItem[] = [
  { label: 'Bots', target: '/settings/bots' },
  { label: 'Providers', target: '/settings/providers' },
  { label: 'Memory', target: '/settings/memory' },
  { label: 'Web Search', target: '/settings/web-search' },
  { label: 'Voice', target: '/settings/voice' },
  { label: 'Email', target: '/settings/email' },
  { label: 'Supermarket', target: '/settings/supermarket' },
  { label: 'Usage', target: '/settings/usage' },
  { label: 'Members', target: '/settings/people' },
  { label: 'Appearance', target: '/settings/appearance' },
  { label: 'Keyboard', target: '/settings/keyboard' },
  { label: 'Profile', target: '/settings/profile' },
  { label: 'About', target: '/settings/about' },
]

function openSettingsRoute(target?: string): void {
  const window = ensureWindow('chat')
  focusWindow(window)
  if (target?.startsWith('/settings')) {
    dispatchRendererNavigate(window, target)
  }
}

function openMemohSettings(): void {
  openSettingsRoute('/settings')
}

function quitFromTray(): void {
  app.quit()
}

function buildTrayMenu(bots: TrayBot[] = []): Electron.Menu {
  const botItems: MenuItemConstructorOptions[] = bots.length > 0
    ? bots.map((bot) => ({
        label: bot.displayName,
        click: () => openBotWorkspace(bot.id),
      }))
    : [{ label: 'No Bots', enabled: false }]

  return Menu.buildFromTemplate([
    {
      label: `Show ${DESKTOP_PRODUCT_NAME}`,
      click: revealChatWindow,
    },
    { type: 'separator' },
    {
      label: 'Bots',
      enabled: false,
    },
    ...botItems,
    { type: 'separator' },
    {
      label: 'Settings',
      submenu: [
        {
          label: 'All Settings',
          click: () => openSettingsRoute('/settings'),
        },
        { type: 'separator' },
        ...SETTINGS_TRAY_ITEMS.map((item) => ({
          label: item.label,
          click: () => openSettingsRoute(item.target),
        })),
      ],
    },
    { type: 'separator' },
    {
      label: `Quit ${DESKTOP_PRODUCT_NAME}`,
      click: quitFromTray,
    },
  ])
}

// The main process has no credentials of its own, so it never fetches bots
// from the server. The renderer — which owns the authenticated SDK session —
// pushes its bot list over `desktop:set-tray-bots`; main keeps the last
// pushed list and rebuilds the tray menu from it.
let trayBots: TrayBot[] = []

function normalizeTrayBots(payload: unknown): TrayBot[] {
  if (!Array.isArray(payload)) return []
  return payload.flatMap((item): TrayBot[] => {
    if (!item || typeof item !== 'object') return []
    const record = item as { id?: unknown, displayName?: unknown }
    const id = typeof record.id === 'string' ? record.id.trim() : ''
    if (!id) return []
    const displayName = (typeof record.displayName === 'string' ? record.displayName.trim() : '') || id
    return [{ id, displayName }]
  })
}

function setTrayMenu(): void {
  appTray?.setContextMenu(buildTrayMenu(trayBots))
}

function showTrayMenu(): void {
  if (!appTray) return
  const menu = buildTrayMenu(trayBots)
  appTray.setContextMenu(menu)
  appTray.popUpContextMenu(menu)
}

function createAppTray(): void {
  if (appTray) return

  appTray = new Tray(createTrayIcon())
  appTray.setToolTip(DESKTOP_PRODUCT_NAME)
  setTrayMenu()
  appTray.on('click', () => {
    showTrayMenu()
  })
  appTray.on('right-click', () => {
    showTrayMenu()
  })
}

// `electron-vite` emits the preload bundle as `index.mjs` because the
// package is ESM (`"type": "module"`). Electron silently no-ops if this
// path doesn't exist — keeping the file name in sync with the build
// output is what wires the IPC bridge into the renderer.
const PRELOAD_FILE = '../preload/index.mjs'

function macWindowChromeOptions(tabbingIdentifier: string): Partial<Electron.BrowserWindowConstructorOptions> {
  if (process.platform !== 'darwin') return {}
  return {
    titleBarStyle: 'hidden',
    trafficLightPosition: { x: 14, y: 13 },
    transparent: true,
    backgroundColor: '#00000000',
    tabbingIdentifier,
  }
}

function createChatWindow(): BrowserWindow {
  const window = new BrowserWindow({
    ...rememberedWindowOptions('chat', CHAT_DEFAULTS),
    ...macWindowChromeOptions('memoh-chat'),
    show: false,
    autoHideMenuBar: true,
    title: DESKTOP_PRODUCT_NAME,
    icon: iconPng,
    webPreferences: {
      preload: join(__dirname, PRELOAD_FILE),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })
  attachWindowStatePersistence(window, 'chat', CHAT_DEFAULTS)

  window.once('ready-to-show', () => {
    restoreWindowMaximized(window, 'chat')
    window.show()
  })
  window.on('close', (event) => {
    if (isQuitting) return
    event.preventDefault()
    window.hide()
  })
  window.on('closed', () => {
    chatWindow = null
  })

  applyExternalLinkHandler(window)
  loadRendererEntry(window, 'index')
  return window
}

function ensureWindow(_kind: WindowKind): BrowserWindow {
  if (!chatWindow || chatWindow.isDestroyed()) chatWindow = createChatWindow()
  return chatWindow
}

function focusWindow(window: BrowserWindow): void {
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}

const menuAcceleratorOverrides = new Map<string, string>()

function effectiveMenuAccelerator(command: AppKeyboardCommand): string | undefined {
  return menuAcceleratorOverrides.get(command) ?? acceleratorForCommand(command)
}

function closeWindowMenuItem(): MenuItemConstructorOptions {
  return {
    label: 'Close',
    accelerator: effectiveMenuAccelerator(appKeyboardCommands.closeCurrentWorkspaceTab),
    click: () => {
      dispatchFocusedWindowCommand(
        chatWindow,
        BrowserWindow.getFocusedWindow(),
        appKeyboardCommands.closeCurrentWorkspaceTab,
      )
    },
  }
}

async function rebuildAppMenu(): Promise<void> {
  const template: MenuItemConstructorOptions[] = []
  if (process.platform === 'darwin') {
    template.push({
      label: app.name,
      submenu: [
        { role: 'about' },
        { type: 'separator' },
        {
          label: 'Memoh Settings...',
          click: openMemohSettings,
        },
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
        closeWindowMenuItem(),
      ],
    },
  )

  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

app.whenReady().then(async () => {
  if (maybeSelfInstallMacOS()) return

  electronApp.setAppUserModelId('ai.memoh.desktop')

  if (process.platform === 'darwin' && app.dock && is.dev) {
    app.dock.setIcon(iconPng)
  }

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
    attachExternalLinkGuards(window.webContents)
  })

  createAppTray()

  ipcMain.handle('window:close-self', (event) => {
    const sender = BrowserWindow.fromWebContents(event.sender)
    sender?.close()
  })
  ipcMain.handle('desktop:server-status', () => getDesktopServerStatus())
  ipcMain.handle('desktop:api-base-url', () => getDesktopApiBaseUrl())
  ipcMain.handle('desktop:set-menu-accelerators', async (_event, rawPayload: unknown) => {
    if (!rawPayload || typeof rawPayload !== 'object') return
    const incoming = new Map<string, string>()
    for (const [command, accelerator] of Object.entries(rawPayload as Record<string, unknown>)) {
      if (typeof accelerator !== 'string' || !accelerator) continue
      incoming.set(command, accelerator)
    }
    let changed = incoming.size !== menuAcceleratorOverrides.size
    if (!changed) {
      for (const [command, accelerator] of incoming) {
        if (menuAcceleratorOverrides.get(command) !== accelerator) {
          changed = true
          break
        }
      }
    }
    if (!changed) return
    menuAcceleratorOverrides.clear()
    for (const [command, accelerator] of incoming) {
      menuAcceleratorOverrides.set(command, accelerator)
    }
    await rebuildAppMenu()
  })
  ipcMain.handle('desktop:open-external-url', (_event, rawURL: unknown) => openExternalUrl(rawURL))
  ipcMain.handle('desktop:set-tray-bots', (_event, payload: unknown) => {
    trayBots = normalizeTrayBots(payload)
    setTrayMenu()
  })
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
    revealChatWindow()
  })

  await rebuildAppMenu()
})

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') app.quit()
})
