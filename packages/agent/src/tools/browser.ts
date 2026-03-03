import { tool } from 'ai'
import { z } from 'zod'
import type { AuthFetcher } from '../types'
import type { IdentityContext } from '../types/agent'

interface BrowserToolParams {
  fetch: AuthFetcher
  identity: IdentityContext
}

interface BrowserSession {
  session_id: string
}

const getBasePath = (botId: string) => `/bots/${botId}/browser/sessions`

export const getBrowserTools = ({ fetch, identity }: BrowserToolParams) => {
  const botId = identity.botId.trim()
  const base = getBasePath(botId)

  const newSession = tool({
    description: 'Create a browser session for current bot',
    inputSchema: z.object({
      idle_ttl_seconds: z.number().int().positive().optional().describe('Session idle ttl in seconds'),
    }),
    execute: async (input) => {
      const res = await fetch(base, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input ?? {}),
      })
      return res.json()
    },
  })

  const listSessions = tool({
    description: 'List browser sessions for current bot',
    inputSchema: z.object({}),
    execute: async () => {
      const res = await fetch(base, { method: 'GET' })
      return res.json()
    },
  })

  const executeAction = tool({
    description: 'Execute action in a browser session',
    inputSchema: z.object({
      session_id: z.string().describe('Browser session id'),
      name: z.enum(['goto', 'click', 'type', 'screenshot', 'extract_text']).describe('Action name'),
      url: z.string().optional().describe('Target URL used by goto/extract_text'),
      target: z.string().optional().describe('Selector/target used by click/type/screenshot'),
      value: z.string().optional().describe('Input value used by type'),
      params: z.record(z.string(), z.unknown()).optional().describe('Additional action parameters'),
    }),
    execute: async ({ session_id, ...payload }) => {
      const res = await fetch(`${base}/${session_id}/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      })
      return res.json()
    },
  })

  const closeSession = tool({
    description: 'Close a browser session',
    inputSchema: z.object({
      session_id: z.string().describe('Browser session id'),
    }),
    execute: async ({ session_id }) => {
      const res = await fetch(`${base}/${session_id}`, { method: 'DELETE' })
      return res.json()
    },
  })

  const withAutoSession = async <T>(fn: (session: BrowserSession) => Promise<T>): Promise<T> => {
    const listRes = await fetch(base, { method: 'GET' })
    const listBody = await listRes.json() as { items?: BrowserSession[] }
    const existing = Array.isArray(listBody.items) && listBody.items.length > 0 ? listBody.items[0] : null
    if (existing?.session_id) {
      return fn(existing)
    }
    const createRes = await fetch(base, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: '{}' })
    const created = await createRes.json() as BrowserSession
    return fn(created)
  }

  const goto = tool({
    description: 'Open URL in bot browser session',
    inputSchema: z.object({
      url: z.string().describe('URL to open'),
    }),
    execute: async ({ url }) => withAutoSession(async (session) => {
      const res = await fetch(`${base}/${session.session_id}/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'goto', url }),
      })
      return res.json()
    }),
  })

  const extractText = tool({
    description: 'Extract text from current page or provided URL',
    inputSchema: z.object({
      url: z.string().optional().describe('Optional URL, defaults to current page'),
    }),
    execute: async ({ url }) => withAutoSession(async (session) => {
      const res = await fetch(`${base}/${session.session_id}/actions`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: 'extract_text', url }),
      })
      return res.json()
    }),
  })

  return {
    'browser_new_session': newSession,
    'browser_list_sessions': listSessions,
    'browser_execute_action': executeAction,
    'browser_close_session': closeSession,
    'browser_goto': goto,
    'browser_extract_text': extractText,
  }
}
