import { Elysia } from 'elysia'
import { loadConfig } from '@memohai/config'
import { corsMiddleware } from './middlewares/cors'
import { errorMiddleware } from './middlewares/error'
import { initBrowsers, browsers } from './browser'
import { contextModule } from './modules/context'
import { devicesModule } from './modules/devices'
import { coresModule } from './modules/cores'

const configuredPath = process.env.MEMOH_CONFIG_PATH?.trim() || process.env.CONFIG_PATH?.trim()
const configPath = configuredPath && configuredPath.length > 0 ? configuredPath : '../../config.toml'
const config = loadConfig(configPath)

await initBrowsers()

export { browsers }

const app = new Elysia()
  .use(corsMiddleware)
  .use(errorMiddleware)
  .get('/health', () => ({
    status: 'ok',
  }))
  .use(coresModule)
  .use(contextModule)
  .use(devicesModule)
  .onStop(async () => {
    for (const browser of browsers.values()) {
      await browser.close()
    }
  })
  .listen({
    port: config.browser_gateway.port ?? 8083,
    hostname: config.browser_gateway.host ?? '127.0.0.1',
    idleTimeout: 255,
  })

console.log(`🌐 Browser Gateway is running at ${app.server!.url}`)
