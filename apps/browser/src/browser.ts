import { chromium, firefox } from 'playwright'
import type { Browser } from 'playwright'
import { spawn, type ChildProcess } from 'child_process'

export type BrowserCore = 'chromium' | 'firefox'

// --- Per-bot browser entry ---
// Uses launch() for the gateway's own Browser handle (Bun-compatible),
// and spawns a Node child process running launchServer() for the Tier 2
// remote WS endpoint that Python clients connect to.

export interface BotBrowserEntry {
  botId: string
  core: BrowserCore
  browser: Browser
  // Tier 2: remote WS endpoint for native Playwright clients
  wsEndpoint?: string
  serverProcess?: ChildProcess
}

const botBrowsers = new Map<string, BotBrowserEntry>()
const inflightBrowserCreations = new Map<string, Promise<BotBrowserEntry>>()

const MAX_BOT_BROWSERS = parseInt(process.env.MAX_BOT_BROWSER_SERVERS ?? '20', 10)

function getBrowserType(core: BrowserCore) {
  return core === 'firefox' ? firefox : chromium
}

export async function getOrCreateBotBrowser(botId: string, core: BrowserCore): Promise<BotBrowserEntry> {
  const existing = botBrowsers.get(botId)
  if (existing) {
    if (existing.core !== core) {
      // Reject core change if active sessions/contexts exist
      throw new Error(`Bot ${botId} already has a ${existing.core} browser. Cannot switch to ${core} while active.`)
    }
    return existing
  }

  // Deduplicate concurrent creation for the same bot
  const inflight = inflightBrowserCreations.get(botId)
  if (inflight) return inflight

  const promise = (async () => {
    if (botBrowsers.size >= MAX_BOT_BROWSERS) {
      throw new Error(`Browser limit reached (${MAX_BOT_BROWSERS}). Cannot create new browser for bot ${botId}.`)
    }

    const browserType = getBrowserType(core)
    const browser = await browserType.launch({ headless: true })

    const entry: BotBrowserEntry = { botId, core, browser }
    botBrowsers.set(botId, entry)

    browser.on('disconnected', () => {
      botBrowsers.delete(botId)
      console.log(`Browser for bot ${botId} disconnected unexpectedly, cleaned up.`)
    })

    console.log(`Launched browser for bot ${botId} (${core})`)
    return entry
  })().finally(() => {
    inflightBrowserCreations.delete(botId)
  })

  inflightBrowserCreations.set(botId, promise)
  return promise
}

// Launch a Tier 2 remote server for a bot (Node child process running launchServer)
// Returns the WS endpoint that remote Python clients can connect to.
export async function ensureBotRemoteServer(botId: string, core: BrowserCore): Promise<string> {
  const entry = botBrowsers.get(botId)
  if (entry?.wsEndpoint && entry.serverProcess) {
    return entry.wsEndpoint
  }

  // Ensure bot has a Tier 1 browser first (for gateway-side operations)
  await getOrCreateBotBrowser(botId, core)

  const wsPath = `/${crypto.randomUUID()}`
  const wsEndpoint = await launchRemoteServer(botId, core, wsPath)

  console.log(`Remote WS server for bot ${botId} at ${wsEndpoint}`)
  return wsEndpoint
}

// Spawn a Node child process that runs launchServer() and returns the WS endpoint
function launchRemoteServer(botId: string, core: BrowserCore, wsPath: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const script = `
      const { ${core} } = require('playwright');
      (async () => {
        const server = await ${core}.launchServer({ headless: true, wsPath: '${wsPath}' });
        process.stdout.write('WS:' + server.wsEndpoint() + '\\n');
        process.on('SIGTERM', () => { server.close().then(() => process.exit(0)); });
        process.on('SIGINT', () => { server.close().then(() => process.exit(0)); });
      })().catch(e => { process.stderr.write(e.message); process.exit(1); });
    `

    const child = spawn('node', ['-e', script], {
      cwd: process.cwd(),
      env: process.env,
      stdio: ['ignore', 'pipe', 'pipe'],
    })

    let resolved = false
    const timeout = setTimeout(() => {
      if (!resolved) {
        resolved = true
        child.kill()
        reject(new Error('Timed out launching remote browser server'))
      }
    }, 30000)

    child.stdout!.on('data', (data: Buffer) => {
      const line = data.toString().trim()
      if (line.startsWith('WS:') && !resolved) {
        resolved = true
        clearTimeout(timeout)
        const wsEndpoint = line.slice(3)

        // Directly assign to the correct bot entry by ID
        const botEntry = botBrowsers.get(botId)
        if (botEntry) {
          botEntry.serverProcess = child
          botEntry.wsEndpoint = wsEndpoint
        }

        child.on('exit', () => {
          if (botEntry) {
            botEntry.wsEndpoint = undefined
            botEntry.serverProcess = undefined
            console.log(`Remote server process for bot ${botId} exited`)
          }
        })

        resolve(wsEndpoint)
      }
    })

    child.stderr!.on('data', (data: Buffer) => {
      if (!resolved) {
        resolved = true
        clearTimeout(timeout)
        reject(new Error(`Remote server error: ${data.toString()}`))
      }
    })

    child.on('error', (err) => {
      if (!resolved) {
        resolved = true
        clearTimeout(timeout)
        reject(err)
      }
    })
  })
}

export function getBotBrowser(botId: string): BotBrowserEntry | undefined {
  return botBrowsers.get(botId)
}

export async function closeBotBrowser(botId: string): Promise<void> {
  const entry = botBrowsers.get(botId)
  if (!entry) return
  botBrowsers.delete(botId)
  if (entry.serverProcess) {
    entry.serverProcess.kill('SIGTERM')
  }
  try {
    await entry.browser.close()
  } catch { /* browser may already be closed */ }
  console.log(`Closed browser for bot ${botId}`)
}

export function getAllBotBrowsers(): Map<string, BotBrowserEntry> {
  return botBrowsers
}

// --- Shared fallback browser (backward compat for requests without bot_id) ---

export const browsers = new Map<BrowserCore, Browser>()

export const initBrowsers = async (): Promise<Map<BrowserCore, Browser>> => {
  const raw = process.env.BROWSER_CORES ?? 'chromium'
  const cores = raw.split(',').map(s => s.trim()) as BrowserCore[]

  for (const core of cores) {
    if (core === 'chromium') {
      browsers.set('chromium', await chromium.launch({ headless: true }))
    } else if (core === 'firefox') {
      browsers.set('firefox', await firefox.launch({ headless: true }))
    }
  }

  if (browsers.size === 0) {
    browsers.set('chromium', await chromium.launch({ headless: true }))
  }

  return browsers
}

export const getBrowser = (core: BrowserCore = 'chromium'): Browser => {
  const b = browsers.get(core) ?? browsers.values().next().value
  if (!b) throw new Error(`Browser core "${core}" is not available`)
  return b
}

export const getAvailableCores = (): BrowserCore[] => {
  const raw = process.env.BROWSER_CORES ?? 'chromium'
  return raw.split(',').map(s => s.trim()) as BrowserCore[]
}

// --- Shutdown all ---

export async function closeAllBotBrowsers(): Promise<void> {
  const ids = [...botBrowsers.keys()]
  for (const id of ids) {
    await closeBotBrowser(id)
  }
}
