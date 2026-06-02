import { app, dialog, Menu, shell, BrowserWindow, ipcMain, nativeImage, Tray, type MenuItemConstructorOptions } from 'electron'
import { join } from 'node:path'
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import iconPng from '../../resources/icon.png?asset'
import trayIconPng from '../../resources/tray-icon.png?asset'
import { stopEmbeddedQdrant } from './qdrant'
import {
  defaultWorkspacePath,
  ensureLocalServer,
  ensureProviderOAuthCallbackProxy,
  getDesktopAuthToken,
  getLocalServerStatus,
  stopManagedServer,
  stopProviderOAuthCallbackProxy,
} from './local-server'
import {
  detectCliState,
  installCli,
  linuxPathHint,
  readCliPrefs,
  uninstallCli,
  writeCliPrefs,
  type CliStatus,
} from './cli-integration'

type DesktopRuntimeMode = 'local' | 'remote'

const DESKTOP_FLAVOR = __MEMOH_DESKTOP_FLAVOR__ === 'online' ? 'online' : 'offline'
const DESKTOP_RUNTIME_MODE: DesktopRuntimeMode = DESKTOP_FLAVOR === 'online' ? 'remote' : 'local'
const ONLINE_PRODUCT_NAME = 'Memoh'
const LOCAL_PRODUCT_NAME = 'Memoh Local'
const LEGACY_REMOTE_PRODUCT_NAME = 'Memoh Online'
const LEGACY_LOCAL_PRODUCT_NAME = 'Memoh'
const DESKTOP_PRODUCT_NAME = DESKTOP_RUNTIME_MODE === 'remote' ? ONLINE_PRODUCT_NAME : LOCAL_PRODUCT_NAME

interface RemoteProfile {
  baseUrl?: string
}

const LOCAL_USER_DATA_ENTRIES = [
  'config.toml',
  'local-server',
  'local-server.log',
  'local-server.pid.json',
  'qdrant',
  'gstreamer',
  'cli-token.json',
  'cli-prefs.json',
]

function platformUserDataBaseDirectory(): string {
  const home = app.getPath('home')
  switch (process.platform) {
    case 'darwin': {
      return join(home, 'Library', 'Application Support')
    }
    case 'win32': {
      return process.env.APPDATA || join(home, 'AppData', 'Roaming')
    }
    default: {
      return process.env.XDG_CONFIG_HOME || join(home, '.config')
    }
  }
}

function productUserDataDirectory(productName: string): string {
  return join(platformUserDataBaseDirectory(), productName)
}

function legacyPackageUserDataDirectory(): string {
  return join(platformUserDataBaseDirectory(), '@memohai', 'desktop')
}

function moveUserDataEntries(source: string, target: string, entries: string[]): void {
  if (!existsSync(source)) return
  mkdirSync(target, { recursive: true })
  for (const entry of entries) {
    const sourcePath = join(source, entry)
    const targetPath = join(target, entry)
    if (!existsSync(sourcePath) || existsSync(targetPath)) continue
    renameSync(sourcePath, targetPath)
  }
}

function hasLocalUserData(source: string): boolean {
  return LOCAL_USER_DATA_ENTRIES.some((entry) => existsSync(join(source, entry)))
}

function migrateWholeUserDataDirectory(source: string, target: string): boolean {
  if (!existsSync(source) || existsSync(target)) return false
  try {
    renameSync(source, target)
    return true
  } catch (error) {
    console.error('failed to migrate userData directory', { from: source, to: target, error })
    return false
  }
}

function migrateRemoteUserDataDirectory(): void {
  const legacy = productUserDataDirectory(LEGACY_REMOTE_PRODUCT_NAME)
  const modern = productUserDataDirectory(ONLINE_PRODUCT_NAME)
  if (migrateWholeUserDataDirectory(legacy, modern)) return
  try {
    moveUserDataEntries(legacy, modern, ['remote-profile.json'])
  } catch (error) {
    console.error('failed to migrate remote userData entries', { from: legacy, to: modern, error })
  }
}

function migrateLocalUserDataDirectory(): void {
  const modern = productUserDataDirectory(LOCAL_PRODUCT_NAME)
  const legacyPackage = legacyPackageUserDataDirectory()
  const legacyLocal = productUserDataDirectory(LEGACY_LOCAL_PRODUCT_NAME)

  if (!migrateWholeUserDataDirectory(legacyPackage, modern)) {
    try {
      moveUserDataEntries(legacyPackage, modern, LOCAL_USER_DATA_ENTRIES)
    } catch (error) {
      console.error('failed to migrate package userData entries', { from: legacyPackage, to: modern, error })
    }
  }

  if (!existsSync(legacyLocal) || !hasLocalUserData(legacyLocal)) return
  if (!existsSync(join(legacyLocal, 'remote-profile.json')) && migrateWholeUserDataDirectory(legacyLocal, modern)) return
  try {
    moveUserDataEntries(legacyLocal, modern, LOCAL_USER_DATA_ENTRIES)
  } catch (error) {
    console.error('failed to migrate local userData entries', { from: legacyLocal, to: modern, error })
  }
}

// Must run before anything resolves `app.getPath('userData')`.
app.setName(DESKTOP_PRODUCT_NAME)
if (DESKTOP_RUNTIME_MODE === 'remote') {
  migrateRemoteUserDataDirectory()
} else {
  migrateLocalUserDataDirectory()
}

const CHAT_DEFAULTS = { width: 1280, height: 800, minWidth: 960, minHeight: 600 }
const SETTINGS_DEFAULTS = { width: 1080, height: 720, minWidth: 880, minHeight: 560 }
const CONNECTION_DEFAULTS = { width: 520, height: 360, minWidth: 460, minHeight: 320 }

type WindowKind = 'chat' | 'settings'
type TrayBot = {
  id: string
  displayName: string
}
type TraySettingsItem = {
  label: string
  target: string
}

let chatWindow: BrowserWindow | null = null
let settingsWindow: BrowserWindow | null = null
let connectionWindow: BrowserWindow | null = null
let appTray: Tray | null = null

// Pending settings-navigate target keyed by webContents id. Set by the
// `window:open-settings` IPC when the settings window has not finished
// loading yet (cold start, refresh, etc.) and drained by the per-window
// `did-finish-load` listener attached at creation time. Storing on a Map
// rather than a closure variable lets us stay correct if a future change
// ever introduces multiple settings windows.
const pendingSettingsNavigate = new Map<number, string>()
let stoppingLocalProcesses = false

function isRemoteMode(): boolean {
  return DESKTOP_RUNTIME_MODE === 'remote'
}

function remoteProfilePath(): string {
  return join(app.getPath('userData'), 'remote-profile.json')
}

function readRemoteProfile(): RemoteProfile {
  if (!isRemoteMode()) return {}
  const path = remoteProfilePath()
  if (!existsSync(path)) return {}
  try {
    const parsed = JSON.parse(readFileSync(path, 'utf8')) as RemoteProfile
    return {
      baseUrl: typeof parsed.baseUrl === 'string' ? normalizeRemoteBaseUrl(parsed.baseUrl) : undefined,
    }
  } catch (error) {
    console.error('failed to read remote desktop profile', error)
    return {}
  }
}

function writeRemoteProfile(profile: RemoteProfile): void {
  mkdirSync(app.getPath('userData'), { recursive: true })
  writeFileSync(remoteProfilePath(), `${JSON.stringify(profile, null, 2)}\n`, { mode: 0o600 })
}

function getDesktopApiBaseUrl(): string {
  if (!isRemoteMode()) {
    return getLocalServerStatus().baseUrl
  }
  return readRemoteProfile().baseUrl ?? ''
}

function normalizeRemoteBaseUrl(raw: string): string {
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

async function probeRemoteBaseUrl(baseUrl: string): Promise<void> {
  const controller = new AbortController()
  const timeout = setTimeout(() => controller.abort(), 5000)
  try {
    const response = await fetch(`${baseUrl}/ping`, { signal: controller.signal })
    if (!response.ok) {
      throw new Error(`GET /ping failed with HTTP ${response.status}`)
    }
    const payload = await response.json().catch(() => null) as { status?: string } | null
    if (payload?.status !== 'ok') {
      throw new Error('GET /ping did not return a Memoh server response')
    }
  } finally {
    clearTimeout(timeout)
  }
}

function getDesktopServerStatus() {
  if (!isRemoteMode()) {
    return {
      mode: DESKTOP_RUNTIME_MODE,
      ...getLocalServerStatus(),
    }
  }
  const baseUrl = getDesktopApiBaseUrl()
  return {
    mode: DESKTOP_RUNTIME_MODE,
    baseUrl,
    ready: baseUrl !== '',
    managed: false,
  }
}

async function clearRendererAuthState(): Promise<void> {
  const script = `
    window.localStorage.removeItem('token');
    window.localStorage.removeItem('user');
    window.sessionStorage.clear();
  `
  await Promise.all(BrowserWindow.getAllWindows().map(async (window) => {
    if (window.isDestroyed() || window.webContents.isDestroyed()) return
    try {
      await window.webContents.executeJavaScript(script, true)
    } catch (error) {
      console.warn('failed to clear renderer auth state after server switch', error)
    }
  }))
}

function reloadRendererWindowsExcept(webContentsId: number): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (window.isDestroyed() || window.webContents.isDestroyed()) continue
    if (window.webContents.id === webContentsId) continue
    window.webContents.reload()
  }
}

async function stopLocalProcesses(): Promise<void> {
  if (isRemoteMode()) return
  await stopProviderOAuthCallbackProxy()
  await stopManagedServer()
  await stopEmbeddedQdrant()
}

function hideDesktopSurfacesForQuit(): void {
  for (const window of BrowserWindow.getAllWindows()) {
    if (window.isDestroyed()) continue
    window.hide()
    window.destroy()
  }
  appTray?.destroy()
  appTray = null
  if (process.platform === 'darwin' && app.dock) {
    app.dock.hide()
  }
}

app.on('before-quit', (event) => {
  if (stoppingLocalProcesses) return
  stoppingLocalProcesses = true
  event.preventDefault()
  hideDesktopSurfacesForQuit()
  void stopLocalProcesses()
    .catch((error) => {
      console.error('failed to stop local desktop processes', error)
    })
    .finally(() => app.exit(0))
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

function loadRendererEntry(window: BrowserWindow, entry: 'index' | 'settings' | 'connection'): void {
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
  // The identifier may be a bot name or UUID; both resolve on the chat page.
  const target = `/bot/${encodeURIComponent(id)}`
  if (window.webContents.isLoading()) {
    window.webContents.once('did-finish-load', () => {
      if (window.isDestroyed()) return
      window.webContents.send('chat:navigate', target)
    })
    return
  }
  window.webContents.send('chat:navigate', target)
}

const SETTINGS_TRAY_ITEMS: TraySettingsItem[] = [
  { label: 'Bots', target: '/settings/bots' },
  { label: 'Providers', target: '/settings/providers' },
  { label: 'Web Search', target: '/settings/web-search' },
  { label: 'Memory', target: '/settings/memory' },
  { label: 'Speech', target: '/settings/speech' },
  { label: 'Transcription', target: '/settings/transcription' },
  { label: 'Email', target: '/settings/email' },
  { label: 'Supermarket', target: '/settings/supermarket' },
  { label: 'Usage', target: '/settings/usage' },
  { label: 'Appearance', target: '/settings/appearance' },
  { label: 'Profile', target: '/settings/profile' },
  { label: 'About', target: '/settings/about' },
]

function openSettingsWindow(target?: string): void {
  const window = ensureWindow('settings')
  focusWindow(window)
  if (target?.startsWith('/settings')) {
    dispatchSettingsNavigate(window, target)
  }
}

function openMemohSettings(): void {
  focusWindow(ensureConnectionWindow())
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
          click: () => openSettingsWindow('/settings'),
        },
        { type: 'separator' },
        ...SETTINGS_TRAY_ITEMS.map((item) => ({
          label: item.label,
          click: () => openSettingsWindow(item.target),
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

function normalizeTrayBots(payload: unknown): TrayBot[] {
  if (!payload || typeof payload !== 'object') return []
  const items = (payload as { items?: unknown }).items
  if (!Array.isArray(items)) return []
  return items.flatMap((item): TrayBot[] => {
    if (!item || typeof item !== 'object') return []
    const record = item as { id?: unknown; display_name?: unknown; name?: unknown }
    const id = typeof record.id === 'string' ? record.id.trim() : ''
    if (!id) return []
    const displayName =
      (typeof record.display_name === 'string' ? record.display_name.trim() : '') ||
      (typeof record.name === 'string' ? record.name.trim() : '') ||
      id
    return [{ id, displayName }]
  })
}

async function fetchTrayBots(): Promise<TrayBot[]> {
  if (isRemoteMode()) return []
  const baseUrl = getDesktopApiBaseUrl()
  if (!baseUrl) return []
  const token = await getDesktopAuthToken()
  const response = await fetch(`${baseUrl.replace(/\/$/, '')}/bots`, {
    headers: token ? { Authorization: `Bearer ${token}` } : {},
  })
  if (!response.ok) {
    throw new Error(`GET /bots failed with ${response.status}`)
  }
  return normalizeTrayBots(await response.json())
}

function setTrayMenu(bots: TrayBot[] = []): void {
  appTray?.setContextMenu(buildTrayMenu(bots))
}

async function refreshTrayMenu(): Promise<void> {
  try {
    setTrayMenu(await fetchTrayBots())
  } catch (error) {
    console.error('failed to refresh tray bots', error)
    setTrayMenu()
  }
}

async function showTrayMenu(): Promise<void> {
  if (!appTray) return
  try {
    const menu = buildTrayMenu(await fetchTrayBots())
    appTray.setContextMenu(menu)
    appTray.popUpContextMenu(menu)
  } catch (error) {
    console.error('failed to show tray bots', error)
    const menu = buildTrayMenu()
    appTray.setContextMenu(menu)
    appTray.popUpContextMenu(menu)
  }
}

function createAppTray(): void {
  if (appTray) return

  appTray = new Tray(createTrayIcon())
  appTray.setToolTip(DESKTOP_PRODUCT_NAME)
  setTrayMenu()
  appTray.on('click', () => {
    void showTrayMenu()
  })
  appTray.on('right-click', () => {
    void showTrayMenu()
  })
  void refreshTrayMenu()
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
    title: DESKTOP_PRODUCT_NAME,
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
  window.on('close', (event) => {
    if (stoppingLocalProcesses) return
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

function createSettingsWindow(): BrowserWindow {
  const window = new BrowserWindow({
    ...SETTINGS_DEFAULTS,
    ...macWindowChromeOptions('memoh-settings'),
    show: false,
    autoHideMenuBar: true,
    title: `${DESKTOP_PRODUCT_NAME} · Settings`,
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

function createConnectionWindow(): BrowserWindow {
  const window = new BrowserWindow({
    ...CONNECTION_DEFAULTS,
    show: false,
    autoHideMenuBar: true,
    title: 'Memoh Settings',
    icon: iconPng,
    resizable: false,
    minimizable: false,
    maximizable: false,
    parent: chatWindow && !chatWindow.isDestroyed() ? chatWindow : undefined,
    webPreferences: {
      preload: join(__dirname, PRELOAD_FILE),
      sandbox: false,
      contextIsolation: true,
      nodeIntegration: false,
    },
  })

  window.on('ready-to-show', () => {
    if (window.isDestroyed()) return
    window.show()
  })
  window.on('closed', () => {
    connectionWindow = null
  })

  applyExternalLinkHandler(window)
  loadRendererEntry(window, 'connection')
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

function ensureConnectionWindow(): BrowserWindow {
  if (!connectionWindow || connectionWindow.isDestroyed()) {
    connectionWindow = createConnectionWindow()
  }
  return connectionWindow
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
  if (isRemoteMode()) return
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
  if (isRemoteMode()) {
    const template: MenuItemConstructorOptions[] = []
    if (process.platform === 'darwin') {
      template.push({
        label: app.name,
        submenu: [
          { role: 'about' },
          { type: 'separator' },
          {
            label: 'Memoh Settings…',
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
          { role: 'close' },
        ],
      },
    )
    Menu.setApplicationMenu(Menu.buildFromTemplate(template))
    return
  }

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
  electronApp.setAppUserModelId(isRemoteMode() ? 'ai.memoh.desktop.online' : 'ai.memoh.desktop')
  if (!isRemoteMode()) {
    await ensureLocalServer()
    await ensureProviderOAuthCallbackProxy()
  }

  if (process.platform === 'darwin' && app.dock && is.dev) {
    app.dock.setIcon(iconPng)
  }

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  createAppTray()

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
  ipcMain.handle('desktop:server-status', () => getDesktopServerStatus())
  ipcMain.handle('desktop:api-base-url', () => getDesktopApiBaseUrl())
  ipcMain.handle('desktop:auth-token', () => isRemoteMode() ? '' : getDesktopAuthToken())
  ipcMain.handle('desktop:save-remote-base-url', async (event, rawBaseUrl: unknown) => {
    if (!isRemoteMode()) {
      throw new Error('Remote server URL can only be configured in online mode')
    }
    const previousBaseUrl = getDesktopApiBaseUrl()
    const baseUrl = normalizeRemoteBaseUrl(typeof rawBaseUrl === 'string' ? rawBaseUrl : '')
    await probeRemoteBaseUrl(baseUrl)
    const changed = baseUrl !== previousBaseUrl
    writeRemoteProfile({ baseUrl })
    if (changed) {
      await clearRendererAuthState()
      reloadRendererWindowsExcept(event.sender.id)
    }
    return {
      ...getDesktopServerStatus(),
      changed,
    }
  })
  ipcMain.handle('desktop:default-workspace-path', (_event, rawDisplayName: unknown) => {
    if (isRemoteMode()) return ''
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
    void refreshTrayMenu()
  })

  chatWindow = createChatWindow()

  app.on('activate', () => {
    revealChatWindow()
  })

  await rebuildAppMenu()
  void runCliInstallCheck()
})

app.on('window-all-closed', () => {
  if (stoppingLocalProcesses) return
  if (process.platform !== 'darwin') app.quit()
})
