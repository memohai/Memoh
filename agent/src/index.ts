import { Elysia } from 'elysia'
import { chatModule } from './modules/chat'
import { corsMiddleware } from './middlewares/cors'
import { errorMiddleware } from './middlewares/error'
import { loadConfig, getBaseUrl as getBaseUrlByConfig } from '@memoh/config'
import { AuthFetcher } from '@memoh/agent'

const config = loadConfig('../config.toml')

export const getBaseUrl = () => {
  return getBaseUrlByConfig(config)
}

export const createAuthFetcher = (bearer: string | undefined): AuthFetcher => {
  return async (url: string, options?: RequestInit) => {
    const requestOptions = options ?? {}
    const headers = new Headers(requestOptions.headers || {})
    if (bearer && !headers.has('Authorization')) {
      headers.set('Authorization', `Bearer ${bearer}`)
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
  `Agent Gateway is running at ${app.server?.hostname}:${app.server?.port}`,
)
