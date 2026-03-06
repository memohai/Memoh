import { Elysia } from 'elysia'
import { chatModule } from './modules/chat'
import { corsMiddleware } from './middlewares/cors'
import { errorMiddleware } from './middlewares/error'
import { loadConfig, getBaseUrl as getBaseUrlByConfig } from '@memoh/config'
import { AgentAuthContext, AuthFetcher } from '@memoh/agent'

const configuredPath = process.env.MEMOH_CONFIG_PATH?.trim() || process.env.CONFIG_PATH?.trim()
const configPath = configuredPath && configuredPath.length > 0 ? configuredPath : '../../config.toml'
const config = loadConfig(configPath)

export const getBaseUrl = () => {
  return getBaseUrlByConfig(config)
}

function parseJwtExp(token: string): number | null {
  try {
    const base64Url = token.split('.')[1]
    if (!base64Url) return null
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/')
    const jsonPayload = Buffer.from(base64, 'base64').toString('utf8')
    const payload = JSON.parse(jsonPayload)
    return payload.exp ? payload.exp * 1000 : null
  } catch (e) {
    console.error('Failed to parse JWT expiration from token', e)
    return null
  }
}

export const createAuthFetcher = (auth: AgentAuthContext): AuthFetcher => {
  let refreshPromise: Promise<string> | null = null
  return async (url: string, options?: RequestInit) => {
    if (auth.bearer) {
      const exp = parseJwtExp(auth.bearer)
      if (exp !== null && exp - Date.now() < 120000) { // Refresh if expiring in < 2 mins
        if (!refreshPromise) {
          refreshPromise = (async () => {
            const refreshUrl = new URL('/auth/refresh', `${getBaseUrl().replace(/\/$/, '')}/`).toString()
            const res = await fetch(refreshUrl, {
              method: 'POST',
              headers: { 'Authorization': `Bearer ${auth.bearer}` }
            })
            if (res.ok) {
              const data = await res.json()
              return data.access_token
            }
            throw new Error('Failed to refresh token')
          })().finally(() => {
            refreshPromise = null
          })
        }
        try {
          auth.bearer = await refreshPromise
        } catch (e) {
          console.error('Token refresh failed', e)
          throw e
        }
      }
    }

    const requestOptions = options ?? {}
    const headers = new Headers(requestOptions.headers || {})
    if (auth.bearer && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${auth.bearer}`)
    }

    const baseURL = getBaseUrl()
    const requestURL = /^https?:\/\//i.test(url)
      ? url
      : new URL(url, `${baseURL.replace(/\/$/, '')}/`).toString()

    return await fetch(requestURL, {
      ...requestOptions,
      headers,
    })
  }
}

const app = new Elysia()
  .use(corsMiddleware)
  .use(errorMiddleware)
  .get('/health', () => ({
    status: 'ok',
  }))
  .use(chatModule)
  .listen({
    port: config.agent_gateway.port ?? 8081,
    hostname: config.agent_gateway.host ?? '127.0.0.1',
    idleTimeout: 255, // max allowed by Bun, to accommodate long-running tool calls
  })

console.log(
  `⚙️ Agent Gateway is running at ${app.server?.hostname}:${app.server?.port}`,
)
