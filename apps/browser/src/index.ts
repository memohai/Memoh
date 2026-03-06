import { Elysia } from 'elysia'
import { loadConfig } from '@memoh/config'
import { corsMiddleware } from './middlewares/cors'
import { errorMiddleware } from './middlewares/error'
import { initBrowser } from './browser'
import { contextModule } from './modules/context'
import { devicesModule } from './modules/devices'

const config = loadConfig('../../config.toml')

export const browser = await initBrowser()

const app = new Elysia()
  .use(corsMiddleware)
  .use(errorMiddleware)
  .get('/health', () => ({
    status: 'ok',
  }))
  .use(contextModule)
  .use(devicesModule)
  .onStop(async () => {
    await browser.close()
  })
  .listen({
    port: config.browser_gateway.port ?? 8083,
    hostname: config.browser_gateway.host ?? '127.0.0.1',
    idleTimeout: 255,
  })

console.log(`🌐 Browser Gateway is running at ${app.server!.url}`)

