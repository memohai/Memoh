import { Elysia } from 'elysia'
import { storage } from '../storage'
import { z } from 'zod'
import { BrowserContextConfigModel } from '../models'
import { getBrowser, getOrCreateBotBrowser } from '../browser'
import { actionModule } from './action'

export const contextModule = new Elysia({ prefix: '/context' })
  .use(actionModule)
  .get('/:id/exists', ({ params }) => {
    return { exists: storage.has(params.id) }
  })
  .get('/', ({ query }) => {
    const { id } = query
    const entry = storage.get(id)
    if (!entry) return null
    return { id: entry.id, name: entry.name, core: entry.core, config: entry.config }
  }, {
    query: z.object({
      id: z.string(),
    }),
  })
  .post(
    '/',
    async ({ body, set }) => {
      const { name, config, id, bot_id } = body
      const core = config.core ?? 'chromium'

      // Reject duplicate context IDs to prevent orphaning live contexts
      if (storage.has(id)) {
        set.status = 409
        return { error: `context with id "${id}" already exists` }
      }

      // Use per-bot isolated browser process if bot_id provided, otherwise shared fallback
      let browser
      if (bot_id) {
        const botEntry = await getOrCreateBotBrowser(bot_id, core)
        browser = botEntry.browser
      } else {
        browser = getBrowser(core)
      }

      const context = await browser.newContext({
        viewport: config.viewport,
        userAgent: config.userAgent,
        deviceScaleFactor: config.deviceScaleFactor,
        isMobile: config.isMobile,
        locale: config.locale,
        timezoneId: config.timezoneId,
        geolocation: config.geolocation,
        permissions: config.permissions,
        extraHTTPHeaders: config.extraHTTPHeaders,
        ignoreHTTPSErrors: config.ignoreHTTPSErrors,
        proxy: config.proxy,
      })
      storage.set(id, { id, name, botId: bot_id, core, context, config })
      return { id, name, core, config }
    },
    {
      body: z.object({
        name: z.string().default(''),
        config: BrowserContextConfigModel.default({}),
        id: z.string().default(crypto.randomUUID()),
        bot_id: z.string().optional(),
      }),
    },
  )
  .delete('/:id', async ({ params }) => {
    const entry = storage.get(params.id)
    if (entry) {
      await entry.context.close()
      storage.delete(params.id)
    }
    return { success: true }
  })

  // Export storage state (cookies + localStorage) from a context
  .get('/:id/storage-state', async ({ params, set }) => {
    const entry = storage.get(params.id)
    if (!entry) {
      set.status = 404
      return { error: 'context not found' }
    }
    const state = await entry.context.storageState()
    return state
  })

  // Import cookies into an existing context
  .post(
    '/:id/storage-state',
    async ({ params, body, set }) => {
      const entry = storage.get(params.id)
      if (!entry) {
        set.status = 404
        return { error: 'context not found' }
      }
      if (body.cookies && Array.isArray(body.cookies)) {
        await entry.context.addCookies(body.cookies)
      }
      return { success: true }
    },
    {
      body: z.object({
        cookies: z.array(z.any()).optional(),
      }),
    },
  )
