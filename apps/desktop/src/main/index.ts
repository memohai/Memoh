import {
  app,
  dialog,
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
import { existsSync, mkdirSync, readFileSync, renameSync, writeFileSync } from 'node:fs'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import iconPng from '../../resources/icon.png?asset'
import trayIconPng from '../../resources/tray-icon.png?asset'
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
import { acceleratorForCommand, appKeyboardCommands, type AppKeyboardCommand } from '../shared/keyboard-commands'
import { dispatchFocusedWindowCommand } from './window-commands'
import { dispatchRendererNavigate } from './window-navigation'

type DesktopRuntimeMode = 'local' | 'remote'

const DESKTOP_RUNTIME_MODE: DesktopRuntimeMode = __MEMOH_DESKTOP_RUNTIME_MODE__ === 'remote' ? 'remote' : 'local'
const ONLINE_PRODUCT_NAME = 'Memoh'
const LOCAL_PRODUCT_NAME = 'Memoh Local'
const LEGACY_REMOTE_PRODUCT_NAME = 'Memoh Online'
const LEGACY_LOCAL_PRODUCT_NAME = 'Memoh'
const DESKTOP_PRODUCT_NAME = DESKTOP_RUNTIME_MODE === 'remote' ? ONLINE_PRODUCT_NAME : LOCAL_PRODUCT_NAME
const DEFAULT_REMOTE_BASE_URL = is.dev ? 'http://localhost:18080' : 'http://localhost:8080'
const guardedExternalLinkWebContents = new WeakSet<Electron.WebContents>()

interface RemoteProfile {
  baseUrl?: string
}

const LOCAL_USER_DATA_ENTRIES = [
  'config.toml',
  'local-server',
  'local-server.log',
  'local-server.pid.json',
  'gstreamer',
  'cli-token.json',
  'cli-prefs.json',
  'window-state.json',
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
    moveUserDataEntries(legacy, modern, ['remote-profile.json', 'window-state.json'])
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

let stoppingLocalProcesses = false
let windowStatesCache: StoredWindowStates | null = null

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

function getDesktopApiBaseUrl(): string {
  if (!isRemoteMode()) {
    return getLocalServerStatus().baseUrl
  }
  return configuredRemoteBaseUrl()
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

function configuredRemoteBaseUrl(): string {
  const configured =
    process.env.MEMOH_DESKTOP_REMOTE_BASE_URL?.trim()
    || process.env.MEMOH_WEB_PROXY_TARGET?.trim()
    || process.env.VITE_API_URL?.trim()
    || readRemoteProfile().baseUrl
    || DEFAULT_REMOTE_BASE_URL
  return normalizeRemoteBaseUrl(configured)
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

async function stopLocalProcesses(): Promise<void> {
  if (isRemoteMode()) return
  await stopProviderOAuthCallbackProxy()
  await stopManagedServer()
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
  // The identifier may be a bot name or UUID; both resolve on the chat page.
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

  // ready-to-show fires on initial load AND every subsequent full reload
  // (including HMR-triggered full page reloads during AI-assisted editing).
  // Guarding with `once` ensures the window shows only on the first load —
  // it will never steal focus again on subsequent reloads.
  window.once('ready-to-show', () => {
    restoreWindowMaximized(window, 'chat')
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

function ensureWindow(_kind: WindowKind): BrowserWindow {
  if (!chatWindow || chatWindow.isDestroyed()) chatWindow = createChatWindow()
  return chatWindow
}

function focusWindow(window: BrowserWindow): void {
  if (window.isMinimized()) window.restore()
  window.show()
  window.focus()
}

// Renderer-supplied menu accelerators take precedence over the static table.
// Keyed by command id (kebab-case AppKeyboardCommand). The renderer pushes the
// current set whenever the Keyboard Shortcuts store mutates, and we rebuild
// the menu so the native item's label and matching combo stay in sync.
const menuAcceleratorOverrides = new Map<string, string>()

function effectiveMenuAccelerator(command: AppKeyboardCommand): string | undefined {
  return menuAcceleratorOverrides.get(command) ?? acceleratorForCommand(command)
}

// Derive the native Close accelerator from the renderer-pushed overrides,
// falling back to the shared binding table's default. The focused window
// decides whether the command closes an app tab or the window.
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
          closeWindowMenuItem(),
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
        closeWindowMenuItem(),
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
    attachExternalLinkGuards(window.webContents)
  })

  createAppTray()

  ipcMain.handle('window:close-self', (event) => {
    const sender = BrowserWindow.fromWebContents(event.sender)
    sender?.close()
  })
  ipcMain.handle('desktop:server-status', () => getDesktopServerStatus())
  ipcMain.handle('desktop:api-base-url', () => getDesktopApiBaseUrl())
  ipcMain.handle('desktop:auth-token', () => isRemoteMode() ? '' : getDesktopAuthToken())
  ipcMain.handle('desktop:default-workspace-path', (_event, rawDisplayName: unknown) => {
    if (isRemoteMode()) return ''
    return defaultWorkspacePath(typeof rawDisplayName === 'string' ? rawDisplayName : '')
  })
  // A local-mode agent session runs inside a real host folder, so the composer
  // can offer the OS directory picker and bind the chosen path. In remote mode
  // the project lives on the server, where a host path is meaningless, so the
  // picker is withheld and this returns nothing.
  ipcMain.handle('desktop:open-project-folder', async (event) => {
    if (isRemoteMode()) return null
    const owner = BrowserWindow.fromWebContents(event.sender) ?? BrowserWindow.getFocusedWindow()
    const result = owner
      ? await dialog.showOpenDialog(owner, { properties: ['openDirectory', 'createDirectory'] })
      : await dialog.showOpenDialog({ properties: ['openDirectory', 'createDirectory'] })
    if (result.canceled) return null
    return result.filePaths[0] ?? null
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

  // Renderer cache invalidation broadcast. Settings routes share the primary
  // renderer cache, but auxiliary renderers can still exist in desktop builds;
  // when one renderer invalidates Pinia Colada queries, this fans the
  // serializable filter out to siblings. The sender is excluded so we don't
  // echo back into the originating renderer.
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
