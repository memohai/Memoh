import { Elysia } from 'elysia'
import { z } from 'zod'
import { ensureBotRemoteServer } from '../browser'
import type { BrowserCore } from '../browser'

// --- Session types ---

export interface RemotePlaywrightSession {
  id: string
  botId: string
  core: BrowserCore
  wsEndpoint: string
  sessionToken: string
  playwrightVersion: string
  contextConfig?: Record<string, unknown>
  createdAt: Date
  expiresAt: Date
  lastSeenAt: Date
  status: 'active' | 'expired' | 'closed'
}

// --- Session storage ---

const sessions = new Map<string, RemotePlaywrightSession>()

// Per-bot in-flight creation promises to prevent duplicate launches
const inflightCreations = new Map<string, Promise<string>>()

const SESSION_DEFAULT_TTL_MS = 30 * 60 * 1000 // 30 minutes
const SESSION_MAX_TTL_MS = 2 * 60 * 60 * 1000 // 2 hours

function getPlaywrightVersion(): string {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    const pkg = require('playwright/package.json') as { version: string }
    return pkg.version
  } catch {
    return 'unknown'
  }
}

function generateToken(): string {
  const bytes = new Uint8Array(32)
  crypto.getRandomValues(bytes)
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
}

// --- Janitor ---

let janitorHandle: ReturnType<typeof setInterval> | null = null
const JANITOR_INTERVAL_MS = 60_000

function startJanitor() {
  if (janitorHandle) return
  janitorHandle = setInterval(() => {
    const now = new Date()
    for (const [id, session] of sessions) {
      if (session.status !== 'active') continue
      if (now > session.expiresAt) {
        session.status = 'expired'
        sessions.delete(id)
        console.log(`Session ${id} expired (bot: ${session.botId})`)
      }
    }
  }, JANITOR_INTERVAL_MS)
}

startJanitor()

// --- Helper to validate session token ---

function validateSessionToken(sessionId: string, token: string): RemotePlaywrightSession | null {
  const session = sessions.get(sessionId)
  if (!session) return null
  if (session.status !== 'active') return null
  if (session.sessionToken !== token) return null
  if (new Date() > session.expiresAt) return null
  return session
}

// Deduplicated remote server creation
async function getOrCreateRemoteServer(botId: string, core: BrowserCore): Promise<string> {
  const existing = inflightCreations.get(botId)
  if (existing) return existing

  const promise = ensureBotRemoteServer(botId, core).finally(() => {
    inflightCreations.delete(botId)
  })
  inflightCreations.set(botId, promise)
  return promise
}

// --- Elysia module ---
//
// Remote sessions give the client a WS endpoint to a dedicated per-bot
// Playwright server. The client gets full native Playwright API access —
// they create their own contexts, pages, cookies, etc. The gateway only
// tracks session lifecycle metadata (expiry, auth token).

export const sessionModule = new Elysia({ prefix: '/session' })
  // Create a remote Playwright session
  .post(
    '/',
    async ({ body, set }) => {
      const { bot_id, core, ttl_ms, context_config } = body
      const sessionCore = core ?? 'chromium'

      // Launch or reuse the bot's remote Playwright server (Node child process)
      const wsEndpoint = await getOrCreateRemoteServer(bot_id, sessionCore)

      const sessionId = crypto.randomUUID()
      const sessionToken = generateToken()
      const ttl = Math.min(ttl_ms ?? SESSION_DEFAULT_TTL_MS, SESSION_MAX_TTL_MS)
      const now = new Date()

      const session: RemotePlaywrightSession = {
        id: sessionId,
        botId: bot_id,
        core: sessionCore,
        wsEndpoint,
        sessionToken,
        playwrightVersion: getPlaywrightVersion(),
        contextConfig: context_config,
        createdAt: now,
        expiresAt: new Date(now.getTime() + ttl),
        lastSeenAt: now,
        status: 'active',
      }

      sessions.set(sessionId, session)
      set.status = 201

      console.log(`Created remote session ${sessionId} for bot ${bot_id} (core: ${sessionCore}, expires: ${session.expiresAt.toISOString()})`)

      return {
        id: sessionId,
        ws_endpoint: wsEndpoint,
        session_token: sessionToken,
        playwright_version: session.playwrightVersion,
        core: sessionCore,
        context_config: context_config ?? {},
        expires_at: session.expiresAt.toISOString(),
      }
    },
    {
      body: z.object({
        bot_id: z.string(),
        core: z.enum(['chromium', 'firefox']).optional(),
        ttl_ms: z.number().optional(),
        context_config: z.record(z.string(), z.any()).optional(),
      }),
    },
  )

  // Get session metadata
  .get(
    '/:id',
    ({ params, query, set }) => {
      const session = validateSessionToken(params.id, query.token ?? '')
      if (!session) {
        set.status = 404
        return { error: 'session not found or invalid token' }
      }
      return {
        id: session.id,
        bot_id: session.botId,
        core: session.core,
        ws_endpoint: session.wsEndpoint,
        status: session.status,
        playwright_version: session.playwrightVersion,
        context_config: session.contextConfig ?? {},
        created_at: session.createdAt.toISOString(),
        expires_at: session.expiresAt.toISOString(),
        last_seen_at: session.lastSeenAt.toISOString(),
      }
    },
    {
      query: z.object({ token: z.string().optional() }),
    },
  )

  // Close session
  .delete(
    '/:id',
    ({ params, query, set }) => {
      const session = validateSessionToken(params.id, query.token ?? '')
      if (!session) {
        set.status = 404
        return { error: 'session not found or invalid token' }
      }

      session.status = 'closed'
      sessions.delete(session.id)

      console.log(`Closed remote session ${session.id} (bot: ${session.botId})`)
      return { success: true }
    },
    {
      query: z.object({ token: z.string().optional() }),
    },
  )

  // Heartbeat — extend session lifetime
  .post(
    '/:id/heartbeat',
    ({ params, query, set }) => {
      const session = validateSessionToken(params.id, query.token ?? '')
      if (!session) {
        set.status = 404
        return { error: 'session not found or invalid token' }
      }

      const now = new Date()
      const extension = Math.min(SESSION_DEFAULT_TTL_MS, SESSION_MAX_TTL_MS - (now.getTime() - session.createdAt.getTime()))

      if (extension > 0) {
        session.expiresAt = new Date(now.getTime() + extension)
      }
      session.lastSeenAt = now

      return {
        expires_at: session.expiresAt.toISOString(),
        remaining_ms: session.expiresAt.getTime() - now.getTime(),
      }
    },
    {
      query: z.object({ token: z.string().optional() }),
    },
  )

// --- Exports for shutdown ---

export function getActiveSessions(): Map<string, RemotePlaywrightSession> {
  return sessions
}

export async function closeAllSessions(): Promise<void> {
  sessions.clear()
  if (janitorHandle) {
    clearInterval(janitorHandle)
    janitorHandle = null
  }
}
